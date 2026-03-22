package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
)

func GenerateSSHConfig(opt types.ConfigOptions) (string, error) {
	configsDir := installer.GetConfigDir("ssh")
	_ = os.MkdirAll(configsDir, 0755)

	sshUser := opt.SSHUser
	if sshUser == "" {
		sshUser = "user"
	}
	sshPass := opt.SSHPassword
	sshWelcome := opt.SSHWelcomeMessage
	if sshWelcome == "" {
		sshWelcome = "Welcome to TunnelBypass SSH Tunnel.\nAuthorized users only."
	}

	bannerCopyFile := filepath.Join(configsDir, "ssh_server_banner.txt")
	_ = os.WriteFile(bannerCopyFile, []byte(sshWelcome+"\n"), 0644)

	installer.BestEffortConfigureSSHBanner(sshWelcome)
	
	sshPort := opt.Port
	remoteSSH := opt.SSHBackendPort
	if remoteSSH <= 0 {
		remoteSSH = 22
	}

	config := fmt.Sprintf(`# SSH Tunnel — Credentials & Client Commands
# Server IP : %s
# SSH Port   : %d
# Username   : %s
# Password   : %s
#
# SOCKS5 proxy (dynamic) using sshpass:
#   sshpass -p '<PASS>' ssh -D 1080 -N -f -p %d %s@%s
#
# If you don't have sshpass:
#   ssh -D 1080 -N -p %d %s@%s
#
# SSH Server Welcome Message (Banner / MOTD):
#   Banner file to use: %s
#   (we generated a copy here too): %s
#
# UDP support (TunnelBypass UDPGW, badvpn-compatible protocol; bundled with SSH automatically):
#   UDPGW endpoint: 127.0.0.1:7300 (over SSH tunnel); portable: tunnelbypass run --portable ssh
#   Mode via TB_UDPGW_MODE / TB_UDPGW_BINARY
#
`, opt.ServerAddr, sshPort, sshUser, sshPass, sshPort, sshUser, opt.ServerAddr, sshPort, sshUser, opt.ServerAddr, installer.GetSystemSSHBannerPath(), bannerCopyFile)

	fileName := "ssh_tunnel_instructions.txt"
	targetPath := filepath.Join(configsDir, fileName)
	return targetPath, os.WriteFile(targetPath, []byte(config), 0644)
}

func GenerateSSLConfig(opt types.ConfigOptions) (string, error) {
	configsDir := installer.GetConfigDir("stunnel")
	_ = os.MkdirAll(configsDir, 0755)

	sshUser := opt.SSHUser
	if sshUser == "" {
		sshUser = "user"
	}
	sshPass := opt.SSHPassword
	sshWelcome := opt.SSHWelcomeMessage
	if sshWelcome == "" {
		sshWelcome = "Welcome to TunnelBypass SSH Tunnel over SSL.\nAuthorized users only."
	}

	installer.BestEffortConfigureSSHBanner(sshWelcome)

	remoteSSH := opt.SSHBackendPort
	if remoteSSH <= 0 {
		remoteSSH = 22
	}

	localStunnelPort := 2222
	stunnelClientConf := filepath.Join(configsDir, "stunnel-client.conf")
	_ = installer.WriteStunnelClientConfig(stunnelClientConf, opt.ServerAddr, opt.Port, localStunnelPort, opt.Sni)

	sniForUi := opt.Sni
	if strings.TrimSpace(sniForUi) == "" {
		sniForUi = "(empty)"
	}
	sniLine := "# (no SNI)"
	if strings.TrimSpace(opt.Sni) != "" {
		sniLine = "sni = " + opt.Sni
	}

	firstLine := strings.SplitN(sshWelcome, "\n", 2)[0]
	config := fmt.Sprintf(`# TunnelBypass SSL (stunnel) - Quick Instructions
Server:   %s:%d
SNI:      %s
User:     %s
Password: %s
# Server-side SSH port (stunnel target): %d

RUN THIS FIRST (IMPORTANT):
stunnel %s

THEN RUN SSH SOCKS:
ssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p %d %s@127.0.0.1

Optional UDPGW port: 7300
Welcome: %s

Reference stunnel client profile:
client = yes
[ssh-ssl-client]
accept = 127.0.0.1:%d
connect = %s:%d
%s
verify = 0
`, opt.ServerAddr, opt.Port, sniForUi, sshUser, sshPass, remoteSSH,
		stunnelClientConf,
		localStunnelPort, sshUser,
		firstLine,
		localStunnelPort, opt.ServerAddr, opt.Port, sniLine)

	instructionsPath := filepath.Join(configsDir, "ssl_tunnel_instructions.txt")
	return instructionsPath, os.WriteFile(instructionsPath, []byte(config), 0644)
}

func GenerateWSSConfig(opt types.ConfigOptions) (string, error) {
	configsDir := installer.GetConfigDir("wstunnel")
	_ = os.MkdirAll(configsDir, 0755)

	sshUser := opt.SSHUser
	if sshUser == "" {
		sshUser = "user"
	}
	sshPass := opt.SSHPassword
	sshWelcome := opt.SSHWelcomeMessage
	if sshWelcome == "" {
		sshWelcome = "Welcome to TunnelBypass SSH Tunnel over WSS.\nAuthorized users only."
	}

	installer.BestEffortConfigureSSHBanner(sshWelcome)

	localPort := 2222
	remoteSSH := opt.SSHBackendPort
	if remoteSSH <= 0 {
		remoteSSH = 22
	}
	// wstunnel v10: -L tcp://localPort:remoteHost:remotePort
	wstunnelCommand := fmt.Sprintf("wstunnel client -L tcp://127.0.0.1:%d:127.0.0.1:%d wss://%s:%d", localPort, remoteSSH, opt.ServerAddr, opt.Port)
	if opt.Sni != "" {
		// Use -H "Host: <SNI>" as recommended for stealth (fake SNI / Host Header)
		wstunnelCommand += fmt.Sprintf(" -H \"Host: %s\"", opt.Sni)
		// Also keep --tls-sni-override for actual TLS SNI if needed, 
		// but -H is often what's used for the 'Fake' part.
		wstunnelCommand += fmt.Sprintf(" --tls-sni-override %s", opt.Sni)
	}

	firstLine := strings.SplitN(sshWelcome, "\n", 2)[0]
	sniForUi := opt.Sni
	if strings.TrimSpace(sniForUi) == "" {
		sniForUi = "(empty)"
	}
	config := fmt.Sprintf(`# TunnelBypass WSS (wstunnel) - Quick Instructions
Server:   %s:%d
SNI/Host: %s
User:     %s
Password: %s
# Server-side SSH port (wstunnel --restrict-to target): %d

RUN THIS FIRST (IMPORTANT) to start the WebSocket tunnel:
%s

THEN CONNECT VIA SSH (SOCKS5 Proxy):
ssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p %d %s@127.0.0.1

HARDENING TIP (SERVER):
For maximum stealth, ensure your server-side SSH only listens on 127.0.0.1 
and that Port 22 is blocked in your firewall for external traffic.

Optional UDPGW port: 7300
Welcome: %s
`, opt.ServerAddr, opt.Port, sniForUi, sshUser, sshPass,
		remoteSSH,
		wstunnelCommand,
		localPort, sshUser,
		firstLine)

	instructionsPath := filepath.Join(configsDir, "wss_tunnel_instructions.txt")
	return instructionsPath, os.WriteFile(instructionsPath, []byte(config), 0644)
}
