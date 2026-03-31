package hysteria

import (
	"fmt"
	"os"

	"tunnelbypass/internal/utils"

	"gopkg.in/yaml.v3"
)

// EnsureServerYAML normalizes hysteria server.yaml for IPv4 wildcard binds and
// fixes legacy obfs blocks that crash Hysteria v2. Idempotent; no-op if file missing.
func EnsureServerYAML(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data = utils.StripUTF8BOM(data)
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	changed := false

	if l, ok := root["listen"].(string); ok {
		if utils.ListenAddrNeedsIPv4WildcardFix(l) {
			port := 443
			if p, ok := utils.ListenPortFromField(l); ok {
				port = p
			}
			root["listen"] = fmt.Sprintf("0.0.0.0:%d", port)
			changed = true
		}
	} else if root["listen"] == nil {
		root["listen"] = "0.0.0.0:443"
		changed = true
	}

	if obfs, ok := root["obfs"].(map[string]interface{}); ok {
		if t, _ := obfs["type"].(string); t == "salamander" {
			salamander, ok := obfs["salamander"].(map[string]interface{})
			if !ok {
				delete(root, "obfs")
				changed = true
			} else {
				psk, _ := salamander["password"].(string)
				if len(psk) < 4 {
					delete(root, "obfs")
					changed = true
				}
			}
		}
	}

	if !changed {
		return nil
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}
