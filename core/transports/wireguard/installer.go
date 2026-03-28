package wireguard

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"tunnelbypass/core/installer"
)

func findWireGuardExe() (string, error) {
	if runtime.GOOS == "windows" {
		wgExe, err := installer.EnsureWireGuard()
		if err == nil {
			return wgExe, nil
		}
	} else {
		if p, err := exec.LookPath("wg"); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("wireguard not found (please install WireGuard first)")
}

func tunnelNameFromConfigPath(configPath string) string {
	base := filepath.Base(configPath)
	base = strings.TrimSuffix(base, filepath.Ext(base)) // wg_server.conf -> wg_server
	return base
}

func InstallWireGuardService(serviceName, configPath string, port int) error {
	if runtime.GOOS == "linux" {
		return installWireGuardLinux(serviceName, configPath, port)
	}
	if runtime.GOOS == "windows" {
		return installWireGuardWindows(serviceName, configPath, port)
	}
	return fmt.Errorf("wireguard service install is not supported on %s", runtime.GOOS)
}

func installWireGuardWindows(serviceName, configPath string, port int) error {

	wgExe, err := findWireGuardExe()
	if err != nil {
		return err
	}

	absConfig, _ := filepath.Abs(configPath)

	// Ensure any previous "fake" service is removed (best-effort).
	installer.WindowsServiceDelete(serviceName)

	// Uninstall existing WireGuard tunnel service (if present).
	tunnelName := tunnelNameFromConfigPath(absConfig)
	// Stop the tunnel service first if it exists/running, then uninstall.
	_ = exec.Command("sc", "stop", "WireGuardTunnel$"+tunnelName).Run()
	_ = exec.Command(wgExe, "/uninstalltunnelservice", tunnelName).Run()

	// Install official tunnel service. This creates WireGuardTunnel$<tunnelName>.
	// Sometimes SCM needs a moment after stop/uninstall; retry once.
	install := func() error {
		cmd := exec.Command(wgExe, "/installtunnelservice", absConfig)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if err := install(); err != nil {
		// One more cleanup + retry
		_ = exec.Command("sc", "stop", "WireGuardTunnel$"+tunnelName).Run()
		_ = exec.Command(wgExe, "/uninstalltunnelservice", tunnelName).Run()
		if err2 := install(); err2 != nil {
			return fmt.Errorf("failed to install WireGuard tunnel service: %v", err2)
		}
	}

	// Enable NAT + forwarding on Windows so clients can reach the Internet through this server.
	// Best-effort; if it fails, the tunnel may handshake but clients will have no Internet.
	_ = enableWindowsNAT()

	// Some clients attempt DNS-over-TLS (853). Allow it outbound in case the host firewall blocks it.
	_ = installer.OpenFirewallOutboundPort(853, "tcp", "TunnelBypass-DNS-DoT-853")

	// 4. Firewall (UDP for WireGuard)
	if port > 0 {
		_ = installer.OpenFirewallPort(port, "udp", serviceName)
	}

	return nil
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func installWireGuardLinux(serviceName, configPath string, port int) error {
	absConfig, _ := filepath.Abs(configPath)
	tunnelName := tunnelNameFromConfigPath(absConfig)
	targetCfg := filepath.Join("/etc/wireguard", tunnelName+".conf")

	if _, err := exec.LookPath("wg-quick"); err != nil {
		return fmt.Errorf("wg-quick not found; install wireguard-tools first")
	}

	// Copy generated config into canonical systemd/wg-quick path.
	if err := copyFile(absConfig, targetCfg, 0600); err != nil {
		return fmt.Errorf("failed to copy WireGuard config to %s: %v (run as root)", targetCfg, err)
	}
	_ = os.Chmod(targetCfg, 0600)

	if _, err := exec.LookPath("systemctl"); err == nil {
		unit := "wg-quick@" + tunnelName
		// Best-effort cleanup before re-enable.
		_ = exec.Command("systemctl", "disable", "--now", unit).Run()
		_ = exec.Command("wg-quick", "down", tunnelName).Run()
		if err := exec.Command("systemctl", "enable", "--now", unit).Run(); err != nil {
			return fmt.Errorf("failed to enable/start %s: %v", unit, err)
		}
		_ = writeLinuxWGState(linuxWGState{Mode: "systemd", TunnelName: tunnelName})
	} else {
		// Fallback mode for non-systemd Linux: direct wg-quick up/down + persisted state.
		_ = exec.Command("wg-quick", "down", tunnelName).Run()
		if err := exec.Command("wg-quick", "up", tunnelName).Run(); err != nil {
			return fmt.Errorf("failed to start WireGuard via wg-quick up %s: %v", tunnelName, err)
		}
		_ = writeLinuxWGState(linuxWGState{Mode: "wg-quick", TunnelName: tunnelName})
	}

	if port > 0 {
		_ = installer.OpenFirewallPort(port, "udp", serviceName)
	}
	return nil
}

type linuxWGState struct {
	Mode       string `json:"mode"` // "systemd" or "wg-quick"
	TunnelName string `json:"tunnel_name"`
}

func linuxWGStatePath() string {
	return filepath.Join(installer.GetBaseDir(), "services", "wireguard-linux-state.json")
}

func writeLinuxWGState(s linuxWGState) error {
	_ = os.MkdirAll(filepath.Dir(linuxWGStatePath()), 0755)
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(linuxWGStatePath(), b, 0644)
}

func readLinuxWGState() linuxWGState {
	data, err := os.ReadFile(linuxWGStatePath())
	if err != nil {
		return linuxWGState{}
	}
	var s linuxWGState
	if json.Unmarshal(data, &s) != nil {
		return linuxWGState{}
	}
	return s
}

func enableWindowsNAT() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	// Create (or keep) a NAT for 10.0.0.0/24. Idempotent.
	ps := `
$ErrorActionPreference = "SilentlyContinue"
$natName = "TunnelBypass-WireGuard-NAT"
$prefix = "10.0.0.0/24"
if (-not (Get-NetNat -Name $natName -ErrorAction SilentlyContinue)) {
  New-NetNat -Name $natName -InternalIPInterfaceAddressPrefix $prefix | Out-Null
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func UninstallWireGuardService(serviceName string) error {
	if runtime.GOOS == "linux" {
		st := readLinuxWGState()
		tunnelName := st.TunnelName
		if tunnelName == "" {
			tunnelName = "wg_server"
		}
		if strings.HasPrefix(serviceName, "wg-quick@") {
			tunnelName = strings.TrimPrefix(serviceName, "wg-quick@")
		}
		if st.Mode == "systemd" {
			unit := "wg-quick@" + tunnelName
			_ = exec.Command("systemctl", "disable", "--now", unit).Run()
			_ = exec.Command("systemctl", "reset-failed", unit).Run()
		}
		_ = exec.Command("wg-quick", "down", tunnelName).Run()
		_ = os.Remove(filepath.Join("/etc/wireguard", tunnelName+".conf"))
		_ = os.Remove(linuxWGStatePath())
		return nil
	}
	if runtime.GOOS == "windows" {
		// Remove any previous "fake" service (best-effort).
		installer.WindowsServiceDelete(serviceName)

		wgExe, err := findWireGuardExe()
		if err != nil {
			return err
		}

		// Default config name we generate is wg_server.conf -> wg_server
		// This matches the tunnel name WireGuard for Windows uses.
		tunnelName := "wg_server"
		_ = exec.Command(wgExe, "/uninstalltunnelservice", tunnelName).Run()
	}
	return nil
}
