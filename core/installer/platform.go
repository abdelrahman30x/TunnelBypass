package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"tunnelbypass/internal/uicolors"
)

// ── Firewall ──────────────────────────────────────────────────────────────────

// OpenFirewallPort adds an inbound firewall rule for the given port/protocol.
// On Windows it uses netsh. On Linux: firewalld (firewall-cmd) if running, else ufw when
// active, else idempotent iptables+ip6tables INPUT (nftables backend is common; do not mix with active firewalld).
// Without admin/root, prints a hint and returns nil (no-op).
func OpenFirewallPort(port int, protocol, name string) error {
	if port <= 0 {
		return nil
	}
	portStr := fmt.Sprintf("%d", port)
	ruleName := strings.TrimSpace(name)
	if ruleName == "" {
		ruleName = "TunnelBypass-Port-" + portStr
	}

	if runtime.GOOS == "windows" {
		if _, err := exec.Command("net", "session").Output(); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Skipped Windows Firewall inbound rule %q on port %d/%s (not running as Administrator). Add the rule manually if remote clients cannot connect.\n", ruleName, port, protocol)
			return nil
		}
		fmt.Printf("[*] Opening firewall rule '%s' port %d (%s)...\n", ruleName, port, protocol)
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+ruleName).Run()
		cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
			"name="+ruleName, "dir=in", "action=allow", "protocol="+strings.ToUpper(protocol), "localport="+portStr)
		return cmd.Run()
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		fmt.Fprintf(os.Stderr, "[!] Skipped automatic inbound firewall rule for %q port %d/%s: only Windows and Linux implement automatic inbound rules here; open TCP %s in your firewall if needed.\n", ruleName, port, protocol, portStr)
		return nil
	}
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "[!] Skipped automatic inbound allow for %q port %d/%s (not root). Configure ufw, firewalld, or iptables if needed.\n", ruleName, port, protocol)
		return nil
	}

	fmt.Printf("[*] Opening firewall rule '%s' port %d (%s)...\n", ruleName, port, protocol)

	// 1) firewalld (Fedora/RHEL/CentOS) — avoid mixing raw iptables with running firewalld.
	if firewallCmdRunning() {
		_ = exec.Command("firewall-cmd", "--permanent", "--add-port="+portStr+"/"+protocol).Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
		fmt.Printf("[*] Opened port %s/%s via firewalld (firewall-cmd).\n", portStr, protocol)
		return nil
	}

	// 2) UFW when active (Ubuntu/Debian) — ufw manages netfilter; do not duplicate with iptables.
	if ufwIsActive() {
		_ = exec.Command("ufw", "allow", fmt.Sprintf("%d/%s", port, protocol)).Run()
		fmt.Printf("[*] Opened port %s/%s via ufw (active).\n", portStr, protocol)
		return nil
	}

	// UFW installed but inactive: pre-create rule for a later `ufw enable`.
	if _, err := exec.LookPath("ufw"); err == nil {
		_ = exec.Command("ufw", "allow", fmt.Sprintf("%d/%s", port, protocol)).Run()
	}

	// 3) Fallback: iptables + ip6tables (often nft backend on modern distros); idempotent INPUT allow.
	if _, err := exec.LookPath("iptables"); err == nil {
		addIptablesInputAllow(protocol, portStr)
		fmt.Printf("[*] Inbound %s/%s allowed via iptables/ip6tables (no active firewalld/ufw).\n", portStr, protocol)
	} else if runtime.GOOS == "linux" {
		fmt.Fprintf(os.Stderr, "[!] No iptables: could not add direct INPUT allow for %d/%s (install iptables or enable ufw/firewalld).\n", port, protocol)
	}
	return nil
}

// CloseFirewallPort removes or blocks an inbound firewall rule.
func CloseFirewallPort(port int, protocol, name string) error {
	if port <= 0 {
		return nil
	}
	portStr := fmt.Sprintf("%d", port)
	ruleName := strings.TrimSpace(name)
	if ruleName == "" {
		ruleName = "TunnelBypass-Port-" + portStr
	}

	if runtime.GOOS == "windows" {
		if _, err := exec.Command("net", "session").Output(); err != nil {
			return nil
		}
		// On Windows, we can either delete the rule or add a block rule.
		// Deleting is cleaner if we created it.
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+ruleName).Run()
		return nil
	}

	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return nil
	}

	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		return nil
	}

	if firewallCmdRunning() {
		_ = exec.Command("firewall-cmd", "--permanent", "--remove-port="+portStr+"/"+protocol).Run()
		_ = exec.Command("firewall-cmd", "--reload").Run()
		return nil
	}
	if ufwIsActive() {
		_ = exec.Command("ufw", "deny", fmt.Sprintf("%d/%s", port, protocol)).Run()
		return nil
	}
	removeIptablesInputAllow(protocol, portStr)
	return nil
}

// OpenFirewallOutboundPort adds an outbound firewall rule (Windows only).
func OpenFirewallOutboundPort(remotePort int, protocol, name string) error {
	portStr := fmt.Sprintf("%d", remotePort)
	if name == "" {
		name = "TunnelBypass-Out-" + portStr
	}

	if runtime.GOOS == "windows" {
		_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name).Run()
		cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
			"name="+name, "dir=out", "action=allow", "protocol="+strings.ToUpper(protocol), "remoteport="+portStr)
		return cmd.Run()
	}

	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return nil
	}

	if _, err := exec.LookPath("ufw"); err == nil {
		_ = exec.Command("ufw", "allow", "out", portStr+"/"+protocol).Run()
	}
	return nil
}

// EnsureWindowsUser creates/updates a local Windows user; optional admin group.
func EnsureWindowsUser(username, password string, updatePassword bool, isAdmin bool) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	exists := exec.Command("net", "user", username).Run() == nil

	if !exists {
		addCmd := exec.Command("net", "user", username, password, "/add", "/Y", "/active:yes", "/expires:never")
		if err := addCmd.Run(); err != nil {
			return fmt.Errorf("failed to create windows user %s: %w", username, err)
		}
	} else if updatePassword {
		userDisp := uicolors.ColorBold + uicolors.ColorCyan + username + uicolors.ColorReset
		fmt.Printf("%s    - %sUpdating existing user '%s'%s password and ensuring active...%s\n",
			uicolors.ColorGray, uicolors.ColorYellow, userDisp, uicolors.ColorYellow, uicolors.ColorReset)
		passCmd := exec.Command("net", "user", username, password, "/active:yes", "/expires:never")
		if err := passCmd.Run(); err != nil {
			return fmt.Errorf("failed to update windows user %s password: %w", username, err)
		}
	}

	// Resolve the localized name of the Administrators group
	groupName := "Administrators" // Fallback default
	psCmd := `([Security.Principal.SecurityIdentifier]'S-1-5-32-544').Translate([Security.Principal.NTAccount]).Value.Split('\')[-1]`
	if out, err := exec.Command("powershell", "-NoProfile", "-Command", psCmd).Output(); err == nil {
		if name := strings.TrimSpace(string(out)); name != "" {
			groupName = name
		}
	}

	if isAdmin {
		_ = exec.Command("net", "localgroup", groupName, username, "/add").Run()
	} else {
		_ = exec.Command("net", "localgroup", groupName, username, "/delete").Run()
	}

	return nil
}

// ListWindowsUsers returns a filtered list of local Windows users.
// System/built-in accounts (Guest, DefaultAccount, sshd, etc.) are excluded.
func ListWindowsUsers() ([]string, error) {
	if runtime.GOOS != "windows" {
		return []string{"root", "user"}, nil
	}

	isSystemUser := func(u string) bool {
		u = strings.ToLower(strings.TrimSpace(u))
		blacklist := []string{
			"guest", "defaultaccount", "wdagutilityaccount", "utilityaccount",
			"sshd", "localservice", "networkservice", "system", "administrator",
		}
		for _, b := range blacklist {
			if u == b {
				return true
			}
		}
		return strings.HasPrefix(u, "wdag") || strings.HasPrefix(u, "default")
	}

	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Get-LocalUser | Select-Object -ExpandProperty Name")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n")
		users := []string{"Administrator"}
		seen := map[string]bool{"administrator": true}

		for _, l := range lines {
			u := strings.TrimSpace(l)
			if u == "" || strings.ToLower(u) == "administrator" {
				continue
			}
			if !isSystemUser(u) && !seen[strings.ToLower(u)] {
				users = append(users, u)
				seen[strings.ToLower(u)] = true
			}
		}
		if len(users) > 0 {
			return users, nil
		}
	}

	// Fallback: parse 'net user' output.
	cmd = exec.Command("net", "user")
	out, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows users: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	var users []string
	startParsing := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "----") {
			startParsing = true
			continue
		}
		if strings.Contains(line, "The command completed successfully") {
			break
		}
		if startParsing && line != "" {
			for _, f := range strings.Fields(line) {
				if !isSystemUser(f) {
					users = append(users, f)
				}
			}
		}
	}
	return users, nil
}

// envForPkgManager sets env so apt-get does not stop on debconf prompts (kernel upgrade / package config dialogs).
func envForPkgManager(bin string) []string {
	env := os.Environ()
	if bin == "apt-get" {
		env = append(env, "DEBIAN_FRONTEND=noninteractive", "APT_LISTCHANGES_FRONTEND=none")
	}
	return env
}

// EnsureWgQuick ensures wg-quick is on PATH (Linux: may run apt/dnf to install wireguard-tools).
// Required for wg-quick@ systemd units and installWireGuardLinux.
func EnsureWgQuick() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := exec.LookPath("wg-quick"); err == nil {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("wg-quick not found; install WireGuard tools for your OS")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("wg-quick not found; install wireguard-tools (e.g. sudo apt install -y wireguard-tools)")
	}
	fmt.Printf("[*] Installing wireguard-tools (wg-quick)...\n")
	for _, pkg := range [][]string{
		{"apt-get", "install", "-y", "wireguard-tools"},
		{"apt-get", "install", "-y", "wireguard"},
		{"dnf", "install", "-y", "wireguard-tools"},
		{"dnf", "install", "-y", "wireguard"},
		{"yum", "install", "-y", "wireguard-tools"},
		{"apk", "add", "--no-cache", "wireguard-tools"},
		{"pacman", "-Sy", "--noconfirm", "wireguard-tools"},
	} {
		if _, err := exec.LookPath(pkg[0]); err != nil {
			continue
		}
		cmd := exec.Command(pkg[0], pkg[1:]...)
		cmd.Env = envForPkgManager(pkg[0])
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		if _, err := exec.LookPath("wg-quick"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("wg-quick not found after attempting to install wireguard-tools")
}

// EnsureWireGuard installs if needed and returns wireguard.exe or wg path.
func EnsureWireGuard() (string, error) {
	if runtime.GOOS == "windows" {
		prog := os.Getenv("ProgramFiles")
		if prog == "" {
			drive := os.Getenv("SystemDrive")
			if drive == "" {
				drive = "C:"
			}
			prog = filepath.Join(drive, "Program Files")
		}
		wgExe := filepath.Join(prog, "WireGuard", "wireguard.exe")
		if _, err := os.Stat(wgExe); err == nil {
			return wgExe, nil
		}

		arch := runtime.GOARCH
		var msiName string
		switch arch {
		case "amd64":
			msiName = fmt.Sprintf("wireguard-amd64-%s.msi", WireGuardMSIVersion)
		case "386":
			msiName = fmt.Sprintf("wireguard-x86-%s.msi", WireGuardMSIVersion)
		case "arm64":
			msiName = fmt.Sprintf("wireguard-arm64-%s.msi", WireGuardMSIVersion)
		default:
			return "", fmt.Errorf("unsupported Windows arch for WireGuard MSI: %s", arch)
		}

		binDir := GetSystemBinaryDir("wireguard-msi")
		_ = os.MkdirAll(binDir, 0755)
		msiPath := filepath.Join(binDir, msiName)

		if _, err := os.Stat(msiPath); err != nil {
			url := "https://download.wireguard.com/windows-client/" + msiName
			fmt.Printf("[*] WireGuard not found. Downloading %s...\n", msiName)
			if err := downloadFileWithProgress(url, msiPath); err != nil {
				return "", fmt.Errorf("failed to download WireGuard MSI: %w", err)
			}
		}

		fmt.Printf("[*] Installing WireGuard (silent)...\n")
		cmd := exec.Command("msiexec.exe", "/i", msiPath, "DO_NOT_LAUNCH=1", "/qn")
		_ = cmd.Run()

		if _, err := os.Stat(wgExe); err == nil {
			return wgExe, nil
		}
		return "", fmt.Errorf("WireGuard install completed but wireguard.exe not found")
	}

	if p, err := exec.LookPath("wg"); err == nil {
		if _, err := exec.LookPath("wg-quick"); err == nil {
			return p, nil
		}
	}
	for _, pkg := range [][]string{
		{"apt-get", "install", "-y", "wireguard-tools"},
		{"apt-get", "install", "-y", "wireguard"},
		{"dnf", "install", "-y", "wireguard-tools"},
		{"dnf", "install", "-y", "wireguard"},
		{"yum", "install", "-y", "wireguard-tools"},
		{"yum", "install", "-y", "wireguard"},
		{"apk", "add", "--no-cache", "wireguard-tools"},
		{"pacman", "-Sy", "--noconfirm", "wireguard-tools"},
	} {
		if _, err := exec.LookPath(pkg[0]); err != nil {
			continue
		}
		cmd := exec.Command(pkg[0], pkg[1:]...)
		cmd.Env = envForPkgManager(pkg[0])
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		if p, err := exec.LookPath("wg"); err == nil {
			if _, err := exec.LookPath("wg-quick"); err == nil {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("wireguard not found and auto-install failed")
}

// WriteStunnelServerConfig writes stunnel.ini-style server fragment (accept -> local SSH).
func WriteStunnelServerConfig(cfgPath string, acceptPort, connectPort int, certPath, keyPath string) error {
	certPath = filepath.ToSlash(certPath)
	keyPath = filepath.ToSlash(keyPath)

	cfg := fmt.Sprintf(`; TunnelBypass stunnel server config — auto-generated
; DO NOT EDIT — regenerated on each setup run.

; Global options
foreground = yes
debug = debug

[ssh-ssl]
accept  = 0.0.0.0:%d
connect = 127.0.0.1:%d
cert    = %s
key     = %s
sslVersion = all
options = NO_SSLv2
options = NO_SSLv3
`, acceptPort, connectPort, certPath, keyPath)

	_ = os.MkdirAll(filepath.Dir(cfgPath), 0755)
	return os.WriteFile(cfgPath, []byte(cfg), 0644)
}

// WriteStunnelClientConfig writes a stunnel client configuration file for end-user use.
func WriteStunnelClientConfig(cfgPath, serverIP string, serverPort, localPort int, sni string) error {
	sniLine := ""
	if sni != "" {
		sniLine = fmt.Sprintf("sni           = %s\n", sni)
	}
	cfg := fmt.Sprintf(`; TunnelBypass stunnel CLIENT config — auto-generated
; Run on the CLIENT machine to create a local SSL tunnel.

[ssh-ssl-client]
client        = yes
accept        = 127.0.0.1:%d
connect       = %s:%d
%sverify        = 0
`, localPort, serverIP, serverPort, sniLine)

	_ = os.MkdirAll(filepath.Dir(cfgPath), 0755)
	return os.WriteFile(cfgPath, []byte(cfg), 0644)
}
