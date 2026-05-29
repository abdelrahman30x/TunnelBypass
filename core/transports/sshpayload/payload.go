package sshpayload

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	DefaultListenPort    = 80
	DefaultMaxHeaderSize = 16 * 1024
	DefaultHeaderTimeout = 5 * time.Second
	DefaultDialTimeout   = 5 * time.Second
)

type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

type Options struct {
	ListenAddr     string
	TargetAddr     string
	PayloadPath    string
	MaxHeaderBytes int
	HeaderTimeout  time.Duration
	DialTimeout    time.Duration
	Logger         *slog.Logger
	DialContext    DialContextFunc
}

func NormalizePayloadPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if i := strings.IndexAny(path, " \t\r\n"); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		return ""
	}
	return path
}

func GeneratePayloadPath() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "/ssh-ws-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "/ssh-ws-" + hex.EncodeToString(b[:])
}

func Run(ctx context.Context, opt Options) error {
	opt = withDefaults(opt)
	if opt.ListenAddr == "" {
		return errors.New("ssh-payload: listen address is required")
	}
	if opt.TargetAddr == "" {
		return errors.New("ssh-payload: target address is required")
	}
	if opt.PayloadPath == "" {
		return errors.New("ssh-payload: payload path is required")
	}

	ln, err := net.Listen("tcp", opt.ListenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	if opt.Logger != nil {
		opt.Logger.Info("ssh-payload: listening", "listen", opt.ListenAddr, "target", opt.TargetAddr, "path", opt.PayloadPath)
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if opt.Logger != nil {
				opt.Logger.Warn("ssh-payload: accept failed", "err", err)
			}
			continue
		}
		go handleConn(ctx, c, opt)
	}
}

func withDefaults(opt Options) Options {
	opt.PayloadPath = NormalizePayloadPath(opt.PayloadPath)
	if opt.MaxHeaderBytes <= 0 {
		opt.MaxHeaderBytes = DefaultMaxHeaderSize
	}
	if opt.HeaderTimeout <= 0 {
		opt.HeaderTimeout = DefaultHeaderTimeout
	}
	if opt.DialTimeout <= 0 {
		opt.DialTimeout = DefaultDialTimeout
	}
	if opt.DialContext == nil {
		d := net.Dialer{Timeout: opt.DialTimeout}
		opt.DialContext = d.DialContext
	}
	return opt
}

func handleConn(ctx context.Context, client net.Conn, opt Options) {
	opt = withDefaults(opt)
	defer client.Close()

	header, extra, err := readHTTPHeader(client, opt.MaxHeaderBytes, opt.HeaderTimeout)
	if err != nil {
		if opt.Logger != nil {
			opt.Logger.Debug("ssh-payload: header read failed", "err", err)
		}
		return
	}

	req := parseRequestHeader(header)
	if !pathMatches(req.Path, opt.PayloadPath) {
		_, _ = client.Write([]byte("HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n"))
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, opt.DialTimeout)
	defer cancel()
	backend, err := opt.DialContext(dialCtx, "tcp", opt.TargetAddr)
	if err != nil {
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n"))
		return
	}
	defer backend.Close()

	if _, err := client.Write(responseFor(req)); err != nil {
		return
	}
	if len(extra) > 0 {
		if _, err := backend.Write(extra); err != nil {
			return
		}
	}

	bridge(client, backend)
}

func readHTTPHeader(c net.Conn, max int, timeout time.Duration) ([]byte, []byte, error) {
	if timeout > 0 {
		_ = c.SetReadDeadline(time.Now().Add(timeout))
		defer c.SetReadDeadline(time.Time{})
	}
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := c.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if idx := bytes.Index(buf, []byte("\r\n\r\n")); idx >= 0 {
				end := idx + 4
				if end > max {
					return nil, nil, fmt.Errorf("header too large: %d > %d", end, max)
				}
				return buf[:end], buf[end:], nil
			}
			if idx := bytes.Index(buf, []byte("\n\n")); idx >= 0 {
				end := idx + 2
				if end > max {
					return nil, nil, fmt.Errorf("header too large: %d > %d", end, max)
				}
				return buf[:end], buf[end:], nil
			}
			if len(buf) > max {
				return nil, nil, fmt.Errorf("header too large: %d > %d", len(buf), max)
			}
		}
		if err != nil {
			return nil, nil, err
		}
	}
}

type requestHeader struct {
	Method    string
	Path      string
	UpgradeWS bool
}

func parseRequestHeader(header []byte) requestHeader {
	text := strings.ReplaceAll(string(header), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var req requestHeader
	if len(lines) > 0 {
		fields := strings.Fields(lines[0])
		if len(fields) >= 2 {
			req.Method = strings.ToUpper(fields[0])
			req.Path = normalizeRequestTarget(fields[1])
		}
	}
	for _, line := range lines[1:] {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "upgrade:") && strings.Contains(lower, "websocket") {
			req.UpgradeWS = true
			break
		}
	}
	return req
}

func normalizeRequestTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if i := strings.Index(target, "://"); i >= 0 {
			rest := target[i+3:]
			if slash := strings.Index(rest, "/"); slash >= 0 {
				target = rest[slash:]
			}
		}
	}
	if q := strings.Index(target, "?"); q >= 0 {
		target = target[:q]
	}
	return NormalizePayloadPath(target)
}

func pathMatches(got, want string) bool {
	return NormalizePayloadPath(got) == NormalizePayloadPath(want)
}

func responseFor(req requestHeader) []byte {
	if req.Method == "CONNECT" {
		return []byte("HTTP/1.1 200 Connection Established\r\nConnection: keep-alive\r\n\r\n")
	}
	if req.UpgradeWS {
		return []byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
	}
	return []byte("HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\n")
}

func bridge(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		closeWrite(b)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		closeWrite(a)
	}()
	wg.Wait()
}

func closeWrite(c net.Conn) {
	type closeWriter interface {
		CloseWrite() error
	}
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}
