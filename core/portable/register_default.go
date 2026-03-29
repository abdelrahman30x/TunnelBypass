package portable

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/udpgw"
)

func init() {
	Register("ssh", func() Transport { return sshTransport{} })
	Register("udpgw", func() Transport { return udpgwTransport{} })
	Register("vless", func() Transport { return vlessTransport{} })
	Register("vless-ws", func() Transport { return vlesswsTransport{} })
	Register("reality", func() Transport { return realityTransport{} })
	Register("hysteria", func() Transport { return hysteriaTransport{} })
	Register("wireguard", func() Transport { return wireguardTransport{} })
	Register("wss", func() Transport { return wssTransport{} })
	Register("tls", func() Transport { return tlsTransport{} })
}

type sshTransport struct{}

func (sshTransport) Name() string { return "ssh" }

func (sshTransport) Dependencies() []string { return []string{"udpgw"} }

func (sshTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	if o.ExternalUDPGW {
		return RunEmbeddedSSH(ctx, log, o.SSHPort, o.UDPGWPort, o.SSHUser, o.SSHPass)
	}
	return RunSSHStack(ctx, log, o.SSHPort, o.UDPGWPort, o.SSHUser, o.SSHPass)
}

type udpgwTransport struct{}

func (udpgwTransport) Name() string { return "udpgw" }

func (udpgwTransport) Dependencies() []string { return nil }

func (udpgwTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	log.Warn("udpgw: standalone mode is deprecated; use `run portable ssh` (or `run --portable ssh`) for SSH+UDPGW")
	p := o.UDPGWPort
	if p <= 0 {
		p = 7300
	}
	return udpgw.Run(ctx, udpgw.Options{Port: p, Logger: log.With("component", "udpgw")})
}

type vlessTransport struct{}

func (vlessTransport) Name() string { return "vless" }

func (vlessTransport) Dependencies() []string { return nil }

func (vlessTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	cfg := defaultConfigPath("vless", "server.json", o.ConfigPath)
	if _, err := os.Stat(cfg); err != nil {
		return fmt.Errorf("vless: config not found at %s", cfg)
	}
	exe, err := installer.EnsureBinary("xray")
	if err != nil {
		return err
	}
	_ = WriteRunMeta(installer.GetBaseDir(), "vless", RunMeta{Extra: map[string]any{"config": cfg}})
	return runForeground(ctx, log, "xray", exe, []string{"run", "-config", cfg})
}

type vlesswsTransport struct{}

func (vlesswsTransport) Name() string { return "vless-ws" }

func (vlesswsTransport) Dependencies() []string { return nil }

func (vlesswsTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	cfg := defaultConfigPath("vless-ws", "server.json", o.ConfigPath)
	if _, err := os.Stat(cfg); err != nil {
		return fmt.Errorf("vless-ws: config not found at %s", cfg)
	}
	exe, err := installer.EnsureBinary("xray")
	if err != nil {
		return err
	}
	_ = WriteRunMeta(installer.GetBaseDir(), "vless-ws", RunMeta{Extra: map[string]any{"config": cfg}})
	return runForeground(ctx, log, "xray", exe, []string{"run", "-config", cfg})
}

type realityTransport struct{}

func (realityTransport) Name() string { return "reality" }

func (realityTransport) Dependencies() []string { return nil }

func (realityTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	return vlessTransport{}.Run(ctx, log, o)
}

type hysteriaTransport struct{}

func (hysteriaTransport) Name() string { return "hysteria" }

func (hysteriaTransport) Dependencies() []string { return nil }

func (hysteriaTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	cfg := defaultConfigPath("hysteria", "server.yaml", o.ConfigPath)
	if _, err := os.Stat(cfg); err != nil {
		return fmt.Errorf("hysteria: config not found at %s", cfg)
	}
	exe, err := installer.EnsureBinary("hysteria")
	if err != nil {
		return err
	}
	_ = WriteRunMeta(installer.GetBaseDir(), "hysteria", RunMeta{Extra: map[string]any{"config": cfg}})
	return runForeground(ctx, log, "hysteria", exe, []string{"server", "-c", cfg})
}

type wireguardTransport struct{}

func (wireguardTransport) Name() string { return "wireguard" }

func (wireguardTransport) Dependencies() []string { return nil }

func (wireguardTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("wireguard: portable run is not supported on Windows from this CLI; use WireGuard or the setup wizard")
	}
	conf := defaultConfigPath("wireguard", "wg_client.conf", o.ConfigPath)
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("wireguard: client config not found at %s", conf)
	}
	wgQuick, err := exec.LookPath("wg-quick")
	if err != nil {
		return fmt.Errorf("wireguard: wg-quick not found in PATH")
	}
	_ = WriteRunMeta(installer.GetBaseDir(), "wireguard", RunMeta{Extra: map[string]any{"config": conf}})
	up := exec.CommandContext(ctx, wgQuick, "up", conf)
	up.Stdout = os.Stdout
	up.Stderr = os.Stderr
	log.Info("portable: wg-quick up", "config", conf)
	if err := up.Run(); err != nil {
		return fmt.Errorf("wg-quick up: %w", err)
	}
	defer func() {
		down := exec.Command(wgQuick, "down", conf)
		down.Stdout = os.Stdout
		down.Stderr = os.Stderr
		_ = down.Run()
	}()
	<-ctx.Done()
	return ctx.Err()
}

type wssTransport struct{}

func (wssTransport) Name() string { return "wss" }

func (wssTransport) Dependencies() []string { return []string{"ssh"} }

func (wssTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	back := installer.GetSSHBackendPort()
	if !installer.PortListening(back) {
		return fmt.Errorf("wss: local SSH not listening on port %d; start `tunnelbypass run --portable ssh` (or system sshd) first", back)
	}
	sshBack := fmt.Sprintf("127.0.0.1:%d", back)
	wssPort := o.WssPort
	if wssPort <= 0 {
		wssPort = 443
	}
	cfgDir := installer.GetConfigDir("wstunnel")
	certPath := filepath.Join(cfgDir, "wss-cert.pem")
	keyPath := filepath.Join(cfgDir, "wss-key.pem")
	if _, err := os.Stat(certPath); err != nil {
		return fmt.Errorf("wss: missing TLS cert %s (complete SSL setup in wizard first)", certPath)
	}
	if _, err := os.Stat(keyPath); err != nil {
		return fmt.Errorf("wss: missing TLS key %s", keyPath)
	}
	exe, err := installer.EnsureBinary("wstunnel")
	if err != nil {
		return err
	}
	args := []string{
		"server",
		"--restrict-to", sshBack,
		fmt.Sprintf("wss://0.0.0.0:%d", wssPort),
		"--tls-certificate", certPath,
		"--tls-private-key", keyPath,
	}
	_ = WriteRunMeta(installer.GetBaseDir(), "wss", RunMeta{Ports: map[string]int{"wss": wssPort}, Extra: map[string]any{"ssh_backend": sshBack}})
	return runForeground(ctx, log, "wstunnel", exe, args)
}

type tlsTransport struct{}

func (tlsTransport) Name() string { return "tls" }

func (tlsTransport) Dependencies() []string { return []string{"ssh"} }

func (tlsTransport) Run(ctx context.Context, log *slog.Logger, o Options) error {
	back := installer.GetSSHBackendPort()
	if !installer.PortListening(back) {
		return fmt.Errorf("tls: local SSH not listening on port %d; start `tunnelbypass run portable ssh` (or system sshd) first", back)
	}
	stunnelPath, err := installer.EnsureStunnel()
	if err != nil {
		return err
	}
	conf := filepath.Join(installer.GetConfigDir("stunnel"), "stunnel-server.conf")
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("tls: missing stunnel server config %s", conf)
	}
	_ = WriteRunMeta(installer.GetBaseDir(), "tls", RunMeta{Extra: map[string]any{"config": conf, "ssh_backend_port": back}})
	return runForeground(ctx, log, "stunnel", stunnelPath, []string{conf})
}
