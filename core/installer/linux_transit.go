package installer

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	linuxTransitSysctlPath = "/etc/sysctl.d/99-tunnelbypass.conf"
	linuxTransitSysctlBody = `# TunnelBypass — transit / performance (managed idempotently; do not flush INPUT).
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
net.ipv6.conf.default.forwarding=1
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
`
)

// ApplyLinuxTransitNetworking configures sysctl (forwarding, BBR, fq), DNS (systemd-resolved /
// resolvectl or resolv.conf fallback), OUTPUT policy ACCEPT, and transit iptables/ip6tables
// (mangle MSS, NAT masquerade, FORWARD accept). It never flushes chains and never touches INPUT.
//
// Requires root on Linux; otherwise it is a no-op. Idempotent: safe to run on every install.
// Called after VLESS/Xray, Hysteria v2, and WireGuard (Linux) service setup — before inbound firewall opens.
func ApplyLinuxTransitNetworking() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	if os.Getenv("TUNNELBYPASS_SKIP_LINUX_TRANSIT") == "1" {
		return nil
	}
	if _, err := exec.LookPath("iptables"); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit: iptables not found; skipping mangle/nat/FORWARD rules.\n")
	}

	if err := ensureLinuxTransitSysctl(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit sysctl: %v\n", err)
	}

	// Allow outbound HTTPS/DNS for IP detection and resolvectl (before DNS healing).
	if err := ensureOutputPolicyAccept(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit OUTPUT policy: %v\n", err)
	}

	if err := ensureDNSLinuxTransit(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit DNS: %v\n", err)
	}

	if err := ensureIptablesTransit(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit iptables: %v\n", err)
	}
	if err := ensureIP6TablesTransit(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit ip6tables: %v\n", err)
	}

	warnSELinuxIfEnforcing()

	return nil
}

func ensureLinuxTransitSysctl() error {
	want := strings.TrimSuffix(strings.TrimSpace(linuxTransitSysctlBody), "\n") + "\n"
	needWrite := true
	if existing, err := os.ReadFile(linuxTransitSysctlPath); err == nil {
		if bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace([]byte(want))) {
			needWrite = false
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if needWrite {
		_ = os.MkdirAll("/etc/sysctl.d", 0755)
		if err := os.WriteFile(linuxTransitSysctlPath, []byte(want), 0644); err != nil {
			return err
		}
		fmt.Printf("[*] Wrote %s (sysctl tuning).\n", linuxTransitSysctlPath)
	}
	// Apply: user requested sysctl --system; BBR may log errors on older kernels (non-fatal).
	_ = exec.Command("sysctl", "-p", linuxTransitSysctlPath).Run()
	if err := exec.Command("sysctl", "--system").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] sysctl --system returned %v (check kernel BBR / fq support).\n", err)
	}
	return nil
}

// dnsResolutionHealthy is true when a normal hostname lookup succeeds (skip DNS surgery if OK).
func dnsResolutionHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, "example.com")
	return err == nil
}

func ensureSystemdResolved() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}
	// Best-effort: Ubuntu/Debian use systemd-resolved for resolvectl.
	_ = exec.Command("systemctl", "enable", "--now", "systemd-resolved.service").Run()
	_ = exec.Command("systemctl", "daemon-reload").Run()
}

// ensureDNSLinuxTransit configures DNS via systemd-resolved when needed; avoids edits if lookups already work.
func ensureDNSLinuxTransit() error {
	if dnsResolutionHealthy() {
		return nil
	}

	ensureSystemdResolved()

	iface := defaultIPv4Interface()
	if iface == "" {
		// Still try legacy file path
		return legacyResolvFallback()
	}

	if tryResolvectlDNSFull(iface) {
		time.Sleep(2 * time.Second)
		return nil
	}

	return legacyResolvFallback()
}

// tryResolvectlDNSFull sets DNS + search domain on the interface (systemd-resolved runtime).
func tryResolvectlDNSFull(iface string) bool {
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return false
	}
	cmd := exec.Command("resolvectl", "dns", iface, "8.8.8.8", "1.1.1.1")
	if err := cmd.Run(); err != nil {
		return false
	}
	_ = exec.Command("resolvectl", "domain", iface, "~.").Run()
	fmt.Printf("[*] resolvectl dns %s 8.8.8.8 1.1.1.1 (systemd-resolved).\n", iface)
	return true
}

func defaultIPv4Interface() string {
	// Match: ip route | grep default | awk '{print $5}' (IPv4 default route dev)
	out, err := exec.Command("sh", "-c", "ip route 2>/dev/null | grep default | awk '{print $5}' | head -1").Output()
	if err == nil {
		if iface := strings.TrimSpace(string(out)); iface != "" {
			return iface
		}
	}
	out, err = exec.Command("sh", "-c", "ip -4 route show default 2>/dev/null | awk '{print $5}' | head -1").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func legacyResolvFallback() error {
	const path = "/etc/resolv.conf"
	body := "nameserver 8.8.8.8\nnameserver 1.1.1.1\n"
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(body), 0644); err != nil {
				return err
			}
			fmt.Printf("[*] Created %s with nameservers 8.8.8.8, 1.1.1.1.\n", path)
			return nil
		}
		return nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		fmt.Fprintf(os.Stderr, "[!] Skipped writing /etc/resolv.conf (symlink); use resolvectl or configure systemd-resolved.\n")
		return nil
	}
	if err := os.WriteFile(path, []byte(body), fi.Mode().Perm()); err != nil {
		return err
	}
	fmt.Printf("[*] Wrote static resolv.conf with nameservers 8.8.8.8, 1.1.1.1.\n")
	return nil
}

// ensureOutputPolicyAccept sets filter OUTPUT default to ACCEPT so outbound HTTPS/DNS for IP detection is not blocked.
func ensureOutputPolicyAccept() error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil
	}
	if err := exec.Command("iptables", "-P", "OUTPUT", "ACCEPT").Run(); err != nil {
		return err
	}
	fmt.Printf("[*] iptables: OUTPUT policy ACCEPT\n")
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return nil
	}
	if err := exec.Command("ip6tables", "-P", "OUTPUT", "ACCEPT").Run(); err != nil {
		return fmt.Errorf("ip6tables OUTPUT: %w", err)
	}
	fmt.Printf("[*] ip6tables: OUTPUT policy ACCEPT\n")
	return nil
}

func warnSELinuxIfEnforcing() {
	data, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return
	}
	if strings.TrimSpace(string(data)) != "1" {
		return
	}
	fmt.Fprintf(os.Stderr, "[!] SELinux is enforcing: if the tunnel binary is blocked, check audit logs (AVC) and file labels; AppArmor may similarly restrict profiles on Ubuntu.\n")
}

// mangle: MSS clamp on forwarded TCP (mobile / PPPoE path MTU issues).
// nat: masquerade egress for forwarded traffic.
// filter: allow forwarding (transit only; does not touch INPUT).
func ensureIptablesTransit() error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil
	}

	mangleArgs := []string{
		"-t", "mangle", "-C", "FORWARD",
		"-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN",
		"-j", "TCPMSS", "--clamp-mss-to-pmtu",
	}
	if err := exec.Command("iptables", mangleArgs...).Run(); err != nil {
		// -C uses -t mangle -C FORWARD ...; -I uses -t mangle -I FORWARD 1 ... + match from -p onward.
		insert := append([]string{"iptables", "-t", "mangle", "-I", "FORWARD", "1"}, mangleArgs[4:]...)
		if err := exec.Command(insert[0], insert[1:]...).Run(); err != nil {
			return fmt.Errorf("mangle TCPMSS: %w", err)
		}
		fmt.Printf("[*] iptables mangle FORWARD: TCPMSS --clamp-mss-to-pmtu\n")
	}

	natArgs := []string{"-t", "nat", "-C", "POSTROUTING", "-j", "MASQUERADE"}
	if err := exec.Command("iptables", natArgs...).Run(); err != nil {
		insert := []string{"iptables", "-t", "nat", "-I", "POSTROUTING", "1", "-j", "MASQUERADE"}
		if err := exec.Command(insert[0], insert[1:]...).Run(); err != nil {
			return fmt.Errorf("nat MASQUERADE: %w", err)
		}
		fmt.Printf("[*] iptables nat POSTROUTING: MASQUERADE\n")
	}

	fwdArgs := []string{"-C", "FORWARD", "-j", "ACCEPT"}
	if err := exec.Command("iptables", fwdArgs...).Run(); err != nil {
		insert := []string{"iptables", "-I", "FORWARD", "1", "-j", "ACCEPT"}
		if err := exec.Command(insert[0], insert[1:]...).Run(); err != nil {
			return fmt.Errorf("filter FORWARD ACCEPT: %w", err)
		}
		fmt.Printf("[*] iptables filter FORWARD: ACCEPT\n")
	}

	return nil
}

// ensureIP6TablesTransit mirrors ensureIptablesTransit for IPv6 when ip6tables exists (no INPUT changes).
func ensureIP6TablesTransit() error {
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return nil
	}

	mangleArgs := []string{
		"-t", "mangle", "-C", "FORWARD",
		"-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN",
		"-j", "TCPMSS", "--clamp-mss-to-pmtu",
	}
	if err := exec.Command("ip6tables", mangleArgs...).Run(); err != nil {
		insert := append([]string{"ip6tables", "-t", "mangle", "-I", "FORWARD", "1"}, mangleArgs[4:]...)
		if err := exec.Command(insert[0], insert[1:]...).Run(); err != nil {
			return fmt.Errorf("ip6tables mangle TCPMSS: %w", err)
		}
		fmt.Printf("[*] ip6tables mangle FORWARD: TCPMSS --clamp-mss-to-pmtu\n")
	}

	// IPv6 SNAT/NAT is not always compiled in; treat as best-effort (forward + MSS still apply).
	natArgs := []string{"-t", "nat", "-C", "POSTROUTING", "-j", "MASQUERADE"}
	if err := exec.Command("ip6tables", natArgs...).Run(); err != nil {
		insert := []string{"ip6tables", "-t", "nat", "-I", "POSTROUTING", "1", "-j", "MASQUERADE"}
		if err2 := exec.Command(insert[0], insert[1:]...).Run(); err2 != nil {
			fmt.Fprintf(os.Stderr, "[!] ip6tables nat MASQUERADE skipped (kernel may lack IPv6 NAT table): %v\n", err2)
		} else {
			fmt.Printf("[*] ip6tables nat POSTROUTING: MASQUERADE\n")
		}
	}

	fwdArgs := []string{"-C", "FORWARD", "-j", "ACCEPT"}
	if err := exec.Command("ip6tables", fwdArgs...).Run(); err != nil {
		insert := []string{"ip6tables", "-I", "FORWARD", "1", "-j", "ACCEPT"}
		if err := exec.Command(insert[0], insert[1:]...).Run(); err != nil {
			return fmt.Errorf("ip6tables filter FORWARD ACCEPT: %w", err)
		}
		fmt.Printf("[*] ip6tables filter FORWARD: ACCEPT\n")
	}

	return nil
}
