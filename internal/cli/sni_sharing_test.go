package cli

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tunnelbypass/core/installer"
	"tunnelbypass/tools/host_catalog"
)

func writeTestVLESSRealityServerConfig(t *testing.T, cfgPath string, sharing []interface{}, serverNames []interface{}) {
	t.Helper()
	root := map[string]interface{}{
		host_catalog.MetaKey: map[string]interface{}{
			"version":               1,
			"sharingSNIs":           sharing,
			"realityDest":           "dest.example.com:443",
			"realityDestHost":       "dest.example.com",
			"realityDestExtraHosts": []interface{}{"extra-dest.example.com"},
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"port": float64(443),
				"settings": map[string]interface{}{
					"clients": []interface{}{
						map[string]interface{}{
							"id": "11111111-1111-1111-1111-111111111111",
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"realitySettings": map[string]interface{}{
						"dest":        "dest.example.com:443",
						"privateKey":  "",
						"publicKey":   "test-public-key",
						"shortIds":    []interface{}{"abcd", ""},
						"serverNames": serverNames,
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
}

func TestAddNewSNIForRealityWritesConfigAndRestarts(t *testing.T) {
	dir := t.TempDir()
	installer.SetDataRootOverride(dir)
	t.Cleanup(func() { installer.SetDataRootOverride("") })

	cfgDir := installer.GetConfigDir("vless")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "server.json")
	writeTestVLESSRealityServerConfig(t, cfgPath,
		[]interface{}{"primary.example.com"},
		[]interface{}{"primary.example.com", "one.one.one.one", "www.facebook.com"},
	)

	oldRestart := restartXrayAfterSNIChange
	restarted := false
	restartXrayAfterSNIChange = func(serviceName, configPath string, port int) error {
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
		return nil
	}
	t.Cleanup(func() { restartXrayAfterSNIChange = oldRestart })

	addNewSNIForService(bufio.NewReader(strings.NewReader("added.example.com\n")), "TunnelBypass-VLESS")

	if !restarted {
		t.Fatal("expected Xray restart after adding SNI")
	}
	sharing, err := sharingTunnelHostnamesFromConfig("TunnelBypass-VLESS")
	if err != nil {
		t.Fatal(err)
	}
	wantSharing := []string{"primary.example.com", "added.example.com"}
	if strings.Join(sharing, ",") != strings.Join(wantSharing, ",") {
		t.Fatalf("sharingSNIs got %#v want %#v", sharing, wantSharing)
	}

	cfg, err := readJSONConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := firstRealitySettings(cfg)
	if err != nil {
		t.Fatal(err)
	}
	serverNames := interfaceStringSlice(rs["serverNames"])
	for _, want := range []string{"primary.example.com", "added.example.com", "dest.example.com", "extra-dest.example.com"} {
		if !hostListContainsFold(serverNames, want) {
			t.Fatalf("serverNames missing %q: %#v", want, serverNames)
		}
	}
}

func TestVLESSRealityShareLinksUseSharingSNIsOnly(t *testing.T) {
	dir := t.TempDir()
	installer.SetDataRootOverride(dir)
	t.Cleanup(func() { installer.SetDataRootOverride("") })

	cfgDir := installer.GetConfigDir("vless")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(cfgDir, "server.json")
	writeTestVLESSRealityServerConfig(t, cfgPath,
		[]interface{}{"primary.example.com", "added.example.com"},
		[]interface{}{"primary.example.com", "added.example.com", "one.one.one.one", "www.facebook.com"},
	)

	links, err := vlessRealityShareLinksForService("TunnelBypass-VLESS", "203.0.113.9")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d share links, want 2: %#v", len(links), links)
	}
	got := links[0].Label + "\n" + links[0].URL + "\n" + links[1].Label + "\n" + links[1].URL
	for _, want := range []string{"primary.example.com", "added.example.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("share links missing %q:\n%s", want, got)
		}
	}
	for _, notWant := range []string{"one.one.one.one", "www.facebook.com"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("share links leaked catalog/serverName %q:\n%s", notWant, got)
		}
	}
}
