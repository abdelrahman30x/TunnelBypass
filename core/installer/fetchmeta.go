package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// FetchMeta records last binary download outcome for debugging.
type FetchMeta struct {
	Tool        string `json:"tool"`
	LastOK      bool   `json:"last_ok"`
	LastUnix    int64  `json:"last_unix"`
	LastURL     string `json:"last_url,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	LastVersion string `json:"last_version,omitempty"`
}

func fetchMetaPath(binDir, tool string) string {
	return filepath.Join(binDir, tool+".fetch.json")
}

func writeFetchMetaOK(binDir, tool, url, version string) {
	m := FetchMeta{
		Tool: tool, LastOK: true, LastUnix: time.Now().Unix(),
		LastURL: url, LastVersion: version,
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(fetchMetaPath(binDir, tool), b, 0644)
}

func writeFetchMetaFail(binDir, tool, url, version, errStr string) {
	m := FetchMeta{
		Tool: tool, LastOK: false, LastUnix: time.Now().Unix(),
		LastURL: url, LastVersion: version, LastError: errStr,
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(fetchMetaPath(binDir, tool), b, 0644)
}
