package installer

import (
	"fmt"
	"os/exec"
	"strings"

	"tunnelbypass/internal/uicolors"
)

// firewallCmdRunning is true when firewalld is active (preferred over raw iptables on many distros).
func firewallCmdRunning() bool {
	if _, err := exec.LookPath("firewall-cmd"); err != nil {
		return false
	}
	return exec.Command("firewall-cmd", "--state").Run() == nil
}

// ufwIsActive is true when `ufw status` reports active (wrapper — rules persist across reloads).
func ufwIsActive() bool {
	if _, err := exec.LookPath("ufw"); err != nil {
		return false
	}
	out, _ := exec.Command("ufw", "status").CombinedOutput()
	return strings.Contains(string(out), "Status: active")
}

// addIptablesInputAllow adds idempotent INPUT allow for IPv4 and IPv6 when no firewalld/ufw manages the stack.
func addIptablesInputAllow(protocol, portStr string) {
	checkInsert := func(bin string) {
		check := []string{bin, "-C", "INPUT", "-p", protocol, "--dport", portStr, "-j", "ACCEPT"}
		if exec.Command(check[0], check[1:]...).Run() == nil {
			return
		}
		_ = exec.Command(bin, "-I", "INPUT", "1", "-p", protocol, "--dport", portStr, "-j", "ACCEPT").Run()
	}
	if _, err := exec.LookPath("iptables"); err == nil {
		checkInsert("iptables")
	}
	if _, err := exec.LookPath("ip6tables"); err == nil {
		checkInsert("ip6tables")
	}
}

// removeIptablesInputAllow best-effort deletes our INPUT allow (IPv4 + IPv6).
func removeIptablesInputAllow(protocol, portStr string) {
	del := func(bin string) {
		_ = exec.Command(bin, "-D", "INPUT", "-p", protocol, "--dport", portStr, "-j", "ACCEPT").Run()
	}
	if _, err := exec.LookPath("iptables"); err == nil {
		del("iptables")
	}
	if _, err := exec.LookPath("ip6tables"); err == nil {
		del("ip6tables")
	}
}

// PrintCloudProviderFirewallHint reminds operators that VPS cloud panels often block ports independently of Linux.
func PrintCloudProviderFirewallHint(port int, protocol string) {
	if port <= 0 {
		return
	}
	p := strings.ToLower(strings.TrimSpace(protocol))
	if p == "" {
		p = "tcp"
	}
	fmt.Printf("\n%s[!] Cloud / VPS:%s If clients cannot connect, open port %d/%s in your provider's Security Group / Firewall dashboard (e.g. AWS, Azure, GCP, Contabo).%s\n",
		uicolors.ColorYellow, uicolors.ColorReset, port, strings.ToUpper(p), uicolors.ColorReset)
}
