package vless

import (
	"fmt"
	"path/filepath"
	"tunnelbypass/core/installer"
)

// InstallXrayService installs XRay via the TunnelBypass service wrapper
func InstallXrayService(serviceName, configPath string, port int) error {
	xrayPath, err := installer.EnsureBinary("xray")
	if err != nil {
		return err
	}

	absConfig, _ := filepath.Abs(configPath)

	if err := installer.CreateService(
		serviceName,
		serviceName+" (Xray)",
		xrayPath,
		[]string{"run", "-config", absConfig},
		installer.GetBaseDir(),
	); err != nil {
		return fmt.Errorf("failed to create xray service: %v", err)
	}

	if port > 0 {
		_ = installer.OpenFirewallPort(port, "tcp", serviceName)
	}

	return nil
}

func UninstallXrayService(serviceName string) error {
	installer.UninstallService(serviceName)
	return nil
}
