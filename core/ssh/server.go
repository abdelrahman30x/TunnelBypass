// Package tbssh: minimal embedded SSH (password auth, direct-tcpip).
package tbssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

const maxConcurrentSessions = 20

// Config holds listen address and credentials.
type Config struct {
	ListenAddr string
	Username   string
	Password   string
	KeyPath    string
	Logger     *slog.Logger
}

func (c Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Run listens until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "0.0.0.0:2222"
	}
	if cfg.Username == "" {
		return fmt.Errorf("tbssh: Username required")
	}
	if cfg.Password == "" {
		return fmt.Errorf("tbssh: Password required")
	}

	signer, err := hostSigner(cfg.KeyPath, cfg.logger())
	if err != nil {
		return err
	}

	serverConf := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == cfg.Username && subtle.ConstantTimeCompare(password, []byte(cfg.Password)) == 1 {
				return &ssh.Permissions{}, nil
			}
			return nil, fmt.Errorf("denied")
		},
	}
	serverConf.AddHostKey(signer)

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	cfg.logger().Info("tbssh: listening", "addr", cfg.ListenAddr)

	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			handleConn(ctx, c, serverConf, cfg.logger())
		}(conn)
	}
}

func hostSigner(keyPath string, log *slog.Logger) (ssh.Signer, error) {
	if keyPath == "" {
		return nil, fmt.Errorf("tbssh: KeyPath required")
	}
	if _, err := os.Stat(keyPath); err != nil {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		blk, err := ssh.MarshalPrivateKey(priv, "")
		if err != nil {
			return nil, err
		}
		_ = os.MkdirAll(filepath.Dir(keyPath), 0700)
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(blk), 0600); err != nil {
			return nil, err
		}
		log.Info("tbssh: generated host key", "path", keyPath)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(keyPEM)
}

func handleConn(ctx context.Context, conn net.Conn, cfg *ssh.ServerConfig, log *slog.Logger) {
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		log.Debug("tbssh: handshake failed", "err", err)
		return
	}
	defer sshConn.Close()
	go ssh.DiscardRequests(reqs)

	var activeSessions int32
 
	for newCh := range chans {
		switch newCh.ChannelType() {
		case "session":
			if atomic.LoadInt32(&activeSessions) >= maxConcurrentSessions {
				_ = newCh.Reject(ssh.ResourceShortage, "too many sessions")
				continue
			}
			atomic.AddInt32(&activeSessions, 1)
			go func(ch ssh.NewChannel) {
				defer atomic.AddInt32(&activeSessions, -1)
				handleSession(ch, log)
			}(newCh)
		case "direct-tcpip":
			go handleDirectTCPIP(ctx, newCh, log)
		default:
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func handleSession(ch ssh.NewChannel, log *slog.Logger) {
	c, reqs, err := ch.Accept()
	if err != nil {
		return
	}
	defer c.Close()
	for req := range reqs {
		_ = req.Reply(false, nil)
	}
}

func handleDirectTCPIP(ctx context.Context, ch ssh.NewChannel, log *slog.Logger) {
	var payload struct {
		Host       string
		Port       uint32
		OriginAddr string
		OriginPort uint32
	}
	if err := ssh.Unmarshal(ch.ExtraData(), &payload); err != nil {
		_ = ch.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	dest := net.JoinHostPort(payload.Host, fmt.Sprintf("%d", payload.Port))
	d := net.Dialer{Timeout: 20 * time.Second}
	remote, err := d.DialContext(ctx, "tcp", dest)
	if err != nil {
		log.Debug("tbssh: dial backend failed", "dest", dest, "err", err)
		_ = ch.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	sshCh, reqs, err := ch.Accept()
	if err != nil {
		_ = remote.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	go func() {
		_, _ = io.Copy(sshCh, remote)
		_ = sshCh.Close()
		_ = remote.Close()
	}()
	_, _ = io.Copy(remote, sshCh)
	_ = remote.Close()
	_ = sshCh.Close()
}
