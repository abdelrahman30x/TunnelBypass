package mdns

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
)

// InstallMDNSService installs the MasterDnsVPN server service.
func InstallMDNSService(serviceName, cfgPath string, port int, opt types.ConfigOptions) error {
	warnWindowsICSIfNeeded(port)

	if runtime.GOOS == "linux" {
		if err := ensureLinuxPort53Available(); err != nil {
			return err
		}
	}

	exe, err := installer.EnsureBinary("masterdnsvpn-server")
	if err != nil {
		return err
	}

	if err := installer.CreateService(serviceName, serviceName+" (MasterDnsVPN)", exe, []string{"-config", cfgPath}, installer.GetBaseDir()); err != nil {
		return err
	}

	if err := installer.ApplyLinuxTransitNetworking(opt); err != nil {
		installer.RunLinuxRollback()
		return err
	}

	if port > 0 {
		_ = installer.OpenFirewallPort(port, "udp", serviceName)
		installer.PrintCloudProviderFirewallHint(port, "udp")
	}

	return nil
}

func warnWindowsICSIfNeeded(port int) {
	if runtime.GOOS != "windows" || port != 53 {
		return
	}
	out, _ := exec.Command("sc", "query", "SharedAccess").CombinedOutput()
	if strings.Contains(string(out), "RUNNING") {
		fmt.Println("\n[!] Windows Internet Connection Sharing (ICS) is running and reserves UDP port 53.")
		fmt.Println("    If MasterDnsVPN fails to bind port 53, disable ICS from services.msc or run:")
		fmt.Println("        net stop SharedAccess")
	}
}

func ensureLinuxPort53Available() error {
	// Check systemd-resolved
	out, _ := exec.Command("systemctl", "is-active", "systemd-resolved").CombinedOutput()
	if strings.TrimSpace(string(out)) == "active" {
		// Check if it's using DNSStubListener
		resolvedConf, _ := exec.Command("cat", "/etc/systemd/resolved.conf").CombinedOutput()
		confStr := string(resolvedConf)
		if strings.Contains(confStr, "DNSStubListener=yes") || !strings.Contains(confStr, "DNSStubListener=no") {
			return fmt.Errorf(`systemd-resolved is using UDP port 53.

To free port 53, edit /etc/systemd/resolved.conf:
    DNSStubListener=no
Then run: sudo systemctl restart systemd-resolved

Alternatively, disable systemd-resolved entirely:
    sudo systemctl disable --now systemd-resolved

Other common DNS services that may conflict:
    dnsmasq    -> sudo systemctl stop dnsmasq
    bind9      -> sudo systemctl stop bind9
    named      -> sudo systemctl stop named
    unbound    -> sudo systemctl stop unbound`)
		}
	}

	// Check other common DNS services
	services := []string{"dnsmasq", "bind9", "named", "unbound"}
	for _, svc := range services {
		out, _ := exec.Command("systemctl", "is-active", svc).CombinedOutput()
		if strings.TrimSpace(string(out)) == "active" {
			return fmt.Errorf("service %q is running and may be using UDP port 53. Stop it with: sudo systemctl stop %s", svc, svc)
		}
	}

	return nil
}
