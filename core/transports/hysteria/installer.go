package hysteria

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"tunnelbypass/core/installer"
)

// InstallHysteriaService registers hysteria with the TunnelBypass service wrapper.
func InstallHysteriaService(serviceName, configPath string, port int) error {
	hyPath, err := installer.EnsureBinary("hysteria")
	if err != nil {
		return err
	}

	absConfig, _ := filepath.Abs(configPath)
	_ = sanitizeObfsInServerConfig(absConfig)

	if err := installer.CreateService(
		serviceName,
		serviceName+" (Hysteria)",
		hyPath,
		[]string{"server", "-c", absConfig},
		installer.GetBaseDir(),
	); err != nil {
		return fmt.Errorf("failed to create hysteria service: %v", err)
	}

	if port > 0 {
		_ = installer.OpenFirewallPort(port, "udp", serviceName)
	}

	return nil
}

// sanitizeObfsInServerConfig fixes legacy/invalid obfs blocks that crash Hysteria v2.
func sanitizeObfsInServerConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	obfs, ok := root["obfs"].(map[string]interface{})
	if !ok {
		return nil
	}
	if t, _ := obfs["type"].(string); t != "salamander" {
		return nil
	}
	salamander, ok := obfs["salamander"].(map[string]interface{})
	if !ok {
		delete(root, "obfs")
	} else {
		psk, _ := salamander["password"].(string)
		if len(psk) < 4 {
			delete(root, "obfs")
		}
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

func UninstallHysteriaService(serviceName string) error {
	installer.UninstallService(serviceName)
	return nil
}
