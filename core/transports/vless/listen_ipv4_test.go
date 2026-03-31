package vless

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureInboundListenIPv4(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "server.json")
	raw := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "vless-in",
				"port":     float64(443),
				"listen":   "::",
				"protocol": "vless",
			},
			map[string]interface{}{
				"tag":      "api-in",
				"listen":   "127.0.0.1",
				"port":     float64(10085),
				"protocol": "dokodemo-door",
			},
		},
	}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureInboundListenIPv4(path); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	inbounds := root["inbounds"].([]interface{})
	vless := inbounds[0].(map[string]interface{})
	if vless["listen"] != "0.0.0.0" {
		t.Fatalf("vless listen: got %v", vless["listen"])
	}
	api := inbounds[1].(map[string]interface{})
	if api["listen"] != "127.0.0.1" {
		t.Fatalf("api listen unchanged: got %v", api["listen"])
	}
}
