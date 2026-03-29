package portable

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tunnelbypass/internal/runtimeenv"
	"tunnelbypass/internal/utils"
)

// RunMeta is persisted under run/ for status and tooling.
type RunMeta struct {
	Transport string         `json:"transport"`
	Ports     map[string]int `json:"ports,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
	Updated   int64          `json:"updated_unix"`
}

func metaPath(baseDir, transport string) string {
	return filepath.Join(runtimeenv.RunDir(baseDir), "portable-"+sanitize(transport)+".meta")
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r <= ' ' || r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, s)
}

// WriteRunMeta writes run metadata for a portable transport.
func WriteRunMeta(baseDir, transport string, m RunMeta) error {
	m.Transport = transport
	m.Updated = time.Now().Unix()
	dir := runtimeenv.RunDir(baseDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(metaPath(baseDir, transport), b, 0644); err != nil {
		return err
	}
	if m.Ports != nil {
		UpdateTransportPorts(transport, m.Ports)
	}
	return nil
}

// ReadRunMeta loads metadata if present.
func ReadRunMeta(baseDir, transport string) (RunMeta, error) {
	var m RunMeta
	b, err := os.ReadFile(metaPath(baseDir, transport))
	if err != nil {
		return m, err
	}
	b = utils.StripUTF8BOM(b)
	err = json.Unmarshal(b, &m)
	return m, err
}
