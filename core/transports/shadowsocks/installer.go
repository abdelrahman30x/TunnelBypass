package shadowsocks

import (
	"fmt"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
)

// InstallShadowsocksService registers shadowsocks-rust (ssserver) with the TunnelBypass service wrapper.
func InstallShadowsocksService(serviceName, configPath string, port int, opt types.ConfigOptions) error {
	ssPath, err := installer.EnsureBinary("shadowsocks")
	if err != nil {
		return err
	}

	absConfig, _ := filepath.Abs(configPath)

	if err := installer.CreateService(
		serviceName,
		serviceName+" (Shadowsocks)",
		ssPath,
		[]string{"-c", absConfig},
		installer.GetBaseDir(),
	); err != nil {
		return fmt.Errorf("failed to create shadowsocks service: %v", err)
	}

	if err := installer.ApplyLinuxTransitNetworking(opt); err != nil {
		installer.RunLinuxRollback()
		return err
	}

	if port > 0 {
		// Shadowsocks uses both TCP and UDP
		_ = installer.OpenFirewallPort(port, "tcp", serviceName)
		_ = installer.OpenFirewallPort(port, "udp", serviceName)
		installer.PrintCloudProviderFirewallHint(port, "tcp+udp")
	}

	return nil
}

// UninstallShadowsocksService stops and removes the Shadowsocks OS service.
func UninstallShadowsocksService(serviceName string) error {
	installer.UninstallService(serviceName)
	return nil
}
