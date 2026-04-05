package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTunnelBypassSystemdUnitFilesCount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if n := TunnelBypassSystemdUnitFilesCount(dir); n != 0 {
		t.Fatalf("empty dir: want 0 got %d", n)
	}
	_ = os.WriteFile(filepath.Join(dir, "TunnelBypass-VLESS.service"), []byte("[Service]\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "TunnelBypass-Hysteria.service"), []byte("[Service]\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "other.service"), []byte("[Service]\n"), 0644)
	if n := TunnelBypassSystemdUnitFilesCount(dir); n != 2 {
		t.Fatalf("want 2 TunnelBypass units, got %d", n)
	}
}

func TestTunnelBypassSystemdUnitFilesCount_emptyPath(t *testing.T) {
	t.Parallel()
	if n := TunnelBypassSystemdUnitFilesCount(""); n != 0 {
		t.Fatalf("want 0 got %d", n)
	}
}
