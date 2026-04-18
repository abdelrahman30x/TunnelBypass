package network

import "testing"

func TestParseHostMode(t *testing.T) {
	tests := []struct {
		in   string
		want HostMode
		ok   bool
	}{
		{"lan", LANMode, true},
		{"LAN", LANMode, true},
		{" self-host ", LANMode, true},
		{"local", LANMode, true},
		{"", InternetMode, true},
		{"internet", InternetMode, true},
		{"PUBLIC", InternetMode, true},
		{"invalid", InternetMode, false},
	}
	for _, tc := range tests {
		m, ok := ParseHostMode(tc.in)
		if ok != tc.ok || m != tc.want {
			t.Fatalf("ParseHostMode(%q) = (%v,%v) want (%v,%v)", tc.in, m, ok, tc.want, tc.ok)
		}
	}
}

func TestBuildXrayPrivateRule(t *testing.T) {
	b := BuildXrayPrivateRule(BlockPrivate)
	if len(b) != 1 {
		t.Fatalf("BlockPrivate: got len %d", len(b))
	}
	m, ok := b[0].(map[string]interface{})
	if !ok || m["outboundTag"] != "block" {
		t.Fatalf("unexpected rule %#v", b[0])
	}
	r := BuildXrayPrivateRule(AllowAllPrivate)
	if len(r) != 0 {
		t.Fatalf("AllowAllPrivate: want empty slice, got %#v", r)
	}
}

func TestNetworkProfilePrivateRouting(t *testing.T) {
	p := NetworkProfile{Mode: LANMode, Security: RelaxedLAN}
	if p.PrivateRouting() != AllowAllPrivate {
		t.Fatal("LAN+Relaxed should AllowAllPrivate")
	}
	p = NetworkProfile{Mode: LANMode, Security: StrictLAN}
	if p.PrivateRouting() != BlockPrivate {
		t.Fatal("LAN+Strict should BlockPrivate")
	}
	p = NetworkProfile{Mode: InternetMode, Security: RelaxedLAN}
	if p.PrivateRouting() != BlockPrivate {
		t.Fatal("Internet should still BlockPrivate")
	}
}

func TestIfaceNameScore(t *testing.T) {
	if ifaceNameScore("docker0") != -1 {
		t.Fatal("docker0 should skip")
	}
	if ifaceNameScore("eth0") != 3 {
		t.Fatalf("eth0 score got %d", ifaceNameScore("eth0"))
	}
	if ifaceNameScore("wlan0") != 2 {
		t.Fatalf("wlan0 score got %d", ifaceNameScore("wlan0"))
	}
}
