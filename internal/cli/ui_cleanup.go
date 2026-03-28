package cli

import (
	"fmt"
	"os"
	"os/exec"
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
	transportXray      installedTransport = "xray"
	transportHysteria  installedTransport = "hysteria"
	transportWireGuard installedTransport = "wireguard"
	transportSSH       installedTransport = "ssh"
	transportSSL       installedTransport = "ssl"
	transportWSS       installedTransport = "wss"
	transportUDPGW     installedTransport = "udpgw"
	transportUnknown   installedTransport = "unknown"
)

func detectInstalledTransport(serviceName string) installedTransport {
	s := strings.ToLower(serviceName)
	switch {
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
	case strings.Contains(s, "udpgw"):
		return transportUDPGW
	case strings.Contains(s, "vless"), strings.Contains(s, "udp"), strings.Contains(s, "tunnel"):
		return transportXray
	default:
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")); err == nil {
			return transportHysteria
		}
		if _, err := os.Stat(filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf")); err == nil {
			return transportWireGuard
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
		return transportUnknown
	}
}

func freshSetupCleanup(serviceName string) error {
	tr := detectInstalledTransport(serviceName)

	if serviceName != "" {
		if strings.Contains(serviceName, "Hysteria") {
			_ = hysteria.UninstallHysteriaService(serviceName)
		} else if strings.Contains(serviceName, "WireGuard") || strings.HasPrefix(serviceName, "WireGuardTunnel$") || strings.HasPrefix(serviceName, "wg-quick@") {
			_ = wireguard.UninstallWireGuardService(serviceName)
		} else {
			_ = vless.UninstallXrayService(serviceName)
		}

		installer.UninstallService(serviceName)
		removePortAllocState(serviceName)
	}

	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/IM", "xray.exe").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "hysteria.exe").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "wstunnel.exe").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "stunnel.exe").Run()
	} else {
		_ = exec.Command("pkill", "-9", "xray").Run()
		_ = exec.Command("pkill", "-9", "hysteria").Run()
		_ = exec.Command("pkill", "-9", "wstunnel").Run()
		_ = exec.Command("pkill", "-9", "stunnel").Run()
	}

	cleanupArtifactsForTransport(tr, serviceName)
	return nil
}

func cleanupArtifactsForTransport(tr installedTransport, serviceName string) {
	baseDir := installer.GetBaseDir()
	switch tr {
	case transportXray:
		_ = os.RemoveAll(installer.GetConfigDir("vless"))
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
	case transportUDPGW:
		// No config tree; logs cleaned below.
	}

	if strings.TrimSpace(serviceName) != "" {
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".out.log"))
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".err.log"))
		_ = os.Remove(filepath.Join(baseDir, "logs", serviceName+".wrapper.log"))
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
