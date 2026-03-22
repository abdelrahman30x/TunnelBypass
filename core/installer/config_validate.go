package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateTunnelConfigFile checks the config file is non-empty and, for JSON configs, syntactically valid.
// YAML (e.g. Hysteria) is not fully validated here to avoid pulling a YAML dependency.
func ValidateTunnelConfigFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config missing: %w (create it or fix -config path)", err)
		}
		if os.IsPermission(err) {
			return fmt.Errorf("cannot read config (permission denied): %w", err)
		}
		return err
	}
	trim := bytes.TrimSpace(b)
	if bytes.HasPrefix(trim, []byte{0xef, 0xbb, 0xbf}) {
		trim = trim[3:]
	}
	if len(trim) == 0 {
		return fmt.Errorf("config file is empty: %s", path)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".yaml" || ext == ".yml" {
		return nil
	}
	if trim[0] == '{' || trim[0] == '[' {
		if !json.Valid(trim) {
			return fmt.Errorf("invalid JSON in %s (file may be corrupted; restore from backup)", path)
		}
	}
	return nil
}
