package utils

import (
	"regexp"
	"strings"
	"testing"
)

// TestGenerateUUID verifies basic UUID v4 format compliance.
func TestGenerateUUID(t *testing.T) {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	for i := 0; i < 100; i++ {
		got := GenerateUUID()
		if !uuidRegex.MatchString(got) {
			t.Errorf("GenerateUUID() = %q; want UUID v4 format", got)
		}
	}
}

// TestGenerateUUIDUniqueness ensures generated UUIDs are unique.
func TestGenerateUUIDUniqueness(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := GenerateUUID()
		if seen[id] {
			t.Errorf("GenerateUUID() produced duplicate value: %q", id)
		}
		seen[id] = true
	}
}

// TestGenerateX25519Keys verifies that key generation produces valid base64url strings.
func TestGenerateX25519Keys(t *testing.T) {
	priv, pub, err := GenerateX25519Keys()
	if err != nil {
		t.Fatalf("GenerateX25519Keys() error: %v", err)
	}
	if priv == "" {
		t.Error("GenerateX25519Keys() returned empty private key")
	}
	if pub == "" {
		t.Error("GenerateX25519Keys() returned empty public key")
	}
	// X25519 keys are 32 bytes → 43 chars base64url (no padding)
	if len(priv) != 43 {
		t.Errorf("private key length = %d; want 43 (base64url of 32 bytes)", len(priv))
	}
	if len(pub) != 43 {
		t.Errorf("public key length = %d; want 43 (base64url of 32 bytes)", len(pub))
	}
}

// TestGenerateX25519KeysAreDistinct verifies that successive calls produce different key pairs.
func TestGenerateX25519KeysAreDistinct(t *testing.T) {
	priv1, pub1, _ := GenerateX25519Keys()
	priv2, pub2, _ := GenerateX25519Keys()
	if priv1 == priv2 {
		t.Error("GenerateX25519Keys() returned the same private key twice")
	}
	if pub1 == pub2 {
		t.Error("GenerateX25519Keys() returned the same public key twice")
	}
}

func TestX25519PublicKeyFromPrivateMatchesGenerated(t *testing.T) {
	priv, pub, err := GenerateX25519Keys()
	if err != nil {
		t.Fatal(err)
	}
	derived, err := X25519PublicKeyFromPrivate(priv)
	if err != nil {
		t.Fatalf("X25519PublicKeyFromPrivate: %v", err)
	}
	if derived != pub {
		t.Errorf("derived public %q != expected %q", derived, pub)
	}
}

// TestGenerateRandomShortIds verifies the structure of generated short IDs.
func TestGenerateRandomShortIds(t *testing.T) {
	ids := GenerateRandomShortIds()
	if len(ids) != 4 {
		t.Fatalf("GenerateRandomShortIds() returned %d ids; want 4", len(ids))
	}
	hexRegex := regexp.MustCompile(`^[0-9a-f]*$`)
	for _, id := range ids {
		if !hexRegex.MatchString(id) {
			t.Errorf("shortId %q contains non-hex characters", id)
		}
	}
	// Last element should be empty (catch-all short ID in Xray Reality)
	if ids[3] != "" {
		t.Errorf("GenerateRandomShortIds()[3] = %q; want empty string", ids[3])
	}
	// First element should be 16 hex chars (8 bytes)
	if len(ids[0]) != 16 {
		t.Errorf("GenerateRandomShortIds()[0] length = %d; want 16", len(ids[0]))
	}
}

// TestSanitizeForTag verifies the tag sanitization function.
func TestSanitizeForTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"hello", "hello"},
		{"Hello-World", "Hello-World"},
		{"hello world", "hello-world"},
		{"www.example.com", "www.example.com"},
		{"foo_bar.baz-1", "foo_bar.baz-1"},
		{"foo@bar!baz", "foo-bar-baz"},
		{"  leading  ", "leading"},
	}
	for _, tt := range tests {
		got := SanitizeForTag(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeForTag(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

// TestSanitizeForTagNoPanic verifies that edge-case Unicode input does not panic.
func TestSanitizeForTagNoPanic(t *testing.T) {
	inputs := []string{"日本語", "🔥", "\x00\x01", strings.Repeat("a", 10000)}
	for _, inp := range inputs {
		_ = SanitizeForTag(inp) // should not panic
	}
}
