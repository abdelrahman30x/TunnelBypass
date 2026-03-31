package vless

import (
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"

	"tunnelbypass/internal/utils"
)

// EnsureInboundListenIPv4 rewrites server.json inbounds so the main listener
// binds to IPv4 all-interfaces (0.0.0.0) when listen was empty, IPv6 wildcard,
// or [::]:port. Skips localhost / API inbounds. No-op if file missing.
func EnsureInboundListenIPv4(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}
	inbounds, ok := root["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return nil
	}
	changed := false
	for _, raw := range inbounds {
		ib, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := ib["tag"].(string)
		if tag == "api-in" {
			continue
		}
		listen, _ := ib["listen"].(string)
		if strings.HasPrefix(strings.TrimSpace(listen), "127.") {
			continue
		}
		if !utils.ListenAddrNeedsIPv4WildcardFix(listen) {
			continue
		}
		if host, portStr, err := net.SplitHostPort(strings.TrimSpace(listen)); err == nil {
			host = strings.Trim(host, "[]")
			if host == "::" || host == "::0" {
				ib["listen"] = "0.0.0.0"
				if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
					ib["port"] = float64(p)
				}
				changed = true
				continue
			}
			// Malformed or unexpected; do not overwrite a non-wildcard address.
			continue
		}
		ib["listen"] = "0.0.0.0"
		changed = true
	}
	if !changed {
		return nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}
