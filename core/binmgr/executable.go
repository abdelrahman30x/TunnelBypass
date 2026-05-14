package binmgr

import (
	"path/filepath"
	"runtime"
	"strings"
)

// On-disk binary name (e.g. xray / xray.exe).
func ExecutableFilename(tool string) string {
	t := strings.ToLower(strings.TrimSpace(tool))
	base := t
	switch t {
	case "shadowsocks":
		base = "ssserver"
	}
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// ExpectedExecutablePath joins a binary directory with the conventional executable name.
func ExpectedExecutablePath(tool, binDir string) string {
	return filepath.Join(binDir, ExecutableFilename(tool))
}
