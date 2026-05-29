package cfg

import (
	"testing"

	"tunnelbypass/core/types"
)

func TestSSHPayloadAliasesAndDefaults(t *testing.T) {
	for _, alias := range []string{"ssh-payload", "payload", "ssh-http-payload", "http-payload"} {
		if got := NormalizeTransport(alias); got != "ssh-payload" {
			t.Fatalf("NormalizeTransport(%q)=%q, want ssh-payload", alias, got)
		}
	}
	if got := RunnerTransportFor("payload"); got != "ssh-payload" {
		t.Fatalf("RunnerTransportFor(payload)=%q, want ssh-payload", got)
	}

	spec := RunSpec{Transport: "payload"}
	FillDefaults(&spec)
	if spec.Transport != "ssh-payload" {
		t.Fatalf("transport=%q, want ssh-payload", spec.Transport)
	}
	if spec.Port != types.DefaultSSHPayloadListenPort {
		t.Fatalf("port=%d, want %d", spec.Port, types.DefaultSSHPayloadListenPort)
	}
	if spec.SSH.Port != 0 {
		t.Fatalf("ssh port=%d, want dynamic 0", spec.SSH.Port)
	}
	if !spec.UDPGW.Enabled {
		t.Fatal("ssh-payload should enable UDPGW")
	}
}

func TestRunSpecAcceptsPayloadPath(t *testing.T) {
	base := RunSpec{Transport: "ssh-payload"}
	override := RunSpec{PayloadPath: "/ssh-ws-test"}
	got := Merge(base, override)
	if got.PayloadPath != "/ssh-ws-test" {
		t.Fatalf("payload path=%q", got.PayloadPath)
	}
}
