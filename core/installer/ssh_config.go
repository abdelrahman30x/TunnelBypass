package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func systemDriveRoot() string {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	if !strings.HasSuffix(drive, "\\") && !strings.HasSuffix(drive, "/") {
		drive += "\\"
	}
	return drive
}

func GetSystemSSHDConfigPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(systemDriveRoot(), "ProgramData", "ssh", "sshd_config")
	}
	return "/etc/ssh/sshd_config"
}

func GetSystemSSHBannerPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(systemDriveRoot(), "ProgramData", "ssh", "TunnelBypassBanner.txt")
	}
	return "/etc/ssh/tunnelbypass_banner.txt"
}

func RestartSSHD() error {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell", "-Command", "Restart-Service sshd -ErrorAction SilentlyContinue").Run()
	}
	if runtime.GOOS == "darwin" {
		// macOS has no systemd; reload OpenSSH if running (HUP) or kick the system service.
		_ = exec.Command("killall", "-HUP", "sshd").Run()
		_ = exec.Command("launchctl", "kickstart", "-k", "system/com.openssh.sshd").Run()
		return nil
	}
	_ = exec.Command("systemctl", "restart", "ssh").Run()
	return exec.Command("systemctl", "restart", "sshd").Run()
}

func BestEffortConfigureSSHBanner(bannerText string) {
	bannerPath := GetSystemSSHBannerPath()
	_ = os.MkdirAll(filepath.Dir(bannerPath), 0755)
	_ = os.WriteFile(bannerPath, []byte(bannerText+"\n"), 0644)

	configPath := GetSystemSSHDConfigPath()
	b, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(b), "\n")
	changed := false
	for i := range lines {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "Banner ") || strings.HasPrefix(trim, "Banner\t") {
			lines[i] = "Banner " + bannerPath
			changed = true
			break
		}
	}
	if !changed {
		lines = append(lines, "", "Banner "+bannerPath)
	}

	_ = os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644)
	_ = RestartSSHD()
}

func EnsureSaneSSHConfig() {
	if SSHEmbedActive() {
		return
	}
	// No longer overwriting the entire file.
	// We only ensure basic password/pubkey auth is allowed for the system if we are root.
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		return
	}
}

func EnsureManagedSSHConfig(username string) {
	if SSHEmbedActive() {
		return
	}
	if username == "" {
		return
	}
	// Use the safe snippet-based logic instead of overwriting the whole file.
	EnsureSSHUserOnly(username)
}

func EnsureSSHUserOnly(username string) {
	if username == "" {
		return
	}
	configPath := GetSystemSSHDConfigPath()
	b, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	uname := strings.TrimSpace(username)
	matchHeader := "Match User " + uname
	matchHeaderLower := strings.ToLower(matchHeader)
	start := -1
	end := len(lines)
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(t), "match ") {
			if strings.ToLower(t) == matchHeaderLower {
				start = i
				for j := i + 1; j < len(lines); j++ {
					tt := strings.TrimSpace(lines[j])
					if strings.HasPrefix(strings.ToLower(tt), "match ") {
						end = j
						break
					}
				}
				break
			}
		}
	}
	block := []string{
		matchHeader,
		"\tPasswordAuthentication yes",
		"\tPermitTTY no",
		"\tX11Forwarding no",
		"\tAllowAgentForwarding no",
		"\tAllowTcpForwarding yes",
		"\tAllowStreamLocalForwarding no",
		"\tPermitTunnel no",
		"\tForceCommand echo \"Tunnel access only.\"",
		"\tMaxSessions 10",
	}
	if start != -1 {
		newLines := make([]string, 0, len(lines)-(end-start)+len(block))
		newLines = append(newLines, lines[:start]...)
		newLines = append(newLines, block...)
		if end < len(lines) {
			newLines = append(newLines, lines[end:]...)
		}
		lines = newLines
	} else {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "")
		lines = append(lines, block...)
	}
	_ = os.WriteFile(configPath, []byte(strings.Join(lines, sshdConfigNewlines())), 0644)
	_ = RestartSSHD()
}

func ManageSSHAllowUsers(username string, add bool) {
	configPath := GetSystemSSHDConfigPath()
	b, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	allowUsersLineIdx := -1
	var currentUsers []string
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "AllowUsers") {
			allowUsersLineIdx = i
			currentUsers = strings.Fields(trim)[1:]
			break
		}
	}
	username = strings.ToLower(username)
	newUsers := []string{}
	found := false
	for _, u := range currentUsers {
		if strings.ToLower(u) == username {
			found = true
			if add {
				newUsers = append(newUsers, u)
			}
		} else {
			newUsers = append(newUsers, u)
		}
	}
	if add && !found {
		newUsers = append(newUsers, username)
	}
	newAllowLine := "AllowUsers " + strings.Join(newUsers, " ")
	if allowUsersLineIdx != -1 {
		if len(newUsers) == 0 {
			lines = append(lines[:allowUsersLineIdx], lines[allowUsersLineIdx+1:]...)
		} else {
			lines[allowUsersLineIdx] = newAllowLine
		}
	} else if add {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, newAllowLine)
	}
	_ = os.WriteFile(configPath, []byte(strings.Join(lines, sshdConfigNewlines())), 0644)
	_ = RestartSSHD()
}

// sshdConfigNewlines: OpenSSH expects LF on Unix; Windows configs often use CRLF for local editors.
func sshdConfigNewlines() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}
