package installer

/*
Linux Transit Philosophy:

  1. Application-level DNS first — resolve inside Xray/Hysteria where possible so tunnel users
     get working names without rewriting system-wide DNS (company resolvers stay intact).

  2. Isolated networking — TB_* iptables chains with -m comment for TunnelBypass rules only;
     MASQUERADE uses -o <egress_iface> from the default route so Docker/OVS bridge traffic is not
     accidentally NATed.

  3. Safe rollback — each reversible change appends an explicit inverse line to
     /usr/local/etc/tunnelbypass/.rollback.sh (prefer -D over iptables-save|grep -v|restore).
     Full RunLinuxRollback (sysctl + iptables inverses) from uninstall runs only when no
     TunnelBypass*.service units remain under /etc/systemd/system, so removing one inbound does not
     strip BBR/MSS while another service still runs.

  4. MSS clamping — mangle chain TUNNEL_BYPASS with TCPMSS (--clamp-mss-to-pmtu), jumped from FORWARD and
     OUTPUT on every Linux transit install when iptables is available, to reduce fragmentation without
     requiring --optimize-net. Legacy chain TB_MANGLE is removed best-effort on setup.

  5. Adaptive optimization — sysctl/BBR/fq and optional gai.conf (TUNNELBYPASS_LINUX_GAI) run only when
     optimize-net is on (explicit flag, env, or engine.EvaluateLinuxAutopilot before service install).
     Autopilot uses jitter + median TCP latency thresholds; it does not replace MSS.

  6. Future (phase 2) — optional long-lived sysctl re-apply on sustained packet loss (watcher/timer)
     is out of scope for the install-time path; see project notes / env TUNNELBYPASS_LINUX_SYSCTL_WATCHER.
*/

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

	"tunnelbypass/core/types"
)

const (
	linuxTransitSysctlPath = "/etc/sysctl.d/99-tunnelbypass.conf"
	linuxTransitSysctlBody = `# TunnelBypass v1.3.2 — transit / gaming-friendly sysctl (drop-in; do not edit /etc/sysctl.conf).
# Managed idempotently by TunnelBypass; do not flush INPUT from this file.
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
net.ipv6.conf.default.forwarding=1
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
net.core.somaxconn=4096
net.ipv4.tcp_slow_start_after_idle=0
net.ipv4.tcp_fastopen=3
net.ipv4.tcp_mtu_probing=1
net.core.rmem_max=16777216
net.core.wmem_max=16777216
`
	tbNatChain            = "TB_POSTROUTING"
	tunBypassMangleChain  = "TUNNEL_BYPASS"
	legacyMangleChainName = "TB_MANGLE"
	tbForwardChain        = "TB_FORWARD"
	cmtNAT                = "tunnelbypass-nat"
	cmtMSS                = "tunnelbypass-mss"
	cmtFwd                = "tunnelbypass-forward"
)

type linuxTransitOpts struct {
	OptimizeNet    bool
	DNSFix         bool
	Router         bool
	NoAutoOptimize bool
}

func mergeLinuxTransitOpts(opt types.ConfigOptions) linuxTransitOpts {
	o := linuxTransitOpts{
		OptimizeNet:    opt.LinuxOptimizeNet,
		DNSFix:         opt.LinuxDNSFix,
		Router:         opt.LinuxRouter,
		NoAutoOptimize: opt.LinuxNoAutoOptimize,
	}
	env := func(k string) string { return strings.TrimSpace(os.Getenv(k)) }
	if v := env("TUNNELBYPASS_LINUX_OPTIMIZE_NET"); v == "1" || strings.EqualFold(v, "true") {
		o.OptimizeNet = true
	}
	if v := env("TUNNELBYPASS_LINUX_DNS_FIX"); v == "1" || strings.EqualFold(v, "true") {
		o.DNSFix = true
	}
	if v := env("TUNNELBYPASS_LINUX_ROUTER"); v == "1" || strings.EqualFold(v, "true") {
		o.Router = true
	}
	if v := env("TUNNELBYPASS_NO_AUTO_OPTIMIZE"); v == "1" || strings.EqualFold(v, "true") {
		o.NoAutoOptimize = true
	}
	return o
}

// ApplyLinuxTransitNetworking applies minimal Linux defaults (OUTPUT policy, loopback INPUT), MSS
// mangle rules (TUNNEL_BYPASS) whenever iptables exists, and when optimize-net is set: sysctl tuning
// and optional gai.conf. DNS fixes and router NAT are unchanged. Auto-optimize thresholds are
// evaluated in engine.EvaluateLinuxAutopilot before this runs on service install.
// Requires root on Linux; otherwise no-op. Idempotent for repeated installs.
func ApplyLinuxTransitNetworking(opt types.ConfigOptions) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	if os.Getenv("TUNNELBYPASS_SKIP_LINUX_TRANSIT") == "1" {
		return nil
	}
	o := mergeLinuxTransitOpts(opt)

	if _, err := exec.LookPath("iptables"); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit: iptables not found; skipping mangle/nat/FORWARD rules.\n")
	}

	if err := ensureOutputPolicyAccept(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit OUTPUT policy: %v\n", err)
	}

	if err := ensureLoopbackInputAccept(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit loopback INPUT: %v\n", err)
	}

	if err := ensureDNSLinuxTransit(o); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit DNS: %v\n", err)
	}

	if err := ensureTBChainsMangleIPv4(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit iptables mangle: %v\n", err)
		RunLinuxRollback()
		return err
	}
	if err := ensureIP6TablesMangle(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] Linux transit ip6tables mangle: %v\n", err)
	}

	if o.OptimizeNet {
		if err := ensureLinuxTransitSysctl(); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Linux transit sysctl: %v\n", err)
			return err
		}
		// Single rollback line: remove drop-in first, then reload sysctl defaults.
		_ = AppendRollbackInverse(`sh -c 'rm -f /etc/sysctl.d/99-tunnelbypass.conf && sysctl --system 2>/dev/null || true'`)

		if err := ensureGaiConfIPv4Preference(); err != nil {
			fmt.Fprintf(os.Stderr, "[!] Linux transit gai.conf: %v\n", err)
		}
	}

	if o.Router {
		iface := LinuxEgressInterface()
		if iface == "" {
			fmt.Fprintf(os.Stderr, "[!] Linux router: could not detect egress interface (ip route get 8.8.8.8); skipping NAT.\n")
		} else {
			if err := ensureTBChainsNATAndForwardIPv4(iface); err != nil {
				fmt.Fprintf(os.Stderr, "[!] Linux transit iptables NAT/FORWARD: %v\n", err)
				RunLinuxRollback()
				return err
			}
			if err := ensureIP6NATBestEffort(iface); err != nil {
				fmt.Fprintf(os.Stderr, "[!] Linux transit ip6tables nat: %v\n", err)
			}
			if err := ensureIP6TablesForward(); err != nil {
				fmt.Fprintf(os.Stderr, "[!] Linux transit ip6tables forward: %v\n", err)
			}
		}
	}

	warnSELinuxIfEnforcing()
	return nil
}

func dnsResolutionHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, "google.com")
	return err == nil
}

func ensureDNSLinuxTransit(o linuxTransitOpts) error {
	if dnsResolutionHealthy() {
		fmt.Printf("[OK] DNS is healthy, skipping remap\n")
		return nil
	}
	if !o.DNSFix {
		fmt.Fprintf(os.Stderr, "[!] DNS resolution check failed; use --dns-fix or TUNNELBYPASS_LINUX_DNS_FIX=1 to adjust system DNS (Xray/Hysteria app DNS is already in generated configs).\n")
		return nil
	}

	ensureSystemdResolved()

	iface := defaultIPv4Interface()
	if iface == "" {
		return legacyResolvFallback()
	}

	if tryResolvectlDNSFull(iface) {
		time.Sleep(2 * time.Second)
		return nil
	}

	return legacyResolvFallback()
}

func ensureSystemdResolved() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}
	_ = exec.Command("systemctl", "enable", "--now", "systemd-resolved.service").Run()
	_ = exec.Command("systemctl", "daemon-reload").Run()
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
	_ = exec.Command("sysctl", "-p", linuxTransitSysctlPath).Run()
	if err := exec.Command("sysctl", "--system").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "[!] sysctl --system returned %v (check kernel BBR / fq support).\n", err)
	}
	return nil
}

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

func ensureLoopbackInputAccept() error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil
	}
	check := []string{"iptables", "-C", "INPUT", "-i", "lo", "-j", "ACCEPT"}
	if exec.Command(check[0], check[1:]...).Run() == nil {
		return nil
	}
	if err := exec.Command("iptables", "-I", "INPUT", "1", "-i", "lo", "-j", "ACCEPT").Run(); err != nil {
		return err
	}
	fmt.Printf("[*] iptables: INPUT loopback (-i lo) ACCEPT\n")
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return nil
	}
	check6 := []string{"ip6tables", "-C", "INPUT", "-i", "lo", "-j", "ACCEPT"}
	if exec.Command(check6[0], check6[1:]...).Run() == nil {
		return nil
	}
	_ = exec.Command("ip6tables", "-I", "INPUT", "1", "-i", "lo", "-j", "ACCEPT").Run()
	fmt.Printf("[*] ip6tables: INPUT loopback (-i lo) ACCEPT\n")
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

func ipt4Has(args []string) bool {
	return exec.Command("iptables", args...).Run() == nil
}

func ipt6Has(args []string) bool {
	return exec.Command("ip6tables", args...).Run() == nil
}

// removeLegacyMangleChainBestEffort drops jumps to TB_MANGLE and removes the old chain (upgrade path).
func removeLegacyMangleChainBestEffort(ipt string, legacy string) {
	_ = exec.Command(ipt, "-t", "mangle", "-D", "FORWARD", "-j", legacy).Run()
	_ = exec.Command(ipt, "-t", "mangle", "-D", "OUTPUT", "-j", legacy).Run()
	_ = exec.Command(ipt, "-t", "mangle", "-F", legacy).Run()
	_ = exec.Command(ipt, "-t", "mangle", "-X", legacy).Run()
}

func ensureTBChainsMangleIPv4() error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil
	}
	removeLegacyMangleChainBestEffort("iptables", legacyMangleChainName)

	_ = exec.Command("iptables", "-t", "mangle", "-N", tunBypassMangleChain).Run()

	jumpFwd := []string{"-t", "mangle", "-C", "FORWARD", "-j", tunBypassMangleChain}
	if !ipt4Has(jumpFwd) {
		if err := exec.Command("iptables", "-t", "mangle", "-A", "FORWARD", "-j", tunBypassMangleChain).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables mangle: jump FORWARD -> %s\n", tunBypassMangleChain)
		_ = AppendRollbackInverse("iptables -t mangle -D FORWARD -j " + tunBypassMangleChain)
	}
	jumpOut := []string{"-t", "mangle", "-C", "OUTPUT", "-j", tunBypassMangleChain}
	if !ipt4Has(jumpOut) {
		if err := exec.Command("iptables", "-t", "mangle", "-A", "OUTPUT", "-j", tunBypassMangleChain).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables mangle: jump OUTPUT -> %s\n", tunBypassMangleChain)
		_ = AppendRollbackInverse("iptables -t mangle -D OUTPUT -j " + tunBypassMangleChain)
	}

	rule := []string{"-t", "mangle", "-C", tunBypassMangleChain,
		"-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN",
		"-m", "comment", "--comment", cmtMSS,
		"-j", "TCPMSS", "--clamp-mss-to-pmtu",
	}
	if !ipt4Has(rule) {
		args := []string{"-t", "mangle", "-A", tunBypassMangleChain,
			"-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-m", "comment", "--comment", cmtMSS,
			"-j", "TCPMSS", "--clamp-mss-to-pmtu",
		}
		if err := exec.Command("iptables", args...).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables mangle %s: TCPMSS clamp (comment %s)\n", tunBypassMangleChain, cmtMSS)
		_ = AppendRollbackInverse("iptables -t mangle -D " + tunBypassMangleChain + " -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment " + cmtMSS + " -j TCPMSS --clamp-mss-to-pmtu")
	}
	return nil
}

func ensureIP6TablesMangle() error {
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return nil
	}
	removeLegacyMangleChainBestEffort("ip6tables", legacyMangleChainName)

	_ = exec.Command("ip6tables", "-t", "mangle", "-N", tunBypassMangleChain).Run()
	jumpFwd := []string{"-t", "mangle", "-C", "FORWARD", "-j", tunBypassMangleChain}
	if !ipt6Has(jumpFwd) {
		if err := exec.Command("ip6tables", "-t", "mangle", "-A", "FORWARD", "-j", tunBypassMangleChain).Run(); err != nil {
			return err
		}
		_ = AppendRollbackInverse("ip6tables -t mangle -D FORWARD -j " + tunBypassMangleChain)
	}
	jumpOut := []string{"-t", "mangle", "-C", "OUTPUT", "-j", tunBypassMangleChain}
	if !ipt6Has(jumpOut) {
		if err := exec.Command("ip6tables", "-t", "mangle", "-A", "OUTPUT", "-j", tunBypassMangleChain).Run(); err != nil {
			return err
		}
		_ = AppendRollbackInverse("ip6tables -t mangle -D OUTPUT -j " + tunBypassMangleChain)
	}
	rule := []string{"-t", "mangle", "-C", tunBypassMangleChain,
		"-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN",
		"-m", "comment", "--comment", cmtMSS,
		"-j", "TCPMSS", "--clamp-mss-to-pmtu",
	}
	if !ipt6Has(rule) {
		args := []string{"-t", "mangle", "-A", tunBypassMangleChain,
			"-p", "tcp", "-m", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-m", "comment", "--comment", cmtMSS,
			"-j", "TCPMSS", "--clamp-mss-to-pmtu",
		}
		if err := exec.Command("ip6tables", args...).Run(); err != nil {
			return err
		}
		_ = AppendRollbackInverse("ip6tables -t mangle -D " + tunBypassMangleChain + " -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment " + cmtMSS + " -j TCPMSS --clamp-mss-to-pmtu")
	}
	return nil
}

func ensureTBChainsNATAndForwardIPv4(iface string) error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return nil
	}
	_ = exec.Command("iptables", "-t", "nat", "-N", tbNatChain).Run()

	jump := []string{"-t", "nat", "-C", "POSTROUTING", "-j", tbNatChain}
	if !ipt4Has(jump) {
		if err := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-j", tbNatChain).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables nat: jump POSTROUTING -> %s\n", tbNatChain)
		_ = AppendRollbackInverse("iptables -t nat -D POSTROUTING -j " + tbNatChain)
	}

	natRule := []string{"-t", "nat", "-C", tbNatChain,
		"-o", iface, "-m", "comment", "--comment", cmtNAT, "-j", "MASQUERADE",
	}
	if !ipt4Has(natRule) {
		args := []string{"-t", "nat", "-A", tbNatChain,
			"-o", iface, "-m", "comment", "--comment", cmtNAT, "-j", "MASQUERADE",
		}
		if err := exec.Command("iptables", args...).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables nat %s: MASQUERADE -o %s (%s)\n", tbNatChain, iface, cmtNAT)
		_ = AppendRollbackInverse(fmt.Sprintf("iptables -t nat -D %s -o %s -m comment --comment %s -j MASQUERADE", tbNatChain, iface, cmtNAT))
	}

	_ = exec.Command("iptables", "-N", tbForwardChain).Run()
	fwdJump := []string{"-C", "FORWARD", "-j", tbForwardChain}
	if !ipt4Has(fwdJump) {
		if err := exec.Command("iptables", "-A", "FORWARD", "-j", tbForwardChain).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables filter: jump FORWARD -> %s\n", tbForwardChain)
		_ = AppendRollbackInverse("iptables -D FORWARD -j " + tbForwardChain)
	}
	fwdRule := []string{"-C", tbForwardChain, "-m", "comment", "--comment", cmtFwd, "-j", "ACCEPT"}
	if !ipt4Has(fwdRule) {
		args := []string{"-A", tbForwardChain, "-m", "comment", "--comment", cmtFwd, "-j", "ACCEPT"}
		if err := exec.Command("iptables", args...).Run(); err != nil {
			return err
		}
		fmt.Printf("[*] iptables filter %s: ACCEPT (%s)\n", tbForwardChain, cmtFwd)
		_ = AppendRollbackInverse("iptables -D " + tbForwardChain + " -m comment --comment " + cmtFwd + " -j ACCEPT")
	}
	return nil
}

func ensureIP6NATBestEffort(iface string) error {
	if iface == "" {
		return nil
	}
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return nil
	}
	_ = exec.Command("ip6tables", "-t", "nat", "-N", tbNatChain).Run()
	jump := []string{"-t", "nat", "-C", "POSTROUTING", "-j", tbNatChain}
	if !ipt6Has(jump) {
		if err := exec.Command("ip6tables", "-t", "nat", "-A", "POSTROUTING", "-j", tbNatChain).Run(); err != nil {
			return err
		}
		_ = AppendRollbackInverse("ip6tables -t nat -D POSTROUTING -j " + tbNatChain)
	}
	rule := []string{"-t", "nat", "-C", tbNatChain, "-o", iface, "-m", "comment", "--comment", cmtNAT, "-j", "MASQUERADE"}
	if !ipt6Has(rule) {
		args := []string{"-t", "nat", "-A", tbNatChain, "-o", iface, "-m", "comment", "--comment", cmtNAT, "-j", "MASQUERADE"}
		if err := exec.Command("ip6tables", args...).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[!] ip6tables nat MASQUERADE skipped (kernel may lack IPv6 NAT): %v\n", err)
			return nil
		}
		_ = AppendRollbackInverse(fmt.Sprintf("ip6tables -t nat -D %s -o %s -m comment --comment %s -j MASQUERADE", tbNatChain, iface, cmtNAT))
	}
	return nil
}

func ensureIP6TablesForward() error {
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return nil
	}
	_ = exec.Command("ip6tables", "-N", tbForwardChain).Run()
	jump := []string{"-C", "FORWARD", "-j", tbForwardChain}
	if !ipt6Has(jump) {
		if err := exec.Command("ip6tables", "-A", "FORWARD", "-j", tbForwardChain).Run(); err != nil {
			return err
		}
		_ = AppendRollbackInverse("ip6tables -D FORWARD -j " + tbForwardChain)
	}
	rule := []string{"-C", tbForwardChain, "-m", "comment", "--comment", cmtFwd, "-j", "ACCEPT"}
	if !ipt6Has(rule) {
		args := []string{"-A", tbForwardChain, "-m", "comment", "--comment", cmtFwd, "-j", "ACCEPT"}
		if err := exec.Command("ip6tables", args...).Run(); err != nil {
			return err
		}
		_ = AppendRollbackInverse("ip6tables -D " + tbForwardChain + " -m comment --comment " + cmtFwd + " -j ACCEPT")
	}
	return nil
}
