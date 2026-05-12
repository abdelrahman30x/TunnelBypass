package network

import "testing"

func TestBuildXrayPrivateRule(t *testing.T) {
	b := BuildXrayPrivateRule(BlockPrivate)
	if len(b) != 1 {
		t.Fatalf("BlockPrivate: got len %d", len(b))
	}
	m, ok := b[0].(map[string]interface{})
	if !ok || m["outboundTag"] != "block" {
		t.Fatalf("unexpected rule %#v", b[0])
	}
}
