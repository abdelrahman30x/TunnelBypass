package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/hysteria"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"

	"gopkg.in/yaml.v3"
)

// xrayConfigPathForSNI returns the server.json path used for Reality / TLS inbounds for this service.
func xrayConfigPathForSNI(serviceName string) string {
	switch {
	case strings.Contains(serviceName, "SSH-TLS"):
		return filepath.Join(installer.GetConfigDir("ssh-tls"), "server.json")
	case strings.Contains(serviceName, "VLESS-WS"):
		return filepath.Join(installer.GetConfigDir("vless-ws"), "server.json")
	case strings.Contains(serviceName, "GRPC"):
		return filepath.Join(installer.GetConfigDir("vless-grpc"), "server.json")
	default:
		return filepath.Join(installer.GetConfigDir("vless"), "server.json")
	}
}

// dedupeHostnamesOrdered removes duplicates (case-insensitive), keeps first occurrence order.
func dedupeHostnamesOrdered(hosts []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		k := strings.ToLower(h)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, h)
	}
	return out
}

func hostListContainsFold(list []string, host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	k := strings.ToLower(host)
	for _, x := range list {
		if strings.ToLower(strings.TrimSpace(x)) == k {
			return true
		}
	}
	return false
}

func interfaceStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, x := range arr {
		if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// tunnelHostnamesForService returns ordered tunnel hostnames (SNI) from the active config for this service.
func tunnelHostnamesForService(serviceName string) ([]string, error) {
	if strings.Contains(serviceName, "Hysteria") {
		p := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		names, err := hysteria.ReadServerNamesFromServerConfig(p)
		if err != nil {
			return nil, err
		}
		return dedupeHostnamesOrdered(names), nil
	}

	path := xrayConfigPathForSNI(serviceName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		return nil, fmt.Errorf("invalid JSON: %s", path)
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return nil, nil
	}
	ib, ok := inbounds[0].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	stream, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	var raw []string
	if rs, ok := stream["realitySettings"].(map[string]interface{}); ok {
		raw = interfaceStringSlice(rs["serverNames"])
	} else if tls, ok := stream["tlsSettings"].(map[string]interface{}); ok {
		raw = interfaceStringSlice(tls["serverNames"])
	}
	return dedupeHostnamesOrdered(raw), nil
}

// sharingTunnelHostnamesFromConfig returns SNIs used for sharing links (primary + user-added), from _tunnelbypass.sharingSNIs.
func sharingTunnelHostnamesFromConfig(serviceName string) ([]string, error) {
	if strings.Contains(serviceName, "Hysteria") {
		p := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		data = utils.StripUTF8BOM(data)
		var root map[string]interface{}
		if yaml.Unmarshal(data, &root) != nil {
			return nil, fmt.Errorf("invalid hysteria yaml")
		}
		meta, ok := root[host_catalog.MetaKey].(map[string]interface{})
		if !ok {
			return nil, nil
		}
		return dedupeHostnamesOrdered(interfaceStringSlice(meta["sharingSNIs"])), nil
	}
	path := xrayConfigPathForSNI(serviceName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		return nil, fmt.Errorf("invalid json: %s", path)
	}
	meta, ok := cfg[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	return dedupeHostnamesOrdered(interfaceStringSlice(meta["sharingSNIs"])), nil
}

func stringSliceToJSONInterfaces(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

// printConfiguredTunnelHostnames prints hostnames that get sharing links (primary + user-added), not the full Reality catalog.
func printConfiguredTunnelHostnames(serviceName string) {
	sharing, err := sharingTunnelHostnamesFromConfig(serviceName)
	if err != nil {
		fmt.Printf("    %s[!] Could not read sharing host list: %v%s\n", ColorYellow, err, ColorReset)
		return
	}
	if len(sharing) == 0 {
		full, _ := tunnelHostnamesForService(serviceName)
		if len(full) > 0 {
			sharing = []string{full[0]}
		}
	}
	fmt.Printf("\n    %sHostnames for sharing links (primary + added by you):%s\n", ColorBold, ColorReset)
	if len(sharing) == 0 {
		fmt.Printf("    %s(none yet — re-run Setup or add a hostname)%s\n", ColorGray, ColorReset)
		return
	}
	for i, h := range sharing {
		fmt.Printf("    %s%2d)%s %s\n", ColorCyan, i+1, ColorReset, h)
	}
	if full, _ := tunnelHostnamesForService(serviceName); len(full) > len(sharing) && len(sharing) > 0 {
		fmt.Printf("    %s(Config has %d Reality serverNames for SNI matching; only the hostnames above get separate sharing links.)%s\n", ColorGray, len(full), ColorReset)
	}
}
