// Package provision: non-interactive config generation for run and wizard.
package provision

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	tbtransport "tunnelbypass/core/transport"
	"tunnelbypass/core/transports/hysteria"
	tbssh "tunnelbypass/core/transports/ssh"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/transports/wireguard"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
)

type Result = tbtransport.Result

func NormalizeUUID(uuid string) string {
	u := strings.TrimSpace(uuid)
	if u == "" || strings.EqualFold(u, "auto") {
		return utils.GenerateUUID()
	}
	return u
}

func ResolveServerAddr(addr string) string {
	if strings.TrimSpace(addr) != "" {
		return strings.TrimSpace(addr)
	}
	if ip := utils.GetPublicIP(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

func ensureHost(opt *types.ConfigOptions) {
	if opt.Host == "" {
		opt.Host = opt.ServerAddr
	}
}

func mkdirLogs(base string) {
	_ = os.MkdirAll(filepath.Join(base, "logs"), 0755)
}

func CopyFileIfDifferent(log *slog.Logger, canonicalSrc, dst string) error {
	dst = strings.TrimSpace(dst)
	if dst == "" {
		return nil
	}
	if filepath.Clean(canonicalSrc) == filepath.Clean(dst) {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	b, err := os.ReadFile(canonicalSrc)
	if err != nil {
		return fmt.Errorf("read canonical config: %w", err)
	}
	if err := os.WriteFile(dst, b, 0644); err != nil {
		return fmt.Errorf("write client/server override path: %w", err)
	}
	if log != nil {
		log.Info("provision: copied config to override path", "from", canonicalSrc, "to", dst)
	}
	return nil
}

func ByTransport(log *slog.Logger, transport string, opt types.ConfigOptions, serverConfigOut, clientConfigOut string) (Result, error) {
	baseDir := installer.GetBaseDir()
	_ = os.MkdirAll(filepath.Join(baseDir, "configs"), 0755)
	mkdirLogs(baseDir)
	return tbtransport.Provision(log, transport, opt, serverConfigOut, clientConfigOut)
}

func provisionReality(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = types.TransportReality
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = 443
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-VLESS")

	opt.UUID = NormalizeUUID(opt.UUID)
	if opt.PrivateKey == "" || opt.PublicKey == "" {
		priv, pub, err := utils.GenerateX25519Keys()
		if err != nil {
			return r, fmt.Errorf("reality keys: %w", err)
		}
		opt.PrivateKey, opt.PublicKey = priv, pub
	}
	if len(opt.ShortIds) == 0 {
		opt.ShortIds = utils.GenerateRandomShortIds()
	}
	if strings.TrimSpace(opt.RealityDest) == "" {
		opt.RealityDest = "www.facebook.com:443"
	}

	srv, err := vless.GenerateServerConfig(opt)
	if err != nil {
		return r, fmt.Errorf("reality server config: %w", err)
	}
	cli, err := vless.GenerateClientConfig(opt)
	if err != nil {
		return r, fmt.Errorf("reality client config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink = vless.GenerateVlessURL(opt)

	v2 := vless.GenerateV2rayNJSON(opt, opt.ServerAddr)
	configsDir := installer.GetConfigDir("vless")
	_ = os.MkdirAll(configsDir, 0755)
	allLinks := vless.GenerateAllSNIUrls(opt)
	_ = os.WriteFile(filepath.Join(configsDir, "v2rayn.json"), []byte(v2), 0644)
	_ = os.WriteFile(filepath.Join(configsDir, "sharing-links-all.txt"),
		[]byte("# Tunnel — sharing links (all hostnames)\n"+strings.Join(allLinks, "\n\n")), 0644)

	qrPath := filepath.Join(configsDir, "qr-primary.png")
	if err := utils.SaveQRCodePNG(qrPath, r.SharingLink, 320); err != nil && log != nil {
		log.Warn("provision: qr png", "err", err)
	}

	if err := CopyFileIfDifferent(log, srv, serverOut); err != nil {
		return r, err
	}
	if err := CopyFileIfDifferent(log, cli, clientOut); err != nil {
		return r, err
	}
	r.ListenPort = opt.Port
	return r, nil
}

func provisionVlessWS(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = "vless-ws"
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = 443
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-VLESS-WS")

	opt.WSPath = vless.NormalizeWSPath(opt.WSPath)
	opt.UUID = NormalizeUUID(opt.UUID)

	srv, err := vless.GenerateVlessWSServerConfig(opt)
	if err != nil {
		return r, fmt.Errorf("vless-ws server config: %w", err)
	}
	cli, err := vless.GenerateVlessWSClientConfig(opt)
	if err != nil {
		return r, fmt.Errorf("vless-ws client config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink = vless.GenerateVlessWSURL(opt)

	configsDir := installer.GetConfigDir("vless-ws")
	_ = os.MkdirAll(configsDir, 0755)
	_ = os.WriteFile(filepath.Join(configsDir, "sharing-link.txt"),
		[]byte("# Tunnel — VLESS WebSocket+TLS\n"+r.SharingLink+"\n"), 0644)

	qrPath := filepath.Join(configsDir, "qr-vless-ws.png")
	if err := utils.SaveQRCodePNG(qrPath, r.SharingLink, 320); err != nil && log != nil {
		log.Warn("provision: qr png", "err", err)
	}

	if err := CopyFileIfDifferent(log, srv, serverOut); err != nil {
		return r, err
	}
	if err := CopyFileIfDifferent(log, cli, clientOut); err != nil {
		return r, err
	}
	r.ListenPort = opt.Port
	return r, nil
}

func provisionHysteria(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = types.TransportHysteria
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	if opt.Port == 0 {
		opt.Port = 8443
	}
	ApplyPortAllocation(log, &opt.Port, "udp", "TunnelBypass-Hysteria")

	opt.UUID = NormalizeUUID(opt.UUID)
	if strings.TrimSpace(opt.ObfsPassword) != "" && len(strings.TrimSpace(opt.ObfsPassword)) < 4 {
		opt.ObfsPassword = utils.GenerateUUID()
	}

	srv, cli, err := hysteria.GenerateHysteriaConfig(opt)
	if err != nil {
		return r, fmt.Errorf("hysteria config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink = hysteria.GenerateHysteriaURL(opt)

	configsDir := installer.GetConfigDir("hysteria")
	_ = os.MkdirAll(configsDir, 0755)
	all := hysteria.GenerateAllSNIUrls(opt)
	_ = os.WriteFile(filepath.Join(configsDir, "sharing-links-all.txt"),
		[]byte("# Tunnel — sharing links (all hostnames)\n"+strings.Join(all, "\n\n")), 0644)

	qrPath := filepath.Join(configsDir, "qr-hysteria.png")
	if err := utils.SaveQRCodePNG(qrPath, r.SharingLink, 320); err != nil && log != nil {
		log.Warn("provision: qr png", "err", err)
	}

	if err := CopyFileIfDifferent(log, srv, serverOut); err != nil {
		return r, err
	}
	if err := CopyFileIfDifferent(log, cli, clientOut); err != nil {
		return r, err
	}
	r.ListenPort = opt.Port
	return r, nil
}

func provisionWireguard(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = 51820
	}
	ApplyPortAllocation(log, &opt.Port, "udp", "TunnelBypass-WireGuard")

	srv, cli, err := wireguard.GenerateWireGuardConfig(opt)
	if err != nil {
		return r, fmt.Errorf("wireguard config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli

	if u, err := wireguard.GenerateClientWGURL(cli, opt.Sni); err == nil {
		r.SharingLink = u
		configsDir := installer.GetConfigDir("wireguard")
		_ = os.MkdirAll(configsDir, 0755)
		_ = os.WriteFile(filepath.Join(configsDir, "sharing-links-all.txt"), []byte(u), 0644)
		wgQR := filepath.Join(configsDir, "qr-wireguard.png")
		if err := utils.SaveQRCodePNG(wgQR, u, 320); err != nil && log != nil {
			log.Warn("provision: qr png", "err", err)
		}
		if _, err := wireguard.GenerateThroneProfile(cli, opt.Sni); err != nil && log != nil {
			log.Debug("provision: throne profile skipped", "err", err)
		}
	}

	if err := CopyFileIfDifferent(log, srv, serverOut); err != nil {
		return r, err
	}
	if err := CopyFileIfDifferent(log, cli, clientOut); err != nil {
		return r, err
	}
	r.ListenPort = opt.Port
	return r, nil
}

func provisionSSH(log *slog.Logger, opt types.ConfigOptions) (Result, error) {
	var r Result
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	// Use SSHBackendPort if provided (from --ssh-port flag), otherwise use Port, use dynamic allocation if both are 0
	if opt.SSHBackendPort > 0 {
		opt.Port = opt.SSHBackendPort
	}
	if opt.Port <= 0 {
		// Use dynamic port allocation
		opt.Port = installer.EnsureFreeTCPPort(0, "ssh")
	}
	opt.SSHBackendPort = opt.Port // Ensure SSHBackendPort is set for later use
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-SSH")

	if strings.TrimSpace(opt.SSHUser) == "" {
		opt.SSHUser = "tunnelbypass"
	}
	if strings.TrimSpace(opt.SSHPassword) == "" {
		opt.SSHPassword = installer.ReadOrCreateEmbedSSHPassword()
	}
	if strings.TrimSpace(opt.SSHWelcomeMessage) == "" {
		opt.SSHWelcomeMessage = fmt.Sprintf("Welcome to TunnelBypass SSH Tunnel.\nAuthorized users only.\nUser: %s", opt.SSHUser)
	}

	if err := installer.EnsureWindowsUser(opt.SSHUser, opt.SSHPassword, true, false); err != nil && log != nil {
		log.Warn("provision: windows user", "err", err)
	}
	if err := ensureSSHBackend(log, &opt); err != nil {
		return r, err
	}

	p, err := tbssh.GenerateSSHConfig(opt)
	if err != nil {
		return r, err
	}
	r.InstructionPath = p
	r.ListenPort = opt.Port
	r.SSHPort = opt.SSHBackendPort
	return r, nil
}

func provisionTLS(log *slog.Logger, opt types.ConfigOptions) (Result, error) {
	var r Result
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	if opt.Port == 0 {
		opt.Port = 443
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-SSL")

	if strings.TrimSpace(opt.SSHUser) == "" {
		opt.SSHUser = "tunnelbypass"
	}
	if strings.TrimSpace(opt.SSHPassword) == "" {
		opt.SSHPassword = installer.ReadOrCreateEmbedSSHPassword()
	}
	if strings.TrimSpace(opt.SSHWelcomeMessage) == "" {
		opt.SSHWelcomeMessage = fmt.Sprintf("Welcome to TunnelBypass SSH Tunnel over SSL.\nAuthorized users only.\nUser: %s", opt.SSHUser)
	}

	if err := installer.EnsureWindowsUser(opt.SSHUser, opt.SSHPassword, true, false); err != nil && log != nil {
		log.Warn("provision: windows user", "err", err)
	}
	if err := ensureSSHBackend(log, &opt); err != nil {
		return r, err
	}

	if err := ensurePortableStunnelArtifacts(log, opt); err != nil {
		return r, err
	}

	p, err := tbssh.GenerateSSLConfig(opt)
	if err != nil {
		return r, err
	}
	r.InstructionPath = p
	r.ListenPort = opt.Port
	r.SSHPort = opt.SSHBackendPort
	return r, nil
}

func provisionWSS(log *slog.Logger, opt types.ConfigOptions) (Result, error) {
	var r Result
	installer.SetSSHServerForwarder(false)
	defer installer.SetSSHServerForwarder(true)

	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	if opt.Port == 0 {
		opt.Port = 443
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-WSS")

	if strings.TrimSpace(opt.SSHUser) == "" {
		opt.SSHUser = "tunnelbypass"
	}
	if strings.TrimSpace(opt.SSHPassword) == "" {
		opt.SSHPassword = installer.ReadOrCreateEmbedSSHPassword()
	}
	if strings.TrimSpace(opt.SSHWelcomeMessage) == "" {
		opt.SSHWelcomeMessage = fmt.Sprintf("Welcome to TunnelBypass SSH Tunnel over WSS.\nAuthorized users only.\nUser: %s", opt.SSHUser)
	}

	if err := installer.EnsureWindowsUser(opt.SSHUser, opt.SSHPassword, true, false); err != nil && log != nil {
		log.Warn("provision: windows user", "err", err)
	}
	if err := ensureSSHBackend(log, &opt); err != nil {
		return r, err
	}

	if err := ensurePortableWssCerts(log, opt); err != nil {
		return r, err
	}

	p, err := tbssh.GenerateWSSConfig(opt)
	if err != nil {
		return r, err
	}
	r.InstructionPath = p
	r.ListenPort = opt.Port
	r.SSHPort = opt.SSHBackendPort
	return r, nil
}

func ensurePortableWssCerts(log *slog.Logger, opt types.ConfigOptions) error {
	cfgDir := installer.GetConfigDir("wstunnel")
	_ = os.MkdirAll(cfgDir, 0755)
	certPath := filepath.Join(cfgDir, "wss-cert.pem")
	keyPath := filepath.Join(cfgDir, "wss-key.pem")
	host := strings.TrimSpace(opt.Sni)
	if host == "" {
		host = "localhost"
	}
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return nil
		}
	}
	if err := installer.EnsureSelfSignedCert(certPath, keyPath, host); err != nil {
		return fmt.Errorf("wss tls cert: %w", err)
	}
	if log != nil {
		log.Info("provision: wrote wss tls cert", "cert", certPath)
	}
	return nil
}

func ensurePortableStunnelArtifacts(log *slog.Logger, opt types.ConfigOptions) error {
	cfgDir := installer.GetConfigDir("stunnel")
	_ = os.MkdirAll(cfgDir, 0755)
	certPath := filepath.Join(cfgDir, "ssl-cert.pem")
	keyPath := filepath.Join(cfgDir, "ssl-key.pem")
	host := strings.TrimSpace(opt.Sni)
	if host == "" {
		host = "localhost"
	}
	if err := installer.EnsureSelfSignedCert(certPath, keyPath, host); err != nil {
		return fmt.Errorf("stunnel tls cert: %w", err)
	}
	sshBack := installer.GetSSHBackendPort()
	conf := filepath.Join(cfgDir, "stunnel-server.conf")
	if err := installer.WriteStunnelServerConfig(conf, opt.Port, sshBack, certPath, keyPath); err != nil {
		return fmt.Errorf("stunnel server conf: %w", err)
	}
	if log != nil {
		log.Info("provision: wrote stunnel server config", "path", conf)
	}
	return nil
}

func NeedsProvision(transport string) bool {
	t := strings.ToLower(strings.TrimSpace(transport))
	switch t {
	case "reality", "vless":
		p := filepath.Join(installer.GetConfigDir("vless"), "server.json")
		_, err := os.Stat(p)
		return err != nil
	case "vless-ws":
		p := filepath.Join(installer.GetConfigDir("vless-ws"), "server.json")
		_, err := os.Stat(p)
		return err != nil
	case "hysteria":
		p := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		_, err := os.Stat(p)
		return err != nil
	case "wireguard":
		p := filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf")
		_, err := os.Stat(p)
		return err != nil
	case "wss":
		c := filepath.Join(installer.GetConfigDir("wstunnel"), "wss-cert.pem")
		k := filepath.Join(installer.GetConfigDir("wstunnel"), "wss-key.pem")
		_, e1 := os.Stat(c)
		_, e2 := os.Stat(k)
		return e1 != nil || e2 != nil
	case "tls":
		p := filepath.Join(installer.GetConfigDir("stunnel"), "stunnel-server.conf")
		_, err := os.Stat(p)
		return err != nil
	case "ssh":
		p := filepath.Join(installer.GetConfigDir("ssh"), "ssh_tunnel_instructions.txt")
		_, err := os.Stat(p)
		return err != nil
	default:
		return false
	}
}

func ensureSSHBackend(log *slog.Logger, opt *types.ConfigOptions) error {
	if err := installer.EnsureSSHServerWithAuth(opt.SSHUser, opt.SSHPassword); err != nil {
		if log != nil {
			log.Warn("provision: ssh server ensure", "err", err)
		}
		return err
	}
	if installer.SSHEmbedActive() {
		opt.SSHBackendPort = installer.GetSSHBackendPort()
	} else {
		// Try to get port from saved config, otherwise use system SSH default
		portCfg, _ := installer.LoadSSHPortConfig()
		if portCfg.InternalPort > 0 {
			opt.SSHBackendPort = portCfg.InternalPort
		} else {
			// Use configured SSHBackendPort if already set, else dynamic allocation
			if opt.SSHBackendPort <= 0 {
				opt.SSHBackendPort = installer.GetSSHBackendPort()
			}
		}
	}
	return nil
}
