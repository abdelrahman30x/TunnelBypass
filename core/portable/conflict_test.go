package portable

import (
	"runtime"
	"strings"
	"testing"
)

func TestFormatConflictIncludesPortAndActions(t *testing.T) {
	c := PortConflict{
		Kind:             ConflictDifferentService,
		Transport:        "wss",
		Port:             443,
		ProcessName:      "wstunnel.exe",
		PID:              1234,
		RunningTransport: "wss",
		Suggestions: []string{
			"[+] Try another port:\ntunnelbypass run --type wss --port 8443",
			"[+] Check/reuse existing:\ntunnelbypass status",
		},
	}
	out := FormatConflict(c)
	if !strings.Contains(out, "Port 443 is already in use") {
		t.Fatalf("missing port warning: %s", out)
	}
	if !strings.Contains(out, "Available actions:") {
		t.Fatalf("missing actions header: %s", out)
	}
	if !strings.Contains(out, "tunnelbypass status") {
		t.Fatalf("missing suggestion command: %s", out)
	}
}

func TestBuildConflictNoConflictOnFreePort(t *testing.T) {
	c := BuildConflict("ssh", 0, "", "tcp", "")
	if c.Kind != ConflictNone {
		t.Fatalf("expected none, got %q", c.Kind)
	}
}

func TestBuildSuggestionsIncludesCommandHint(t *testing.T) {
	c := PortConflict{
		Kind:      ConflictDifferentService,
		Transport: "tls",
		Port:      443,
	}
	sg := buildSuggestions(c, "tunnelbypass run --type tls --data-dir C:\\data", "")
	all := strings.Join(sg, "\n")
	if !strings.Contains(all, "--type tls") {
		t.Fatalf("expected transport hint in suggestions: %s", all)
	}
	if !strings.Contains(all, "tunnelbypass status") {
		t.Fatalf("expected status hint: %s", all)
	}
}

func TestBuildSuggestionsOSServiceStopFirst(t *testing.T) {
	c := PortConflict{
		Kind:          ConflictDifferentService,
		Transport:     "reality",
		Port:          443,
		OSServiceName: "TunnelBypass-VLESS",
	}
	sg := buildSuggestions(c, "tunnelbypass run --type reality --data-dir C:\\data", `/data`)
	if len(sg) < 3 {
		t.Fatalf("expected multiple suggestions, got %d", len(sg))
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(sg[0], "sc stop TunnelBypass-VLESS") {
			t.Fatalf("expected sc stop first: %q", sg[0])
		}
	case "linux":
		if !strings.Contains(sg[0], "systemctl stop TunnelBypass-VLESS") {
			t.Fatalf("expected systemctl stop first: %q", sg[0])
		}
	default:
		if !strings.Contains(sg[0], "TunnelBypass-VLESS") {
			t.Fatalf("expected service name in stop hint: %q", sg[0])
		}
	}
	all := strings.Join(sg, "\n")
	if !strings.Contains(all, "tunnelbypass uninstall") || !strings.Contains(all, "TunnelBypass-VLESS") {
		t.Fatalf("expected uninstall hint: %s", all)
	}
	if !strings.Contains(all, `--data-dir "/data"`) && !strings.Contains(all, `--data-dir /data`) {
		t.Fatalf("expected data-dir in uninstall hint: %s", all)
	}
}

func TestBuildSuggestionsKillHintWithPID(t *testing.T) {
	c := PortConflict{
		Kind:          ConflictDifferentService,
		Transport:     "reality",
		Port:          443,
		OSServiceName: "TunnelBypass-VLESS",
		PID:           10932,
	}
	sg := buildSuggestions(c, "tunnelbypass run --type reality", "")
	last := sg[len(sg)-1]
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(last, "taskkill") || !strings.Contains(last, "10932") {
			t.Fatalf("expected windows kill hint: %q", last)
		}
	default:
		if !strings.Contains(last, "kill") || !strings.Contains(last, "10932") {
			t.Fatalf("expected unix kill hint: %q", last)
		}
	}
}

func TestOSServiceNameForTransport(t *testing.T) {
	if OSServiceNameForTransport("reality") != "TunnelBypass-VLESS" {
		t.Fatalf("reality -> VLESS")
	}
	if OSServiceNameForTransport("hysteria") != "TunnelBypass-Hysteria" {
		t.Fatalf("hysteria")
	}
}
