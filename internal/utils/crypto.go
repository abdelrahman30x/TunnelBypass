package utils

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateX25519Keys generates a private and public key pair for Xray Reality
func GenerateX25519Keys() (string, string, error) {
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %v", err)
	}

	publicKey := privateKey.PublicKey()

	privBase64 := base64.RawURLEncoding.EncodeToString(privateKey.Bytes())
	pubBase64 := base64.RawURLEncoding.EncodeToString(publicKey.Bytes())

	return privBase64, pubBase64, nil
}

// GenerateRandomShortIds generates a set of random hex strings for Xray Reality
func GenerateRandomShortIds() []string {
	b := make([]byte, 8)
	rand.Read(b)
	full := fmt.Sprintf("%x", b)
	return []string{
		full,
		full[:8],
		full[:4],
		"",
	}
}

// GenerateUUID generates a random UUID v4 string
func GenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
