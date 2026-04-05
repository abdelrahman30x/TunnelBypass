package vless

import (
	"encoding/json"
	"testing"
)

func TestMergeXrayDNSIntoConfig(t *testing.T) {
	t.Parallel()
	root := map[string]interface{}{
		"routing": map[string]interface{}{
			"domainStrategy": "IPIfNonMatch",
			"rules": []interface{}{
				map[string]interface{}{"type": "field", "outboundTag": "block"},
			},
		},
	}
	MergeXrayDNSIntoConfig(root)
	dns, ok := root["dns"].(map[string]interface{})
	if !ok || dns["queryStrategy"] != "UseIPv4" {
		t.Fatalf("dns block: %#v", root["dns"])
	}
	srv, _ := dns["servers"].([]interface{})
	if len(srv) != 3 {
		t.Fatalf("dns servers: want 3 got %d %#v", len(srv), srv)
	}
	r := root["routing"].(map[string]interface{})
	if r["domainStrategy"] != "UseIPv4" {
		t.Fatalf("routing.domainStrategy: want UseIPv4 got %#v", r["domainStrategy"])
	}
	rules := r["rules"].([]interface{})
	if len(rules) < 3 {
		t.Fatalf("expected dns+udp53 rules prepended, got %d rules", len(rules))
	}
	first := rules[0].(map[string]interface{})
	if first["outboundTag"] != "direct" {
		t.Fatalf("first rule outboundTag: %#v", first)
	}
	switch prot := first["protocol"].(type) {
	case []string:
		if len(prot) != 1 || prot[0] != "dns" {
			t.Fatalf("first rule protocol: %#v", first["protocol"])
		}
	case []interface{}:
		if len(prot) != 1 || prot[0] != "dns" {
			t.Fatalf("first rule protocol: %#v", first["protocol"])
		}
	default:
		t.Fatalf("first rule protocol type: %#v", first["protocol"])
	}
	second := rules[1].(map[string]interface{})
	if second["outboundTag"] != "direct" || second["network"] != "udp" || second["port"] != 53 {
		t.Fatalf("second rule (udp/53): %#v", second)
	}
	_, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMergeXrayDNSIntoConfig_insertsDirectFreedomWhenMissing(t *testing.T) {
	t.Parallel()
	root := map[string]interface{}{
		"outbounds": []interface{}{
			map[string]interface{}{"protocol": "blackhole", "tag": "block"},
		},
		"routing": map[string]interface{}{
			"rules": []interface{}{},
		},
	}
	MergeXrayDNSIntoConfig(root)
	out := root["outbounds"].([]interface{})
	if len(out) < 2 {
		t.Fatalf("expected direct prepended, got len %d", len(out))
	}
	d0 := out[0].(map[string]interface{})
	if d0["tag"] != "direct" || d0["protocol"] != "freedom" {
		t.Fatalf("first outbound should be freedom direct: %#v", d0)
	}
}

func TestMergeXrayDNSIntoConfig_keepsExistingDirectFreedom(t *testing.T) {
	t.Parallel()
	existing := map[string]interface{}{
		"protocol": "freedom",
		"tag":      "direct",
		"settings": map[string]interface{}{"domainStrategy": "UseIPv4"},
	}
	root := map[string]interface{}{
		"outbounds": []interface{}{existing},
		"routing":   map[string]interface{}{"rules": []interface{}{}},
	}
	MergeXrayDNSIntoConfig(root)
	if r, ok := root["routing"].(map[string]interface{}); ok && r["domainStrategy"] != "UseIPv4" {
		t.Fatalf("routing.domainStrategy: want UseIPv4 got %#v", r["domainStrategy"])
	}
	out := root["outbounds"].([]interface{})
	if len(out) != 1 {
		t.Fatalf("should not duplicate direct outbound, got %d", len(out))
	}
}
