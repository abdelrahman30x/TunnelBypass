package cli

import (
	"bufio"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/svcman"
	"tunnelbypass/core/transports/hysteria"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/transports/wireguard"
)

func findInstalledService() string {
	services := findInstalledServices()
	if len(services) > 0 {
		return services[0]
	}
	return ""
}

func findInstalledServices() []string {
	candidates := []string{
		"TunnelBypass-VLESS",
		"TunnelBypass-VLESS-WS",
		"TunnelBypass-VLESS-GRPC",
		"TunnelBypass-SSH-TLS",
		"TunnelBypass-UDP",
		"TunnelBypass-Hysteria",
		"TunnelBypass-WireGuard",
		installer.UDPGWServiceName,
		"TunnelBypass-SSH",
		"TunnelBypass-SSL",
		"TunnelBypass-WSS",
		"TunnelBypass-Tunnel",
		"WireGuardTunnel$wg_server", // Windows
		"wg-quick@wg_server",        // Linux
	}
	seen := map[string]bool{}
	var out []string
	for _, name := range candidates {
		if serviceExists(name) {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func selectInstalledService(reader *bufio.Reader, services []string, title string) string {
	if len(services) == 0 {
		fmt.Printf("    %sNo installed services found.%s\n", ColorYellow, ColorReset)
		return ""
	}
	if len(services) == 1 {
		return services[0]
	}
	fmt.Printf("\n%s[!] %s%s\n", ColorYellow+ColorBold, title, ColorReset)
	for i, s := range services {
		status := fmt.Sprintf("%sSTOPPED%s", ColorRed, ColorReset)
		if serviceRunning(s) {
			status = fmt.Sprintf("%sRUNNING%s", ColorGreen, ColorReset)
		}
		fmt.Printf("  %s%2d)%s %s%s%s %s[%s]%s\n", ColorCyan, i+1, ColorReset, ColorBold, prettyServiceName(s), ColorReset, ColorGray, status, ColorReset)
	}
	fmt.Printf("  %sa)%s %sUninstall ALL services%s\n", ColorCyan, ColorReset, ColorRed, ColorReset)
	fmt.Printf("  %sb)%s %sBack%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)
	choice := strings.ToLower(prompt(reader, fmt.Sprintf("\n    %sSelect service: %s", ColorBold, ColorReset)))
	if choice == "a" || choice == "all" {
		confirm := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("    %sType 'yes' to uninstall ALL services: %s", ColorBold+ColorRed, ColorReset))))
		if confirm == "yes" {
			uninstallAllServices(services)
		} else {
			fmt.Printf("    %sCancelled.%s\n", ColorYellow, ColorReset)
		}
		return ""
	}
	if choice == "" || choice == "b" || choice == "back" {
		return ""
	}
	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(services) {
		fmt.Printf("    %sInvalid selection.%s\n", ColorRed, ColorReset)
		return ""
	}
	return services[idx-1]
}

func prettyServiceName(name string) string {
	if strings.EqualFold(name, "WireGuardTunnel$wg_server") || strings.EqualFold(name, "wg-quick@wg_server") {
		return "TunnelBypass-WireGuard"
	}
	return name
}

func uninstallAllServices(services []string) {
	if len(services) == 0 {
		fmt.Printf("    %sNo services to uninstall.%s\n", ColorYellow, ColorReset)
		return
	}
	fmt.Printf("\n    %s[*] Uninstalling all detected services...%s\n", ColorYellow, ColorReset)
	// If UDPGW is already in the services list it will be uninstalled by the main loop.
	// Pre-mark it so the SSH/WSS dependency block doesn't trigger a duplicate removal.
	udpgwUninstalled := false
	for _, s := range services {
		if strings.EqualFold(s, installer.UDPGWServiceName) {
			udpgwUninstalled = true
			break
		}
	}
	for _, s := range services {
		tr := detectInstalledTransport(s)
		var err error
		if strings.Contains(s, "Hysteria") {
			err = hysteria.UninstallHysteriaService(s)
		} else if strings.Contains(s, "WireGuard") || strings.HasPrefix(s, "WireGuardTunnel$") || strings.HasPrefix(s, "wg-quick@") {
			err = wireguard.UninstallWireGuardService(s)
		} else {
			err = vless.UninstallXrayService(s)
		}
		if err != nil {
			fmt.Printf("    %s✗ %s: %v%s\n", ColorRed, prettyServiceName(s), err, ColorReset)
		} else {
			fmt.Printf("    %s✓ Uninstalled:%s %s%s%s\n", ColorGreen, ColorReset, ColorBold, prettyServiceName(s), ColorReset)
			removePortAllocState(s)
			cleanupArtifactsForTransport(tr, s)
			// Uninstall UDPGW as a dependency of SSH-based transports (only once)
			if !udpgwUninstalled && (tr == transportSSH || tr == transportSSL || tr == transportWSS || tr == transportSSHTLS) {
				if serviceExists(installer.UDPGWServiceName) {
					installer.UninstallService(installer.UDPGWServiceName)
					removePortAllocState(installer.UDPGWServiceName)
					cleanupArtifactsForTransport(transportUDPGW, installer.UDPGWServiceName)
					fmt.Printf("    %s✓ Uninstalled:%s %s%s%s\n", ColorGreen, ColorReset, ColorBold, prettyServiceName(installer.UDPGWServiceName), ColorReset)
					udpgwUninstalled = true
				}
			}
		}
	}
	// If no SSH-based transport was found but UDPGW exists, uninstall it separately
	if !udpgwUninstalled && serviceExists(installer.UDPGWServiceName) {
		installer.UninstallService(installer.UDPGWServiceName)
		removePortAllocState(installer.UDPGWServiceName)
		cleanupArtifactsForTransport(transportUDPGW, installer.UDPGWServiceName)
		fmt.Printf("    %s✓ Uninstalled:%s %s%s%s\n", ColorGreen, ColorReset, ColorBold, prettyServiceName(installer.UDPGWServiceName), ColorReset)
	}
}

// darwinLaunchctlMatch is a best-effort check against `launchctl list` for jobs whose label contains name
// (e.g. third-party LaunchAgents). UserSupervisor-only installs may not appear here.
func darwinLaunchctlMatch(name string) (exists, running bool) {
	if runtime.GOOS != "darwin" {
		return false, false
	}
	needle := strings.TrimSpace(name)
	if needle == "" {
		return false, false
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		return false, false
	}
	out, err := exec.Command("launchctl", "list").CombinedOutput()
	if err != nil {
		return false, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}
		// Tab-separated: PID, status, label (avoids false positives vs substring in other columns).
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		pidStr := strings.TrimSpace(parts[0])
		label := strings.TrimSpace(parts[2])
		if !strings.Contains(label, needle) {
			continue
		}
		exists = true
		if pidStr != "-" && pidStr != "" {
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				running = true
			}
		}
		return exists, running
	}
	return false, false
}

func serviceExists(name string) bool {
	if name == "" {
		return false
	}
	if svcman.UserSupervisorInstalled(installer.GetBaseDir(), name) {
		return true
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("sc", "query", name)
		return cmd.Run() == nil
	}
	if runtime.GOOS == "linux" {
		// Template units (e.g. wg-quick@wg_server) stay LoadState=loaded even when disabled; use enabled/active.
		if strings.HasPrefix(name, "wg-quick@") {
			if exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil {
				return true
			}
			out, _ := exec.Command("systemctl", "is-enabled", name).CombinedOutput()
			st := strings.TrimSpace(string(out))
			return st == "enabled" || st == "enabled-runtime"
		}
		out, _ := exec.Command("systemctl", "show", "-p", "LoadState", name).CombinedOutput()
		return strings.Contains(string(out), "LoadState=loaded")
	}
	if runtime.GOOS == "darwin" {
		ex, _ := darwinLaunchctlMatch(name)
		return ex
	}
	return false
}

func serviceRunning(name string) bool {
	if name == "" {
		return false
	}
	if svcman.UserSupervisorRunning(installer.GetBaseDir(), name) {
		return true
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("sc", "query", name).CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToUpper(string(out)), "RUNNING")
	}
	if runtime.GOOS == "linux" {
		return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
	}
	if runtime.GOOS == "darwin" {
		_, run := darwinLaunchctlMatch(name)
		return run
	}
	return false
}

func waitForInstalledService(preferred string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	if preferred != "" {
		for time.Now().Before(deadline) {
			if serviceRunning(preferred) || serviceExists(preferred) {
				return preferred
			}
			time.Sleep(600 * time.Millisecond)
		}
		return preferred
	}
	for time.Now().Before(deadline) {
		if s := findInstalledService(); s != "" {
			return s
		}
		time.Sleep(600 * time.Millisecond)
	}
	return ""
}
