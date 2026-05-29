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
	"tunnelbypass/core/transports/mdns"
	tbss "tunnelbypass/core/transports/shadowsocks"
	tbssh "tunnelbypass/core/transports/ssh"
	"tunnelbypass/core/transports/sshpayload"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/transports/wireguard"
	"tunnelbypass/core/transports/xdns"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/destprobe"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
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

func ensureRealityDest(log *slog.Logger, opt *types.ConfigOptions) {
	if strings.TrimSpace(opt.RealityDest) != "" {
		if host, addr := host_catalog.NormalizeRealityDestInput(opt.RealityDest); addr != "" {
			opt.RealityDest = addr
			if strings.TrimSpace(opt.RealityDestHost) == "" {
				opt.RealityDestHost = host
			}
		}
		if strings.TrimSpace(opt.RealityDestHost) == "" {
			opt.RealityDestHost = host_catalog.HostFromRealityDestAddress(opt.RealityDest)
		}
		return
	}
	if sni := host_catalog.NormalizeHost(opt.Sni); sni != "" {
		if res, err := destprobe.ProbeHostWithTimeout(sni, destprobe.DefaultTimeout); err == nil {
			opt.RealityDest = res.Address
			opt.RealityDestHost = res.Host
			if log != nil {
				log.Info("provision: using selected SNI as REALITY dest", "host", res.Host, "dest", res.Address, "status", res.StatusCode)
			}
			return
		} else if log != nil {
			log.Warn("provision: selected SNI is not safe as REALITY dest; falling back to config/default dest", "host", sni, "err", err)
		}
	}
	opt.RealityDest = host_catalog.DefaultRealityDestAddressForConfig(opt.RealityDestHost, opt.RealityDestExtraHosts)
	if strings.TrimSpace(opt.RealityDestHost) == "" {
		opt.RealityDestHost = host_catalog.HostFromRealityDestAddress(opt.RealityDest)
	}
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
		opt.Port = types.DefaultTLSTunnelListenPort
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
	ensureRealityDest(log, &opt)

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

func provisionSSH_TLS(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = "ssh-tls"
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = types.DefaultSSHTLSDirectListenPort
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-SSH-TLS")

	if strings.TrimSpace(opt.Sni) == "" {
		return r, fmt.Errorf("ssh-tls: tunnel hostname (SNI) is required")
	}
	opt.UUID = NormalizeUUID(opt.UUID)

	if strings.TrimSpace(opt.SSHUser) == "" {
		opt.SSHUser = "tunnelbypass"
	}
	if strings.TrimSpace(opt.SSHPassword) == "" {
		opt.SSHPassword = installer.ReadOrCreateEmbedSSHPassword()
	}
	if strings.TrimSpace(opt.SSHWelcomeMessage) == "" {
		opt.SSHWelcomeMessage = fmt.Sprintf("Welcome to TunnelBypass SSH + TLS (direct).\nAuthorized users only.\nUser: %s", opt.SSHUser)
	}

	if err := installer.EnsureWindowsUser(opt.SSHUser, opt.SSHPassword, true, false); err != nil && log != nil {
		log.Warn("provision: windows user", "err", err)
	}
	if err := ensureSSHBackend(log, &opt); err != nil {
		return r, err
	}

	destPort := opt.SSHBackendPort
	if destPort <= 0 {
		destPort = installer.GetSSHBackendPort()
	}
	if destPort <= 0 {
		destPort = 22
	}
	sshDest := fmt.Sprintf("127.0.0.1:%d", destPort)

	srv, err := vless.GenerateVlessSSHDirectTLSServerConfig(opt, sshDest)
	if err != nil {
		return r, fmt.Errorf("ssh-tls server config: %w", err)
	}
	cli, err := vless.GenerateVlessSSHDirectTLSClientConfig(opt)
	if err != nil {
		return r, fmt.Errorf("ssh-tls client config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink = vless.GenerateVlessSSHDirectTLSURL(opt)
	r.SSHPort = destPort

	cfgDir := installer.GetConfigDir("ssh-tls")
	_ = os.MkdirAll(cfgDir, 0755)
	_ = os.WriteFile(filepath.Join(cfgDir, "sharing-link.txt"),
		[]byte("# Tunnel — SSH + TLS (direct) — optional VLESS link; SSH clients use TLS to this port\n"+r.SharingLink+"\n"), 0644)

	if err := CopyFileIfDifferent(log, srv, serverOut); err != nil {
		return r, err
	}
	if err := CopyFileIfDifferent(log, cli, clientOut); err != nil {
		return r, err
	}
	r.ListenPort = opt.Port
	return r, nil
}

func provisionVlessGRPC(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = "vless-grpc"
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = types.DefaultTLSTunnelListenPort
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-VLESS-GRPC")

	if strings.TrimSpace(opt.Sni) == "" {
		return r, fmt.Errorf("vless-grpc: tunnel hostname (SNI) is required")
	}
	opt.UUID = NormalizeUUID(opt.UUID)
	if opt.PrivateKey == "" || opt.PublicKey == "" {
		priv, pub, err := utils.GenerateX25519Keys()
		if err != nil {
			return r, fmt.Errorf("vless-grpc reality keys: %w", err)
		}
		opt.PrivateKey, opt.PublicKey = priv, pub
	}
	if len(opt.ShortIds) == 0 {
		opt.ShortIds = utils.GenerateRandomShortIds()
	}
	ensureRealityDest(log, &opt)

	srv, err := vless.GenerateVlessGRPCServerConfig(opt)
	if err != nil {
		return r, fmt.Errorf("vless-grpc server config: %w", err)
	}
	cli, err := vless.GenerateVlessGRPCClientConfig(opt)
	if err != nil {
		return r, fmt.Errorf("vless-grpc client config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink = vless.GenerateVlessGRPCURL(opt)

	configsDir := installer.GetConfigDir("vless-grpc")
	_ = os.MkdirAll(configsDir, 0755)
	_ = os.WriteFile(filepath.Join(configsDir, "sharing-link.txt"),
		[]byte("# Tunnel — VLESS REALITY + gRPC (Elite / DPI-resistant)\n"+r.SharingLink+"\n"), 0644)

	qrPath := filepath.Join(configsDir, "qr-vless-grpc.png")
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
		opt.Port = types.DefaultTLSTunnelListenPort
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
		opt.Port = types.DefaultHysteriaListenPort
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
		opt.Port = types.DefaultWireGuardListenPort
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
		opt.Port = types.DefaultTLSTunnelListenPort
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
		opt.Port = types.DefaultTLSTunnelListenPort
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

func provisionSSHPayload(log *slog.Logger, opt types.ConfigOptions) (Result, error) {
	var r Result
	installer.SetSSHServerForwarder(false)
	defer installer.SetSSHServerForwarder(true)

	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	if opt.Port == 0 {
		opt.Port = types.DefaultSSHPayloadListenPort
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-SSH-Payload")

	if strings.TrimSpace(opt.SSHUser) == "" {
		opt.SSHUser = "tunnelbypass"
	}
	if strings.TrimSpace(opt.SSHPassword) == "" {
		opt.SSHPassword = installer.ReadOrCreateEmbedSSHPassword()
	}
	if strings.TrimSpace(opt.SSHWelcomeMessage) == "" {
		opt.SSHWelcomeMessage = fmt.Sprintf("Welcome to TunnelBypass SSH Payload.\nAuthorized users only.\nUser: %s", opt.SSHUser)
	}

	if err := installer.EnsureWindowsUser(opt.SSHUser, opt.SSHPassword, true, false); err != nil && log != nil {
		log.Warn("provision: windows user", "err", err)
	}
	if err := ensureSSHBackend(log, &opt); err != nil {
		return r, err
	}

	payloadPath := sshpayload.NormalizePayloadPath(opt.PayloadPath)
	if payloadPath == "" {
		payloadPath = sshpayload.GeneratePayloadPath()
	}
	cfg := sshpayload.Config{
		Transport:      "ssh-payload",
		ServerAddr:     opt.ServerAddr,
		ListenPort:     opt.Port,
		PayloadPath:    payloadPath,
		SSHBackendPort: opt.SSHBackendPort,
		SSHUser:        opt.SSHUser,
		SSHPassword:    opt.SSHPassword,
	}.WithDefaults()

	cfgDir := installer.GetConfigDir("ssh-payload")
	configPath := filepath.Join(cfgDir, "server.json")
	if err := sshpayload.WriteConfig(configPath, cfg); err != nil {
		return r, err
	}
	instructionsPath := filepath.Join(cfgDir, "ssh_payload_instructions.txt")
	if err := sshpayload.WriteInstructions(instructionsPath, cfg); err != nil {
		return r, err
	}

	r.Transport = "ssh-payload"
	r.ServerConfigPath = configPath
	r.InstructionPath = instructionsPath
	r.ListenPort = cfg.ListenPort
	r.SSHPort = cfg.SSHBackendPort
	r.PayloadPath = cfg.PayloadPath
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

func provisionXDNS(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = types.TransportXDNS
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = types.DefaultXDNSListenPort
	}
	ApplyPortAllocation(log, &opt.Port, "udp", "TunnelBypass-XDNS")

	opt.UUID = NormalizeUUID(opt.UUID)
	if strings.TrimSpace(opt.KCPSeed) == "" {
		opt.KCPSeed = utils.GenerateRandomString(32)
	}
	if opt.KCPMTU == 0 {
		opt.KCPMTU = 1350
	}
	if opt.KCPTTI == 0 {
		opt.KCPTTI = 20
	}
	if strings.TrimSpace(opt.KCPHeaderType) == "" {
		opt.KCPHeaderType = "none"
	}

	srv, err := xdns.GenerateServerConfig(opt)
	if err != nil {
		return r, fmt.Errorf("xdns server config: %w", err)
	}
	cli, err := xdns.GenerateClientConfig(opt)
	if err != nil {
		return r, fmt.Errorf("xdns client config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink = xdns.GenerateVlessURL(opt, opt.KCPSeed)

	configsDir := installer.GetConfigDir("xdns")
	_ = os.MkdirAll(configsDir, 0755)
	_ = os.WriteFile(filepath.Join(configsDir, "sharing-link.txt"),
		[]byte("# Tunnel — XDNS (VLESS + mKCP + DNS)\n"+r.SharingLink+"\n"), 0644)

	qrPath := filepath.Join(configsDir, "qr-xdns.png")
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

func provisionMDNS(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = types.TransportMDNS
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	ensureHost(&opt)
	if opt.Port == 0 {
		opt.Port = types.DefaultMDNSListenPort
	}
	ApplyPortAllocation(log, &opt.Port, "udp", "TunnelBypass-MasterDnsVPN")

	// Validate domain
	if strings.TrimSpace(opt.MDNSDomain) == "" {
		opt.MDNSDomain = opt.Host
	}
	if strings.TrimSpace(opt.MDNSDomain) == "" {
		opt.MDNSDomain = opt.Sni
	}
	if strings.TrimSpace(opt.MDNSDomain) == "" {
		return r, fmt.Errorf("mdns: tunnel domain is required (set --mdns-domain, --sni, or --host)")
	}

	// Generate or use provided encryption key
	if strings.TrimSpace(opt.MDNSEncryptionKey) == "" {
		opt.MDNSEncryptionKey = mdns.GenerateEncryptionKey()
	}
	if opt.MDNSEncryptionMethod < 0 || opt.MDNSEncryptionMethod > 5 {
		opt.MDNSEncryptionMethod = 3 // AES-128-GCM
	}

	resolvers := opt.MDNSResolvers
	if len(resolvers) == 0 {
		resolvers = mdns.DefaultResolvers
	}

	srv, err := mdns.GenerateServerConfig(opt, opt.MDNSEncryptionKey)
	if err != nil {
		return r, fmt.Errorf("mdns server config: %w", err)
	}
	cli, resolversPath, err := mdns.GenerateClientConfig(opt, opt.MDNSEncryptionKey, resolvers)
	if err != nil {
		return r, fmt.Errorf("mdns client config: %w", err)
	}

	_ = mdns.GenerateSharingPackage(opt, opt.MDNSEncryptionKey, cli, resolversPath)

	configsDir := installer.GetConfigDir("mdns")
	_ = os.MkdirAll(configsDir, 0755)

	if err := CopyFileIfDifferent(log, srv, serverOut); err != nil {
		return r, err
	}
	if err := CopyFileIfDifferent(log, cli, clientOut); err != nil {
		return r, err
	}

	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.ListenPort = opt.Port

	// Print NS delegation guide
	mdns.PrintNSDelegationGuide(opt.ServerAddr, opt.MDNSDomain)

	return r, nil
}

func provisionShadowsocks(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	var r Result
	opt.Transport = "shadowsocks"
	if opt.SSPlugin == "v2ray-plugin" {
		opt.Transport = "shadowsocks-ws"
	}
	opt.ServerAddr = ResolveServerAddr(opt.ServerAddr)
	if opt.Port == 0 {
		if opt.SSPlugin == "v2ray-plugin" {
			opt.Port = types.DefaultShadowsocksV2rayListenPort
		} else {
			opt.Port = types.DefaultShadowsocksListenPort
		}
	}
	ApplyPortAllocation(log, &opt.Port, "tcp", "TunnelBypass-Shadowsocks")

	opt.UUID = NormalizeUUID(opt.UUID)

	if opt.SSPlugin == "v2ray-plugin" {
		if _, err := installer.EnsureBinary("v2ray-plugin"); err != nil {
			return r, fmt.Errorf("failed to download v2ray-plugin: %w", err)
		}
		configsDir := installer.GetConfigDir("shadowsocks")
		_ = os.MkdirAll(configsDir, 0755)
		certPath := filepath.Join(configsDir, "cert.crt")
		keyPath := filepath.Join(configsDir, "private.key")
		sniList := host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs)
		commonName := "localhost"
		if len(sniList) > 0 && sniList[0] != "" {
			commonName = sniList[0]
		}
		if err := installer.EnsureSelfSignedCertWithSAN(certPath, keyPath, commonName, sniList); err != nil {
			return r, fmt.Errorf("shadowsocks v2ray-plugin cert: %w", err)
		}
		raw, err := tbss.V2rayPluginCertRawFromPEMFile(certPath)
		if err != nil {
			return r, fmt.Errorf("shadowsocks v2ray-plugin client cert raw: %w", err)
		}
		opt.SSV2rayClientCertRaw = raw
		if strings.TrimSpace(opt.SSV2rayPluginWSPath) == "" {
			opt.SSV2rayPluginWSPath = tbss.RandomV2rayPluginWSPath()
		}
		wsPath := tbss.NormalizeV2rayWSPath(opt.SSV2rayPluginWSPath)
		// v2ray-plugin parses plugin_opts with escape rules; '\' mangles Windows paths (C:TunnelBypass...).
		// Forward slashes are accepted on Windows; see also shadowsocks.normalizeV2rayPluginCertKeyPaths.
		opt.SSPluginOpts = fmt.Sprintf("server;tls;host=%s;path=%s;cert=%s;key=%s", commonName, wsPath, filepath.ToSlash(certPath), filepath.ToSlash(keyPath))
	}

	srv, cli, err := tbss.GenerateShadowsocksConfig(opt)
	if err != nil {
		return r, fmt.Errorf("shadowsocks config: %w", err)
	}
	r.ServerConfigPath = srv
	r.ClientConfigPath = cli
	r.SharingLink, err = tbss.GenerateShadowsocksURL(opt)
	if err != nil {
		return r, fmt.Errorf("shadowsocks sharing link: %w", err)
	}

	configsDir := installer.GetConfigDir("shadowsocks")
	_ = os.MkdirAll(configsDir, 0755)
	all, err := tbss.GenerateAllSNIUrls(opt)
	if err != nil {
		return r, fmt.Errorf("shadowsocks all sharing links: %w", err)
	}
	_ = os.WriteFile(filepath.Join(configsDir, "sharing-links-all.txt"),
		[]byte("# Tunnel — sharing links (all hostnames)\n"+strings.Join(all, "\n\n")), 0644)

	qrPath := filepath.Join(configsDir, "qr-shadowsocks.png")
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
	case "vless-grpc":
		p := filepath.Join(installer.GetConfigDir("vless-grpc"), "server.json")
		_, err := os.Stat(p)
		return err != nil
	case "ssh-tls":
		p := filepath.Join(installer.GetConfigDir("ssh-tls"), "server.json")
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
	case "shadowsocks", "ss", "shadowsocks-ws":
		p := filepath.Join(installer.GetConfigDir("shadowsocks"), "server.json")
		_, err := os.Stat(p)
		return err != nil
	case "xdns":
		p := filepath.Join(installer.GetConfigDir("xdns"), "server.json")
		_, err := os.Stat(p)
		return err != nil
	case "mdns":
		p := filepath.Join(installer.GetConfigDir("mdns"), "server_config.toml")
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
