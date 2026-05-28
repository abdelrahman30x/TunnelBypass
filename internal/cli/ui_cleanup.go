package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/hysteria"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/transports/wireguard"
	"tunnelbypass/internal/utils"
)

type installedTransport string

const (
	transportXray          installedTransport = "xray"
	transportVLESSWS       installedTransport = "vless-ws"
	transportHysteria      installedTransport = "hysteria"
	transportWireGuard     installedTransport = "wireguard"
	transportSSH           installedTransport = "ssh"
	transportSSL           installedTransport = "ssl"
	transportWSS           installedTransport = "wss"
	transportSSHTLS        installedTransport = "ssh-tls"
	transportGRPC          installedTransport = "grpc"
	transportUDPGW         installedTransport = "udpgw"
	transportShadowsocks   installedTransport = "shadowsocks"
	transportShadowsocksWS installedTransport = "shadowsocks-ws"
	transportXDNS          installedTransport = "xdns"
	transportMDNS          installedTransport = "mdns"
	transportUnknown       installedTransport = "unknown"
)

func detectInstalledTransport(serviceName string) installedTransport {
	s := strings.ToLower(serviceName)
	switch {
	case strings.Contains(s, "ssh-tls"):
		return transportSSHTLS
	case strings.Contains(s, "ssh-forwarder"):
		return transportSSH
	case strings.Contains(s, "vless-ws"):
		return transportVLESSWS
	case strings.Contains(s, "grpc"):
		return transportGRPC
	case strings.Contains(s, "hysteria"):
		return transportHysteria
	case strings.Contains(s, "wg-quick"):
		return transportWireGuard
	case strings.Contains(s, "wireguard"):
		return transportWireGuard
	case strings.Contains(s, "ssh"):
		return transportSSH
	case strings.Contains(s, "wss"):
		return transportWSS
	case strings.Contains(s, "ssl"):
		return transportSSL
	case strings.Contains(s, "shadowsocks-ws"):
		return transportShadowsocksWS
	case strings.Contains(s, "shadowsocks"):
		return transportShadowsocks
	case strings.Contains(s, "udpgw"):
		return transportUDPGW
	case strings.Contains(s, "xdns"):
		return transportXDNS
	case strings.Contains(s, "mdns"), strings.Contains(s, "masterdnsvpn"):
		return transportMDNS
	case strings.Contains(s, "vless"), strings.Contains(s, "udp"), strings.Contains(s, "tunnel"):
		return transportXray
	default:
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")); err == nil {
			return transportHysteria
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf")); err == nil {
			return transportWireGuard
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("ssh-tls"), "server.json")); err == nil {
			return transportSSHTLS
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("vless"), "server.json")); err == nil {
			return transportXray
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("ssh"), "ssh_tunnel_instructions.txt")); err == nil {
			return transportSSH
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("wstunnel"), "wss_tunnel_instructions.txt")); err == nil {
			return transportWSS
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("stunnel"), "stunnel_server.conf")); err == nil {
			return transportSSL
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("shadowsocks"), "server.json")); err == nil {
			return transportShadowsocks
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("xdns"), "server.json")); err == nil {
			return transportXDNS
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("mdns"), "server_config.toml")); err == nil {
			return transportMDNS
		}
		return transportUnknown
	}
}

func freshSetupCleanup(serviceName string) error {
	return uninstallServiceAndFiles(serviceName, true)
}

func uninstallServiceAndFiles(serviceName string, includeCompanions bool) error {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return nil
	}
	names := []string{serviceName}
	if includeCompanions {
		names = append(names, removableCompanionServices(serviceName)...)
	}
	names = dedupeServiceNames(names)

	var errs []error
	for _, name := range names {
		tr := detectInstalledTransport(name)
		if err := uninstallTransportService(tr, name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", prettyServiceName(name), err))
		}
		removePortAllocState(name)
		cleanupArtifactsForTransport(tr, name)
	}
	runLinuxRollbackIfNoTunnelBypassServices(serviceName)
	return errors.Join(errs...)
}

func uninstallTransportService(tr installedTransport, serviceName string) error {
	switch tr {
	case transportHysteria:
		return hysteria.UninstallHysteriaService(serviceName)
	case transportWireGuard:
		return wireguard.UninstallWireGuardService(serviceName)
	case transportXray, transportVLESSWS, transportGRPC, transportSSHTLS, transportXDNS:
		return vless.UninstallXrayService(serviceName)
	default:
		installer.UninstallService(serviceName)
		return nil
	}
}

func dedupeServiceNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

func sshBackedTransport(tr installedTransport) bool {
	switch tr {
	case transportSSH, transportSSL, transportWSS, transportSSHTLS:
		return true
	default:
		return false
	}
}

func removableCompanionServices(primary string) []string {
	tr := detectInstalledTransport(primary)
	if !sshBackedTransport(tr) {
		return nil
	}
	if tr != transportSSH && otherSSHFrontendsInstalled(primary) {
		fmt.Fprintf(os.Stderr, "[*] Other SSH-backed TunnelBypass services remain; keeping shared SSH/UDPGW companions.\n")
		return nil
	}
	return []string{
		"TunnelBypass-SSH",
		"TunnelBypass-SSH-Forwarder",
		installer.UDPGWServiceName,
	}
}

func otherSSHFrontendsInstalled(primary string) bool {
	for _, name := range []string{"TunnelBypass-WSS", "TunnelBypass-SSL", "TunnelBypass-SSH-TLS"} {
		if strings.EqualFold(name, primary) {
			continue
		}
		if serviceExists(name) {
			return true
		}
	}
	return false
}

func runLinuxRollbackIfNoTunnelBypassServices(serviceName string) {
	if strings.TrimSpace(serviceName) == "" || runtime.GOOS != "linux" {
		return
	}
	if len(findInstalledServices()) == 0 {
		installer.RunLinuxRollback()
		return
	}
	fmt.Fprintf(os.Stderr, "[*] Other TunnelBypass OS services remain; skipping global network rollback (sysctl/iptables).\n")
}

func cleanupArtifactsForTransport(tr installedTransport, serviceName string) {
	baseDir := installer.GetBaseDir()
	switch tr {
	case transportXray:
		_ = os.RemoveAll(installer.GetConfigDir("vless"))
	case transportVLESSWS:
		_ = os.RemoveAll(installer.GetConfigDir("vless-ws"))
	case transportHysteria:
		_ = os.RemoveAll(installer.GetConfigDir("hysteria"))
	case transportWireGuard:
		_ = os.RemoveAll(installer.GetConfigDir("wireguard"))
	case transportSSH:
		_ = os.RemoveAll(installer.GetConfigDir("ssh"))
	case transportSSL:
		_ = os.RemoveAll(installer.GetConfigDir("stunnel"))
	case transportWSS:
		_ = os.RemoveAll(installer.GetConfigDir("wstunnel"))
	case transportSSHTLS:
		_ = os.RemoveAll(installer.GetConfigDir("ssh-tls"))
	case transportGRPC:
		_ = os.RemoveAll(installer.GetConfigDir("vless-grpc"))
	case transportShadowsocks, transportShadowsocksWS:
		_ = os.RemoveAll(installer.GetConfigDir("shadowsocks"))
	case transportXDNS:
		_ = os.RemoveAll(installer.GetConfigDir("xdns"))
	case transportMDNS:
		_ = os.RemoveAll(installer.GetConfigDir("mdns"))
	case transportUDPGW:
		// No config tree; logs cleaned below.
	}

	if strings.TrimSpace(serviceName) != "" {
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".out.log"))
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".err.log"))
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".wrapper.log"))
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".log"))
	}
}

func displayWireGuardClientInfo() {
	cliPath := filepath.Join(installer.GetConfigDir("wireguard"), "wg_client.conf")
	if _, err := os.Stat(cliPath); err != nil {
		fmt.Printf("\n    %s✗ WireGuard client config not found: %s%s\n", ColorRed, cliPath, ColorReset)
		return
	}
	fmt.Printf("\n%s[ WIREGUARD CLIENT ]%s\n", ColorCyan, ColorReset)
	fmt.Printf("    %sClient config: %s%s%s\n", ColorGray, ColorBold, cliPath, ColorReset)
	link, err := wireguard.GenerateClientShareLink(cliPath)
	if err == nil && strings.TrimSpace(link.URL) != "" {
		if wgURL, err := wireguard.GenerateClientWGURL(cliPath, "TunnelBypass"); err == nil {
			fmt.Printf("    %sWG URL: %s%s%s\n", ColorGray, ColorBold, wgURL, ColorReset)
			qrPath := filepath.Join(installer.GetConfigDir("wireguard"), "qr-wireguard.png")
			if err := utils.SaveQRCodePNG(qrPath, wgURL, 320); err == nil {
				fmt.Printf("    %sWG QR saved: %s%s%s\n", ColorGray, ColorBold, qrPath, ColorReset)
			}
		}
	}
}

func displayInstructionFile(tr installedTransport) {
	var p string
	var title string
	switch tr {
	case transportSSH:
		p = filepath.Join(installer.GetConfigDir("ssh"), "ssh_tunnel_instructions.txt")
		title = "SSH INSTRUCTIONS"
	case transportSSL:
		p = filepath.Join(installer.GetConfigDir("stunnel"), "ssl_tunnel_instructions.txt")
		title = "STUNNEL INSTRUCTIONS"
	case transportWSS:
		p = filepath.Join(installer.GetConfigDir("wstunnel"), "wss_tunnel_instructions.txt")
		title = "WSTUNNEL INSTRUCTIONS"
	default:
		return
	}

	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", ColorBold+ColorCyan, ColorReset)
	fmt.Printf("║                 %s%-30s%s                     ║\n", ColorBold+ColorGreen, title, ColorReset)
	fmt.Printf("%s╚══════════════════════════════════════════════════════════════╝%s\n", ColorBold+ColorCyan, ColorReset)
	if _, err := os.Stat(p); err != nil {
		fmt.Printf("    %s✗ File not found: %s%s\n", ColorRed, p, ColorReset)
		return
	}

	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Printf("    %s✗ Failed to read file: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	fmt.Printf("    %sPath:%s %s%s%s\n", ColorGray, ColorReset, ColorBold, p, ColorReset)
	fmt.Printf("    %s──────────────────────────────────────────────────────────%s\n", ColorCyan, ColorReset)
	fmt.Printf("%s%s%s", ColorGray, string(b), ColorReset)
	if len(b) == 0 || b[len(b)-1] != '\n' {
		fmt.Println()
	}
	fmt.Printf("    %s──────────────────────────────────────────────────────────%s\n", ColorCyan, ColorReset)
}
