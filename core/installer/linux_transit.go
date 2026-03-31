package installer

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
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

// ApplyLinuxTransitNetworking configures sysctl (forwarding, BBR, fq), prepends
// Google DNS to /etc/resolv.conf when missing, and adds transit-only iptables rules
// (mangle MSS clamp, NAT masquerade, FORWARD accept). It never flushes chains,
// never changes default policies, and never touches the INPUT chain.
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

	if err := ensureResolvGoogleDNS(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit resolv.conf: %v\n", err)
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

func ensureResolvGoogleDNS() error {
	const path = "/etc/resolv.conf"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if strings.Contains(string(data), "8.8.8.8") {
		return nil
	}
	// Prefer systemd-resolved API when available (avoids breaking resolv.conf symlinks).
	if tryResolvectlDNS() {
		return nil
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		fmt.Fprintf(os.Stderr, "[!] Skipped editing /etc/resolv.conf (symlink). Try: resolvectl dns <iface> 8.8.8.8 or configure systemd-resolved.\n")
		return nil
	}
	newData := append([]byte("nameserver 8.8.8.8\n"), data...)
	if err := os.WriteFile(path, newData, fi.Mode().Perm()); err != nil {
		return err
	}
	fmt.Printf("[*] Prepended nameserver 8.8.8.8 to /etc/resolv.conf.\n")
	return nil
}

// tryResolvectlDNS sets 8.8.8.8 on the default IPv4 interface via resolvectl when possible.
func tryResolvectlDNS() bool {
	if _, err := exec.LookPath("resolvectl"); err != nil {
		return false
	}
	iface := defaultIPv4Interface()
	if iface == "" {
		return false
	}
	cmd := exec.Command("resolvectl", "dns", iface, "8.8.8.8")
	if err := cmd.Run(); err != nil {
		return false
	}
	fmt.Printf("[*] Set DNS 8.8.8.8 on interface %s via resolvectl (systemd-resolved).\n", iface)
	return true
}

func defaultIPv4Interface() string {
	out, err := exec.Command("sh", "-c", "ip -4 route show default 2>/dev/null | awk '{print $5}' | head -1").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
