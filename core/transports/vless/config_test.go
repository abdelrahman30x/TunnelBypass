package vless

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"tunnelbypass/core/layout"
	"tunnelbypass/core/types"
	"tunnelbypass/tools/host_catalog"
)

func TestGenerateServerConfigWritesPerConfigRealityDestPrefs(t *testing.T) {
	dir := t.TempDir()
	layout.SetDataRootOverride(dir)
	t.Cleanup(func() { layout.SetDataRootOverride("") })

	opt := types.ConfigOptions{
		Port:                  443,
		UUID:                  "11111111-1111-1111-1111-111111111111",
		Sni:                   "example.com",
		PrivateKey:            "private",
		PublicKey:             "public",
		ShortIds:              []string{"abcd"},
		RealityDest:           "example.com:443",
		RealityDestHost:       "example.com",
		RealityDestExtraHosts: []string{"cdn.example.com"},
	}
	srvPath, err := GenerateServerConfig(opt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(srvPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	meta, ok := root[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		t.Fatal("missing _tunnelbypass metadata")
	}
	if got, _ := meta["realityDest"].(string); got != "example.com:443" {
		t.Fatalf("metadata realityDest got %q", got)
	}
	if got, _ := meta["realityDestHost"].(string); got != "example.com" {
		t.Fatalf("metadata realityDestHost got %q", got)
	}

	prefs, err := host_catalog.LoadRealityDestPrefsFile(filepath.Dir(srvPath))
	if err != nil {
		t.Fatal(err)
	}
	if prefs.PreferredHost != "example.com" {
		t.Fatalf("prefs preferred host got %q", prefs.PreferredHost)
	}
	if len(prefs.ExtraHosts) != 1 || prefs.ExtraHosts[0] != "cdn.example.com" {
		t.Fatalf("prefs extra hosts got %#v", prefs.ExtraHosts)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(srvPath), "hosts.json")); err != nil {
		t.Fatalf("per-config hosts.json was not written: %v", err)
	}
}
