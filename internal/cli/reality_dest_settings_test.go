package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tunnelbypass/core/installer"
	"tunnelbypass/tools/host_catalog"
)

func TestSetRealityDestForServiceRewritesServerJSONAndRestarts(t *testing.T) {
	dir := t.TempDir()
	installer.SetDataRootOverride(dir)
	t.Cleanup(func() { installer.SetDataRootOverride("") })

	cfgDir := installer.GetConfigDir("vless")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "server.json")
	root := map[string]interface{}{
		host_catalog.MetaKey: map[string]interface{}{
			"version":     1,
			"sharingSNIs": []interface{}{"old.example.com"},
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"port": float64(443),
				"streamSettings": map[string]interface{}{
					"realitySettings": map[string]interface{}{
						"dest":        "old.example.com:443",
						"serverNames": []interface{}{"old.example.com"},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	oldRestart := restartXrayAfterRealityDestChange
	restarted := false
	restartXrayAfterRealityDestChange = func(serviceName, configPath string, port int) error {
		restarted = true
		if serviceName != "TunnelBypass-VLESS" {
			t.Fatalf("serviceName got %q", serviceName)
		}
		if configPath != cfgPath {
			t.Fatalf("configPath got %q want %q", configPath, cfgPath)
		}
		if port != 443 {
			t.Fatalf("port got %d", port)
		}
		if err := verifyXrayRealityDest(configPath, "new.example.com:443"); err != nil {
			t.Fatal(err)
		}
		return nil
	}
	t.Cleanup(func() { restartXrayAfterRealityDestChange = oldRestart })

	if err := setRealityDestForService("TunnelBypass-VLESS", "new.example.com", "new.example.com:443", []string{"new.example.com"}); err != nil {
		t.Fatal(err)
	}
	if !restarted {
		t.Fatal("service restart was not called")
	}

	out, err := readJSONConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := firstRealitySettings(out)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := rs["dest"].(string); got != "new.example.com:443" {
		t.Fatalf("dest got %q", got)
	}
	meta := ensureMeta(out)
	if got, _ := meta["realityDestHost"].(string); got != "new.example.com" {
		t.Fatalf("metadata host got %q", got)
	}
	prefs, err := host_catalog.LoadRealityDestPrefsFile(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.PreferredHost != "new.example.com" {
		t.Fatalf("prefs preferred host got %q", prefs.PreferredHost)
	}
}

func TestServerConfigPathForService(t *testing.T) {
	dir := t.TempDir()
	installer.SetDataRootOverride(dir)
	t.Cleanup(func() { installer.SetDataRootOverride("") })

	tests := []struct {
		service string
		wantEnd string
	}{
		{"TunnelBypass-VLESS", filepath.Join("configs", "vless", "server.json")},
		{"TunnelBypass-VLESS-WS", filepath.Join("configs", "vless-ws", "server.json")},
		{"TunnelBypass-VLESS-GRPC", filepath.Join("configs", "vless-grpc", "server.json")},
		{"TunnelBypass-Hysteria", filepath.Join("configs", "hysteria", "server.yaml")},
		{"TunnelBypass-Shadowsocks", filepath.Join("configs", "shadowsocks", "server.json")},
		{"TunnelBypass-MasterDnsVPN", filepath.Join("configs", "mdns", "server_config.toml")},
		{"TunnelBypass-SSL", filepath.Join("configs", "stunnel", "stunnel-server.conf")},
		{"TunnelBypass-WSS", filepath.Join("configs", "wstunnel", "wss_tunnel_instructions.txt")},
		{"TunnelBypass-SSH", filepath.Join("configs", "ssh", "ssh_tunnel_instructions.txt")},
	}
	for _, tc := range tests {
		got, ok := serverConfigPathForService(tc.service)
		if !ok {
			t.Fatalf("%s: expected server config path", tc.service)
		}
		if !strings.HasSuffix(got, tc.wantEnd) {
			t.Fatalf("%s: got %q, want suffix %q", tc.service, got, tc.wantEnd)
		}
	}

	if _, ok := serverConfigPathForService("NoSuchService"); ok {
		t.Fatal("unknown service should not report a server config path")
	}
}

func TestServiceHasConfigSettingsBeyondRealityDest(t *testing.T) {
	dir := t.TempDir()
	installer.SetDataRootOverride(dir)
	t.Cleanup(func() { installer.SetDataRootOverride("") })

	for _, service := range []string{"TunnelBypass-Hysteria", "TunnelBypass-Shadowsocks", "TunnelBypass-SSH"} {
		if !serviceHasConfigSettings(service) {
			t.Fatalf("%s should expose Config Settings for opening its server file", service)
		}
	}
}
