package xdns

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/types"
)

// InstallXDNSService installs the Xray XDNS service (VLESS + mKCP on UDP 53).
func InstallXDNSService(serviceName, cfgPath string, port int, opt types.ConfigOptions) error {
	warnWindowsICSIfNeeded(port)

	if err := vless.EnsureInboundListenIPv4(cfgPath); err != nil {
		return err
	}
	xrayPath, err := installer.EnsureBinary("xray")
	if err != nil {
		return err
	}
	if err := installer.CreateService(serviceName, serviceName+" (Xray XDNS)", xrayPath, []string{"run", "-config", cfgPath}, installer.GetBaseDir()); err != nil {
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
		fmt.Println("    If XDNS fails to bind port 53, disable ICS from services.msc or run:")
		fmt.Println("        net stop SharedAccess")
	}
}
