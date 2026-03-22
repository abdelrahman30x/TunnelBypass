package binmgr

import (
	"path/filepath"
	"runtime"
	"strings"
)

// On-disk binary name (e.g. xray / xray.exe).
func ExecutableFilename(tool string) string {
	t := strings.ToLower(strings.TrimSpace(tool))
	if runtime.GOOS == "windows" {
		return t + ".exe"
	}
	return t
}

// ExpectedExecutablePath joins a binary directory with the conventional executable name.
func ExpectedExecutablePath(tool, binDir string) string {
	return filepath.Join(binDir, ExecutableFilename(tool))
}
