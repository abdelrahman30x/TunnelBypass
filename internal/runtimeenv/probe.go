package runtimeenv

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ProbeResult is a one-shot snapshot of the host for networking / sysctl / firewall tooling.
type ProbeResult struct {
	OS                string
	Root              bool // Linux: effective UID 0; Windows: admin session
	LikelyContainer   bool
	IptablesPath      string
	NftPath           string
	FirewallBackend   string // iptables, nft, both, none (best-effort label)
	SysctlDirWritable bool // /etc/sysctl.d can create a file (same as ApplyLinuxTransit needs)
	FirewalldRunning  bool
	UFWActive         bool
	SELinuxEnforcing  bool
}

// Probe collects environment hints. Safe on all OSes; cheap to call at startup.
func Probe() ProbeResult {
	p := ProbeResult{OS: runtime.GOOS}
	switch runtime.GOOS {
	case "linux":
		p.Root = os.Geteuid() == 0
	case "windows":
		p.Root = isWindowsAdmin()
	default:
		p.Root = os.Geteuid() == 0
	}
	p.LikelyContainer = likelyContainer()

	if runtime.GOOS == "linux" {
		if path, err := exec.LookPath("iptables"); err == nil {
			p.IptablesPath = path
		}
		if path, err := exec.LookPath("nft"); err == nil {
			p.NftPath = path
		}
		p.FirewallBackend = classifyFirewallBackend(p.IptablesPath, p.NftPath)
		p.SysctlDirWritable = sysctlDirWritable()
		p.FirewalldRunning = firewalldRunningProbe()
		p.UFWActive = ufwActiveProbe()
		p.SELinuxEnforcing = selinuxEnforcingProbe()
	} else {
		p.FirewallBackend = "n/a"
	}

	return p
}

func classifyFirewallBackend(iptablesPath, nftPath string) string {
	hasI := iptablesPath != ""
	hasN := nftPath != ""
	switch {
	case hasI && hasN:
		return "iptables+nft"
	case hasI:
		return "iptables"
	case hasN:
		return "nft"
	default:
		return "none"
	}
}

// sysctlDirWritable is true if we can create a temporary file under /etc/sysctl.d (root typically).
func sysctlDirWritable() bool {
	const dir = "/etc/sysctl.d"
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".tunnelbypass-probe-*")
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return true
}

// WriteProbeSummary writes a compact, human-readable line (e.g. stderr or log).
func WriteProbeSummary(w io.Writer, p ProbeResult) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "[tunnelbypass env] os=%s root=%v container=%v firewall=%s iptables=%q nft=%q sysctl.d_writable=%v firewalld=%v ufw_active=%v selinux_enforcing=%v\n",
		p.OS, p.Root, p.LikelyContainer, p.FirewallBackend, p.IptablesPath, p.NftPath, p.SysctlDirWritable,
		p.FirewalldRunning, p.UFWActive, p.SELinuxEnforcing)
}

// FormatProbeForDebug returns one line suitable for debug.Logf.
func FormatProbeForDebug(p ProbeResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "env probe: os=%s root=%v container=%v firewall=%s", p.OS, p.Root, p.LikelyContainer, p.FirewallBackend)
	if p.IptablesPath != "" {
		fmt.Fprintf(&b, " iptables=%s", p.IptablesPath)
	}
	if p.NftPath != "" {
		fmt.Fprintf(&b, " nft=%s", p.NftPath)
	}
	if runtime.GOOS == "linux" {
		fmt.Fprintf(&b, " sysctl.d_writable=%v firewalld=%v ufw_active=%v selinux=%v", p.SysctlDirWritable, p.FirewalldRunning, p.UFWActive, p.SELinuxEnforcing)
	}
	return b.String()
}

func firewalldRunningProbe() bool {
	if _, err := exec.LookPath("firewall-cmd"); err != nil {
		return false
	}
	return exec.Command("firewall-cmd", "--state").Run() == nil
}

func ufwActiveProbe() bool {
	if _, err := exec.LookPath("ufw"); err != nil {
		return false
	}
	out, _ := exec.Command("ufw", "status").CombinedOutput()
	return strings.Contains(string(out), "Status: active")
}

func selinuxEnforcingProbe() bool {
	data, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == "1"
}

// DropInSysctlPath is the path used by installer Linux transit (for docs / checks).
func DropInSysctlPath() string {
	return filepath.Join("/etc/sysctl.d", "99-tunnelbypass.conf")
}
