package portable

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"tunnelbypass/core/svcman"
	"tunnelbypass/internal/utils"
)

// quoteDataDirForDisplay wraps paths with spaces for copy-paste hints (cmd.exe style).
func quoteDataDirForDisplay(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.ContainsAny(s, " \t") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

type ConflictKind string

const (
	ConflictNone                  ConflictKind = "none"
	ConflictSameServiceSameConfig ConflictKind = "same_service_same_config"
	ConflictSameServiceDiffConfig ConflictKind = "same_service_diff_config"
	ConflictDifferentService      ConflictKind = "different_service_using_port"
)

type PortConflict struct {
	Kind               ConflictKind
	Transport          string
	Port               int
	ProcessName        string
	PID                int
	RunningTransport   string
	RunningTransportOK bool
	// OSServiceName is the Windows SCM / WinSW / systemd unit id to stop (e.g. TunnelBypass-VLESS).
	OSServiceName string
	Suggestions   []string
}

func lookupPortOwner(port int) (proc string, pid int) {
	if port <= 0 {
		return "", 0
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "netstat -ano -p tcp | findstr LISTENING | findstr :"+strconv.Itoa(port))
		out, err := cmd.Output()
		if err != nil || len(out) == 0 {
			return "", 0
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, l := range lines {
			f := strings.Fields(strings.TrimSpace(l))
			if len(f) < 5 {
				continue
			}
			p, err := strconv.Atoi(f[len(f)-1])
			if err != nil || p <= 0 {
				continue
			}
			pid = p
			break
		}
		if pid > 0 {
			tc := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
			tb, _ := tc.Output()
			line := strings.TrimSpace(string(tb))
			if strings.HasPrefix(line, "\"") {
				parts := strings.Split(line, "\",\"")
				if len(parts) > 0 {
					proc = strings.Trim(parts[0], "\"")
				}
			}
		}
		return proc, pid
	}
	cmd := exec.Command("sh", "-lc", "ss -ltnp '( sport = :"+strconv.Itoa(port)+" )' 2>/dev/null || netstat -ltnp 2>/dev/null | grep :"+strconv.Itoa(port))
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return "", 0
	}
	line := strings.TrimSpace(string(bytes.SplitN(out, []byte{'\n'}, 2)[0]))
	if i := strings.Index(line, "pid="); i >= 0 {
		rest := line[i+4:]
		var digits strings.Builder
		for _, r := range rest {
			if r < '0' || r > '9' {
				break
			}
			digits.WriteRune(r)
		}
		pid, _ = strconv.Atoi(digits.String())
	}
	if i := strings.Index(line, "users:((\""); i >= 0 {
		rest := line[i+9:]
		if j := strings.Index(rest, "\""); j > 0 {
			proc = rest[:j]
		}
	}

	// Fallback for netstat output: tcp 0 0 0.0.0.0:443 0.0.0.0:* LISTEN 1234/xray
	if proc == "" && pid == 0 {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			last := fields[len(fields)-1] // e.g. "1234/xray"
			if idx := strings.IndexByte(last, '/'); idx > 0 {
				pidStr := last[:idx]
				procStr := last[idx+1:]
				if p, err := strconv.Atoi(pidStr); err == nil {
					pid = p
					proc = procStr
					// Sometimes proc has a trailing colon or dash, clean it up if needed.
				}
			}
		}
	}
	return proc, pid
}

func runningTransportOnPort(port int) (string, bool) {
	rf, err := ReadRegistry()
	if err != nil || len(rf.Transports) == 0 {
		return "", false
	}
	for name, e := range rf.Transports {
		if e == nil || e.PID <= 0 || !svcman.IsPIDAlive(e.PID) {
			continue
		}
		for _, p := range e.Ports {
			if p == port {
				return name, true
			}
		}
	}
	return "", false
}

// OS service name TunnelBypass uses for this transport (SCM / WinSW / systemd).
func OSServiceNameForTransport(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "reality", "vless":
		return "TunnelBypass-VLESS"
	case "hysteria":
		return "TunnelBypass-Hysteria"
	case "wireguard":
		return "TunnelBypass-WireGuard"
	case "wss":
		return "TunnelBypass-WSS"
	case "tls":
		return "TunnelBypass-SSL"
	default:
		return ""
	}
}

// inferOSServiceName maps a listening process or registry transport to the OS service id we install.
func inferOSServiceName(requestedTransport, processName, runningTransport string) string {
	if runningTransport != "" {
		if s := OSServiceNameForTransport(runningTransport); s != "" {
			return s
		}
	}
	pl := strings.ToLower(processName)
	switch {
	case strings.Contains(pl, "xray"):
		return "TunnelBypass-VLESS"
	case strings.Contains(pl, "hysteria"):
		return "TunnelBypass-Hysteria"
	case strings.Contains(pl, "wstunnel"):
		return "TunnelBypass-WSS"
	case strings.Contains(pl, "stunnel"):
		return "TunnelBypass-SSL"
	case strings.Contains(pl, "wireguard") || strings.Contains(pl, "boringtun") || strings.Contains(pl, "wg-quick"):
		return "TunnelBypass-WireGuard"
	case strings.Contains(pl, "tunnelbypass"):
		if requestedTransport != "" {
			return OSServiceNameForTransport(requestedTransport)
		}
	}
	return ""
}

func osStopServiceHint(serviceName string) string {
	if serviceName == "" {
		return ""
	}
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("[+] Stop the background service manually (Administrator cmd):\n     sc stop %s", serviceName)
	case "linux":
		return fmt.Sprintf("[+] Stop the background service manually (if using systemd):\n     sudo systemctl stop %s", serviceName)
	case "darwin":
		return "[+] Stop the background service manually (Activity Monitor or launchctl), then retry."
	default:
		return fmt.Sprintf("[+] Stop the service %q using your OS service manager, then retry.", serviceName)
	}
}

func portInspectHint(port int) string {
	switch runtime.GOOS {
	case "windows":
		return "[+] Inspect who owns this port:\nnetstat -ano | findstr :" + strconv.Itoa(port)
	default:
		return "[+] Inspect who owns this port:\nss -lntp | grep ':" + strconv.Itoa(port) + " ' || netstat -lntp 2>/dev/null | grep ':" + strconv.Itoa(port) + "'"
	}
}

func BuildConflict(transport string, requestedPort int, commandHint string, network string, dataDir string) PortConflict {
	out := PortConflict{
		Kind:      ConflictNone,
		Transport: strings.ToLower(strings.TrimSpace(transport)),
		Port:      requestedPort,
	}
	dataDir = strings.TrimSpace(dataDir)
	netw := strings.ToLower(strings.TrimSpace(network))
	if netw == "" {
		netw = "tcp"
	}
	if requestedPort <= 0 || utils.IsPortAvailable(netw, requestedPort) {
		return out
	}
	if netw == "tcp" {
		out.ProcessName, out.PID = lookupPortOwner(requestedPort)
	}
	runningT, runningOK := runningTransportOnPort(requestedPort)
	out.RunningTransport = runningT
	out.RunningTransportOK = runningOK

	if runningOK && runningT == out.Transport {
		out.Kind = ConflictSameServiceSameConfig
	} else if runningOK && runningT != out.Transport {
		out.Kind = ConflictDifferentService
	} else if runningOK {
		out.Kind = ConflictSameServiceDiffConfig
	} else {
		out.Kind = ConflictDifferentService
	}
	out.OSServiceName = inferOSServiceName(out.Transport, out.ProcessName, out.RunningTransport)
	out.Suggestions = buildSuggestions(out, commandHint, dataDir)
	return out
}

func uninstallCLIHint(serviceName, dataDir string) string {
	if strings.TrimSpace(serviceName) == "" {
		return ""
	}
	line := utils.AppName() + " uninstall --service " + serviceName + " --yes"
	if q := quoteDataDirForDisplay(dataDir); q != "" {
		line += " --data-dir " + q
	}
	return "[+] Remove the conflicting service and free the port (Smart Cross-Platform Method):\n     " + line
}

func killProcessLastResortHint(c PortConflict) string {
	if c.PID <= 0 {
		return ""
	}
	var intro string
	if c.OSServiceName != "" {
		intro = "[+] Last resort: after stopping the service above, if the process still listens you may force-kill (skipping sc stop can leave a broken service):\n"
	} else {
		intro = "[+] If this listener is not something you need, you may force-kill (caution — may disrupt other software):\n"
	}
	switch runtime.GOOS {
	case "windows":
		return intro + fmt.Sprintf("taskkill /PID %d /F", c.PID)
	default:
		return intro + fmt.Sprintf("sudo kill -9 %d", c.PID)
	}
}

func buildSuggestions(c PortConflict, commandHint string, dataDir string) []string {
	var out []string
	port2 := c.Port + 1
	if port2 < 1024 {
		port2 = 8443
	}
	if commandHint == "" {
		commandHint = utils.AppName() + " run --type " + c.Transport
	}

	// Smart Hint #1: Unified Uninstall
	sName := c.OSServiceName
	if sName == "" && c.RunningTransport != "" {
		sName = OSServiceNameForTransport(c.RunningTransport)
	}
	if sName == "" && c.Transport != "" {
		sName = OSServiceNameForTransport(c.Transport)
	}

	if sName != "" {
		out = append(out, uninstallCLIHint(sName, dataDir))
		if hint := osStopServiceHint(sName); hint != "" {
			out = append(out, hint)
		}
	}

	out = append(out, fmt.Sprintf("[+] Try another port (Quick Fix):\n     %s --port %d", commandHint, port2))
	out = append(out, "[+] Check current status:\n     tunnelbypass status")

	if c.RunningTransport == "" && sName == "" {
		out = append(out, portInspectHint(c.Port))
	}
	if hint := killProcessLastResortHint(c); hint != "" {
		out = append(out, hint)
	}
	return out
}

func FormatConflict(c PortConflict) string {
	if c.Kind == ConflictNone {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[!] Port %d is already in use\n", c.Port))
	b.WriteString("Detected:\n")
	if c.ProcessName != "" || c.PID > 0 {
		b.WriteString(fmt.Sprintf("- Process: %s (PID %d)\n", c.ProcessName, c.PID))
	}
	if c.RunningTransport != "" {
		b.WriteString(fmt.Sprintf("- Transport: %s\n", c.RunningTransport))
	}
	if c.OSServiceName != "" {
		b.WriteString(fmt.Sprintf("- Likely OS service to stop: %s\n", c.OSServiceName))
	}
	b.WriteString("Available actions:\n")
	for i, s := range c.Suggestions {
		b.WriteString(fmt.Sprintf("%d) %s\n", i+1, s))
	}
	return strings.TrimSpace(b.String())
}
