package portablecfg

import (
	"os"
	"path/filepath"
	"testing"

	"tunnelbypass/core/portable"
)

func TestMergePrecedenceEnvOverFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "portable.json")
	if err := os.WriteFile(cfgPath, []byte(`{"ssh_port":2222,"udpgw_port":7301}`), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TB_SSH_PORT", "3333")
	o := Merge(portable.Options{}, f)
	if o.SSHPort != 3333 {
		t.Fatalf("env should override file for ssh_port: got %d", o.SSHPort)
	}
	if o.UDPGWPort != 7301 {
		t.Fatalf("udpgw from file: got %d", o.UDPGWPort)
	}
}
