package installer

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"tunnelbypass/core/svcman"
)

// ProcessInfo represents a running process matching a tool's executable.
type ProcessInfo struct {
	PID     int
	Name    string
	CmdLine string
}

// UpgradePromptFunc is called when an upgrade requires stopping running services.
// Set by CLI layer to prompt the user. Returns true to proceed, false to skip.
// Parameters: tool, installedVer, requiredVer, runningServices, runningProcesses
var UpgradePromptFunc func(tool, installed, required string, services []string, procs []ProcessInfo) bool

// NormalizeVersion removes "v" prefix and trims whitespace for comparison.
func NormalizeVersion(ver string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ver), "v"))
}

// QueryInstalledVersion runs the binary to determine its version.
func QueryInstalledVersion(tool, binPath string) string {
	var cmd *exec.Cmd
	switch tool {
	case "xray":
		cmd = exec.Command(binPath, "version")
	case "hysteria":
		cmd = exec.Command(binPath, "version")
	case "wstunnel":
		cmd = exec.Command(binPath, "--version")
	case "shadowsocks":
		cmd = exec.Command(binPath, "--version")
	case "v2ray-plugin":
		cmd = exec.Command(binPath, "-version")
	case "stunnel":
		cmd = exec.Command(binPath, "-version")
	case "masterdnsvpn-server", "masterdnsvpn-client":
		cmd = exec.Command(binPath, "-version")
	default:
		return ""
	}

	out, err := cmd.CombinedOutput()
	if err != nil && tool != "stunnel" {
		// stunnel often returns non-zero when just asking for version
		return ""
	}

	return ParseVersionOutput(tool, string(out))
}

// ParseVersionOutput extracts the version string from the command output.
func ParseVersionOutput(tool, outStr string) string {
	switch tool {
	case "xray":
		// Example: Xray 26.3.27 (Xray, Penetrates Everything.)
		for _, line := range strings.Split(outStr, "\n") {
			if strings.HasPrefix(line, "Xray ") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					return "v" + parts[1]
				}
			}
		}
	case "hysteria":
		// Example: hysteria2 v2.6.1
		for _, line := range strings.Split(outStr, "\n") {
			if strings.Contains(line, "hysteria") {
				parts := strings.Fields(line)
				for _, p := range parts {
					if strings.HasPrefix(p, "v") {
						return p
					}
				}
			}
		}
	case "wstunnel":
		// Example: wstunnel 10.5.2
		for _, line := range strings.Split(outStr, "\n") {
			if strings.Contains(line, "wstunnel") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					return "v" + parts[1]
				}
			}
		}
	case "shadowsocks":
		// Example: shadowsocks 1.24.0
		for _, line := range strings.Split(outStr, "\n") {
			if strings.Contains(line, "shadowsocks") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					return "v" + parts[1]
				}
			}
		}
	case "v2ray-plugin":
		// Example: v2ray-plugin v5.49.0
		for _, line := range strings.Split(outStr, "\n") {
			if strings.Contains(line, "v2ray-plugin") {
				parts := strings.Fields(line)
				for _, p := range parts {
					if strings.HasPrefix(p, "v") && p != "v2ray-plugin" {
						return p
					}
				}
			}
		}
	case "stunnel":
		return "latest" // We don't upgrade stunnel based on version right now
	case "masterdnsvpn-server", "masterdnsvpn-client":
		for _, line := range strings.Split(outStr, "\n") {
			if strings.Contains(line, "MasterDnsVPN") {
				parts := strings.Fields(line)
				for _, p := range parts {
					if strings.HasPrefix(p, "v") {
						return p
					}
				}
			}
		}
	}

	return ""
}

// IsVersionMatch returns true if the installed version matches the required version.
func IsVersionMatch(tool, binPath string) bool {
	required := versionForTool(tool)
	if required == "" || required == "latest" {
		return true // Ignore tools without specific versions or stunnel
	}

	installed := QueryInstalledVersion(tool, binPath)
	if installed == "" {
		return false // Can't determine version, assume mismatch
	}

	return NormalizeVersion(installed) == NormalizeVersion(required)
}

// ToolToExecutableNames returns possible process names for a tool.
func ToolToExecutableNames(tool string) []string {
	base := strings.ToLower(tool)
	if tool == "shadowsocks" {
		base = "ssserver"
	}
	if tool == "masterdnsvpn-server" || tool == "masterdnsvpn-client" {
		base = tool
	}

	if runtime.GOOS == "windows" {
		return []string{base + ".exe"}
	}
	return []string{base}
}

// ToolToServiceNames returns potential service names for a tool.
func ToolToServiceNames(tool string) []string {
	switch tool {
	case "xray":
		return []string{
			"TunnelBypass-VLESS",
			"TunnelBypass-VLESS-WS",
			"TunnelBypass-VLESS-GRPC",
			"TunnelBypass-SSH-TLS",
			"TunnelBypass-XDNS",
		}
	case "hysteria":
		return []string{"TunnelBypass-Hysteria"}
	case "wstunnel":
		return []string{"TunnelBypass-WSS"}
	case "stunnel":
		return []string{"TunnelBypass-SSL"}
	case "shadowsocks":
		return []string{"TunnelBypass-Shadowsocks", "TunnelBypass-Shadowsocks-WS"}
	case "v2ray-plugin":
		return []string{"TunnelBypass-Shadowsocks-WS"}
	case "masterdnsvpn-server", "masterdnsvpn-client":
		return []string{"TunnelBypass-MasterDnsVPN"}
	default:
		return nil
	}
}

// FindRunningProcesses returns running processes for a given tool.
func FindRunningProcesses(tool string) ([]ProcessInfo, error) {
	var procs []ProcessInfo
	exeNames := ToolToExecutableNames(tool)

	if runtime.GOOS == "windows" {
		for _, exe := range exeNames {
			out, err := exec.Command("tasklist", "/FO", "CSV", "/NH", "/FI", fmt.Sprintf("IMAGENAME eq %s", exe)).Output()
			if err != nil {
				continue
			}

			outStr := string(out)
			if strings.Contains(outStr, "No tasks are running") {
				continue
			}

			lines := strings.Split(strings.TrimSpace(outStr), "\n")
			for _, line := range lines {
				parts := strings.Split(line, "\",\"")
				if len(parts) >= 2 {
					pidStr := strings.Trim(parts[1], "\"")
					var pid int
					fmt.Sscanf(pidStr, "%d", &pid)
					if pid > 0 {
						procs = append(procs, ProcessInfo{
							PID:  pid,
							Name: exe,
						})
					}
				}
			}
		}
	} else {
		for _, exe := range exeNames {
			out, err := exec.Command("pgrep", "-a", exe).Output()
			if err != nil {
				continue
			}

			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				parts := strings.Fields(line)
				if len(parts) >= 1 {
					var pid int
					fmt.Sscanf(parts[0], "%d", &pid)
					if pid > 0 {
						cmdline := ""
						if len(parts) > 1 {
							cmdline = strings.Join(parts[1:], " ")
						}
						procs = append(procs, ProcessInfo{
							PID:     pid,
							Name:    exe,
							CmdLine: cmdline,
						})
					}
				}
			}
		}
	}

	return procs, nil
}

// FindRunningServices returns running TunnelBypass services associated with the tool.
func FindRunningServices(tool string) []string {
	var running []string
	candidates := ToolToServiceNames(tool)

	baseDir := GetBaseDir()
	for _, name := range candidates {
		// Check user supervisor
		if svcman.UserSupervisorRunning(baseDir, name) {
			running = append(running, name)
			continue
		}

		// Check OS service
		if runtime.GOOS == "windows" {
			out, err := exec.Command("sc", "query", name).CombinedOutput()
			if err == nil && strings.Contains(strings.ToUpper(string(out)), "RUNNING") {
				running = append(running, name)
			}
		} else if runtime.GOOS == "linux" {
			if err := exec.Command("systemctl", "is-active", "--quiet", name).Run(); err == nil {
				running = append(running, name)
			}
		}
	}

	return running
}

// StopServicesForTool stops any running services related to the tool.
func StopServicesForTool(tool string) error {
	services := FindRunningServices(tool)
	for _, name := range services {
		UninstallService(name)
	}
	return nil
}

// KillProcessesByName forcefully kills processes matching the tool's executable names.
func KillProcessesByName(tool string) error {
	exeNames := ToolToExecutableNames(tool)

	if runtime.GOOS == "windows" {
		for _, exe := range exeNames {
			_ = exec.Command("taskkill", "/F", "/IM", exe).Run()
		}
	} else {
		for _, exe := range exeNames {
			_ = exec.Command("pkill", "-9", exe).Run()
		}
	}

	return nil
}
