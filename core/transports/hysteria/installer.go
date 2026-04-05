package hysteria

import (
	"fmt"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
)

// InstallHysteriaService registers hysteria with the TunnelBypass service wrapper.
func InstallHysteriaService(serviceName, configPath string, port int, opt types.ConfigOptions) error {
	hyPath, err := installer.EnsureBinary("hysteria")
	if err != nil {
		return err
	}

	absConfig, _ := filepath.Abs(configPath)
	if err := EnsureServerYAML(absConfig); err != nil {
		return fmt.Errorf("hysteria server config: %w", err)
	}

	if err := installer.CreateService(
		serviceName,
		serviceName+" (Hysteria)",
		hyPath,
		[]string{"server", "-c", absConfig},
		installer.GetBaseDir(),
	); err != nil {
		return fmt.Errorf("failed to create hysteria service: %v", err)
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

func UninstallHysteriaService(serviceName string) error {
	installer.UninstallService(serviceName)
	return nil
}
