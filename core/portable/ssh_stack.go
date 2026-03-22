package portable

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tunnelbypass/core/installer"
	tbssh "tunnelbypass/core/ssh"
	"tunnelbypass/core/udpgw"
)

// RunSSHStack starts internal UDPGW and embedded SSH; blocks until ctx cancelled.
func RunSSHStack(ctx context.Context, log *slog.Logger, sshPort, udpgwPort int, username, password string) error {
	if log == nil {
		log = slog.Default()
	}
	if udpgwPort <= 0 {
		udpgwPort = 7300
	}
	if sshPort <= 0 {
		sshPort = tbssh.ListenPreference()
	}
	udpgwPort = installer.EnsureFreeTCPPort(udpgwPort, "UDPGW")
	sshPort = installer.EnsureFreeTCPPort(sshPort, "sshembed")

	u, pw := resolveEmbedCredentials(username, password)

	keyDir := installer.GetConfigDir("ssh")
	if err := os.MkdirAll(keyDir, 0755); err != nil {
		return fmt.Errorf("ssh configs dir: %w", err)
	}
	keyPath := filepath.Join(keyDir, "embed_host_key")

	udgwLog := log.With("component", "udpgw")
	sshLog := log.With("component", "tbssh")

	log.Info("portable ssh stack: starting",
		"pid", os.Getpid(),
		"data_dir", installer.GetBaseDir(),
		"udpgw_tcp_port", udpgwPort,
		"ssh_listen_port", sshPort,
		"ssh_user", u)

	udpgwCtx, cancelU := context.WithCancel(ctx)
	defer cancelU()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		udgwLog.Info("udpgw: listener starting", "addr", fmt.Sprintf("127.0.0.1:%d", udpgwPort), "pid", os.Getpid())
		err := udpgw.Run(udpgwCtx, udpgw.Options{Port: udpgwPort, Logger: udgwLog})
		if err != nil && udpgwCtx.Err() == nil {
			udgwLog.Error("udpgw: exited unexpectedly", "err", err,
				"hint", "check TB_UDPGW_* env and port conflicts; clients need TCP forward to this port over SSH")
		} else {
			udgwLog.Info("udpgw: stopped", "reason", "context_done_or_normal")
		}
	}()

	if err := waitUDPGWListen(ctx, udpgwPort, cancelU, &wg, log); err != nil {
		return err
	}
	udgwLog.Info("udpgw: listening", "port", udpgwPort, "pid", os.Getpid())

	_ = WriteRunMeta(installer.GetBaseDir(), "ssh", RunMeta{
		Ports: map[string]int{"ssh": sshPort, "udpgw": udpgwPort},
	})

	tbssh.StartSOCKS5IfConfigured(ctx, log)
	errSSH := runEmbedSSHListener(ctx, sshLog, sshPort, udpgwPort, u, pw, keyPath)

	tbssh.UseSystemBackend()
	cancelU()
	wg.Wait()

	if errSSH != nil && ctx.Err() != nil {
		log.Info("portable ssh stack: shutdown", "pid", os.Getpid(), "reason", "signal_or_cancel")
		return ctx.Err()
	}
	if errSSH != nil {
		sshLog.Error("embedded ssh: error", "err", errSSH)
	} else {
		sshLog.Info("embedded ssh: listener closed", "pid", os.Getpid())
	}
	log.Info("portable ssh stack: stopped", "pid", os.Getpid())
	return errSSH
}

// RunEmbeddedSSH runs only the embedded SSH listener; UDPGW must already be listening on udpgwPort.
func RunEmbeddedSSH(ctx context.Context, log *slog.Logger, sshPort, udpgwPort int, username, password string) error {
	if log == nil {
		log = slog.Default()
	}
	if udpgwPort <= 0 {
		udpgwPort = 7300
	}
	if sshPort <= 0 {
		sshPort = tbssh.ListenPreference()
	}
	sshPort = installer.EnsureFreeTCPPort(sshPort, "sshembed")

	u, pw := resolveEmbedCredentials(username, password)

	keyDir := installer.GetConfigDir("ssh")
	if err := os.MkdirAll(keyDir, 0755); err != nil {
		return fmt.Errorf("ssh configs dir: %w", err)
	}
	keyPath := filepath.Join(keyDir, "embed_host_key")
	sshLog := log.With("component", "tbssh")

	log.Info("portable ssh embed: starting",
		"pid", os.Getpid(),
		"data_dir", installer.GetBaseDir(),
		"udpgw_tcp_port", udpgwPort,
		"ssh_listen_port", sshPort,
		"ssh_user", u)

	if err := waitUDPGWListenShort(ctx, udpgwPort, log); err != nil {
		return err
	}

	_ = WriteRunMeta(installer.GetBaseDir(), "ssh", RunMeta{
		Ports: map[string]int{"ssh": sshPort, "udpgw": udpgwPort},
	})

	tbssh.StartSOCKS5IfConfigured(ctx, log)
	errSSH := runEmbedSSHListener(ctx, sshLog, sshPort, udpgwPort, u, pw, keyPath)

	tbssh.UseSystemBackend()

	if errSSH != nil && ctx.Err() != nil {
		log.Info("portable ssh embed: shutdown", "pid", os.Getpid(), "reason", "signal_or_cancel")
		return ctx.Err()
	}
	if errSSH != nil {
		sshLog.Error("embedded ssh: error", "err", errSSH)
	} else {
		sshLog.Info("embedded ssh: listener closed", "pid", os.Getpid())
	}
	log.Info("portable ssh embed: stopped", "pid", os.Getpid())
	return errSSH
}

func resolveEmbedCredentials(username, password string) (u, pw string) {
	u = strings.TrimSpace(username)
	if u == "" {
		u = strings.TrimSpace(os.Getenv("TB_SSH_USER"))
	}
	if u == "" {
		u = strings.TrimSpace(os.Getenv("TB_EMBED_SSH_USER"))
	}
	if u == "" {
		u = "tunnelbypass"
	}
	pw = strings.TrimSpace(password)
	if pw == "" {
		pw = strings.TrimSpace(os.Getenv("TB_SSH_PASSWORD"))
	}
	if pw == "" {
		pw = installer.ReadOrCreateEmbedSSHPassword()
	}
	return u, pw
}

func waitUDPGWListen(ctx context.Context, udpgwPort int, cancelU context.CancelFunc, wg *sync.WaitGroup, log *slog.Logger) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if installer.PortListening(udpgwPort) {
			return nil
		}
		select {
		case <-ctx.Done():
			cancelU()
			wg.Wait()
			log.Info("portable ssh stack: cancelled before udpgw ready", "pid", os.Getpid())
			return ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
	cancelU()
	wg.Wait()
	log.Error("udpgw: failed to listen in time", "port", udpgwPort,
		"hint", "choose another port with --udpgw-port or free the port")
	return fmt.Errorf("udpgw did not listen on port %d", udpgwPort)
}

func waitUDPGWListenShort(ctx context.Context, udpgwPort int, log *slog.Logger) error {
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if installer.PortListening(udpgwPort) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
	log.Error("udpgw: not reachable", "port", udpgwPort)
	return fmt.Errorf("udpgw not listening on port %d", udpgwPort)
}

func runEmbedSSHListener(ctx context.Context, sshLog *slog.Logger, sshPort, udpgwPort int, u, pw, keyPath string) error {
	sshLog.Info("embedded ssh: listening", "addr", fmt.Sprintf("0.0.0.0:%d", sshPort), "user", u,
		"udpgw_tcp", fmt.Sprintf("127.0.0.1:%d", udpgwPort),
		"pid", os.Getpid(),
		"hint", "Clients: SSH to this host:ssh_port; for UDP over SSH forward local TCP to 127.0.0.1:udpgw_port on the server")

	cfg := tbssh.Config{
		ListenAddr: fmt.Sprintf("0.0.0.0:%d", sshPort),
		Username:   u,
		Password:   pw,
		KeyPath:    keyPath,
		Logger:     sshLog,
	}
	tbssh.SetEmbedBackend(sshPort)
	return tbssh.Run(ctx, cfg)
}
