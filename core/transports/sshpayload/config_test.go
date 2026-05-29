package sshpayload

import (
	"strings"
	"testing"
)

func TestConfigDefaultsAndInstructions(t *testing.T) {
	cfg := Config{
		ServerAddr:     "203.0.113.10",
		SSHBackendPort: 31043,
		SSHUser:        "u",
		SSHPassword:    "p",
	}.WithDefaults()
	if cfg.ListenPort != DefaultListenPort {
		t.Fatalf("listen port=%d, want %d", cfg.ListenPort, DefaultListenPort)
	}
	if !strings.HasPrefix(cfg.PayloadPath, "/ssh-ws-") {
		t.Fatalf("payload path=%q, want /ssh-ws-*", cfg.PayloadPath)
	}

	ins := Instructions(cfg)
	for _, want := range []string{
		"203.0.113.10:80",
		"User:         u",
		"Password:     p",
		cfg.PayloadPath,
		"PATCH " + cfg.PayloadPath + " HTTP/1.1",
		"CONNECT " + cfg.PayloadPath + " HTTP/1.1",
		"WebSocket-style payload",
		"Remote Proxy: blank",
	} {
		if !strings.Contains(ins, want) {
			t.Fatalf("instructions missing %q:\n%s", want, ins)
		}
	}
}
