// Package binmgr holds optional SHA-256 checksums for downloaded third-party binaries.
package binmgr

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed checksums.json
var embeddedChecksums []byte

type entry struct {
	Tool    string `json:"tool"`
	Goos    string `json:"goos"`
	Goarch  string `json:"goarch"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type manifest struct {
	Entries []entry `json:"entries"`
}

var checksumTable map[string]string

func init() {
	checksumTable = make(map[string]string)
	var m manifest
	if err := json.Unmarshal(embeddedChecksums, &m); err != nil {
		return
	}
	for _, e := range m.Entries {
		if strings.TrimSpace(e.SHA256) == "" {
			continue
		}
		k := key(e.Tool, e.Goos, e.Goarch, e.Version)
		checksumTable[k] = strings.TrimSpace(e.SHA256)
	}
}

func key(tool, goos, goarch, version string) string {
	return strings.ToLower(strings.TrimSpace(tool) + "|" + strings.TrimSpace(goos) + "|" + strings.TrimSpace(goarch) + "|" + strings.TrimSpace(version))
}

// Expected SHA-256 hex for tool/os/arch/version, or empty if not in manifest.
func ExpectedSHA256(tool, goos, goarch, version string) string {
	return checksumTable[key(tool, goos, goarch, version)]
}

// VerifyFile checks path against wantHex (64-char hex). Empty wantHex is a no-op.
func VerifyFile(path, wantHex string) error {
	wantHex = strings.TrimSpace(strings.ToLower(wantHex))
	if wantHex == "" {
		return nil
	}
	want, err := hex.DecodeString(wantHex)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := h.Sum(nil)
	if len(got) != len(want) {
		return fmt.Errorf("checksum mismatch")
	}
	for i := range got {
		if got[i] != want[i] {
			return fmt.Errorf("checksum mismatch")
		}
	}
	return nil
}
