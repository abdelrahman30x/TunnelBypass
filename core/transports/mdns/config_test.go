package mdns

import (
	"os"
	"strings"
	"testing"

	"tunnelbypass/core/layout"
	"tunnelbypass/core/types"
)

func useTempDataRoot(t *testing.T) {
	t.Helper()
	layout.SetDataRootOverride(t.TempDir())
	t.Cleanup(func() { layout.SetDataRootOverride("") })
}

func TestGenerateServerConfig(t *testing.T) {
	useTempDataRoot(t)

	opt := types.ConfigOptions{
		Transport:            "mdns",
		ServerAddr:           "192.0.2.1",
		Port:                 53,
		MDNSDomain:           "v.example.com",
		MDNSEncryptionMethod: 3,
	}

	path, err := GenerateServerConfig(opt, "testkey123456789012345678901234")
	if err != nil {
		t.Fatalf("GenerateServerConfig failed: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `DOMAIN = ["v.example.com"]`) {
		t.Errorf("missing domain in server config")
	}
	if !strings.Contains(content, "UDP_PORT = 53") {
		t.Errorf("missing UDP_PORT = 53")
	}
	if !strings.Contains(content, "DATA_ENCRYPTION_METHOD = 3") {
		t.Errorf("missing encryption method")
	}

	// Check encrypt_key.txt was written
	keyPath := strings.Replace(path, "server_config.toml", "encrypt_key.txt", 1)
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read encrypt_key.txt: %v", err)
	}
	defer os.Remove(keyPath)
	if string(keyData) != "testkey123456789012345678901234" {
		t.Errorf("encrypt key mismatch: got %q", string(keyData))
	}
}

func TestGenerateClientConfig(t *testing.T) {
	useTempDataRoot(t)

	opt := types.ConfigOptions{
		Transport:            "mdns",
		ServerAddr:           "192.0.2.1",
		MDNSDomain:           "v.example.com",
		MDNSEncryptionMethod: 3,
	}

	clientPath, resolversPath, err := GenerateClientConfig(opt, "testkey123456789012345678901234", nil)
	if err != nil {
		t.Fatalf("GenerateClientConfig failed: %v", err)
	}
	defer os.Remove(clientPath)
	defer os.Remove(resolversPath)

	clientData, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatalf("read client config: %v", err)
	}
	content := string(clientData)

	if !strings.Contains(content, `DOMAINS = ["v.example.com"]`) {
		t.Errorf("missing domain in client config")
	}
	if !strings.Contains(content, `ENCRYPTION_KEY = "testkey123456789012345678901234"`) {
		t.Errorf("missing encryption key")
	}
	if !strings.Contains(content, "RESOLVER_BALANCING_STRATEGY = 5") {
		t.Errorf("missing stealth resolver balancing")
	}
	if !strings.Contains(content, "BASE_ENCODE_DATA = true") {
		t.Errorf("missing base encode setting")
	}

	resolverData, err := os.ReadFile(resolversPath)
	if err != nil {
		t.Fatalf("read resolvers: %v", err)
	}
	if !strings.Contains(string(resolverData), "8.8.8.8") {
		t.Errorf("missing default resolver")
	}
}

func TestGenerateEncryptionKey(t *testing.T) {
	key1 := GenerateEncryptionKey()
	key2 := GenerateEncryptionKey()
	if len(key1) != 32 {
		t.Errorf("key length = %d, want 32", len(key1))
	}
	if key1 == key2 {
		t.Error("generated identical keys")
	}
}

func TestGenerateResolverList(t *testing.T) {
	list := GenerateResolverList([]string{"1.1.1.1", "8.8.8.8"})
	if !strings.Contains(list, "1.1.1.1") {
		t.Error("missing 1.1.1.1")
	}
	if !strings.Contains(list, "8.8.8.8") {
		t.Error("missing 8.8.8.8")
	}
	if !strings.HasSuffix(list, "\n") {
		t.Error("expected trailing newline")
	}
}
