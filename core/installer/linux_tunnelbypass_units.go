package installer

import (
	"path/filepath"
	"runtime"
)

const linuxSystemdSystemPath = "/etc/systemd/system"

// TunnelBypassSystemdUnitFilesCount returns the number of files matching TunnelBypass*.service
// under systemdUnitDir. Used to detect remaining OS services after removing one inbound.
func TunnelBypassSystemdUnitFilesCount(systemdUnitDir string) int {
	if systemdUnitDir == "" {
		return 0
	}
	matches, err := filepath.Glob(filepath.Join(systemdUnitDir, "TunnelBypass*.service"))
	if err != nil {
		return 0
	}
	return len(matches)
}

// RemainingTunnelBypassSystemdUnitCount returns how many TunnelBypass*.service unit files exist
// under /etc/systemd/system on Linux. On other OSes returns 0.
func RemainingTunnelBypassSystemdUnitCount() int {
	if runtime.GOOS != "linux" {
		return 0
	}
	return TunnelBypassSystemdUnitFilesCount(linuxSystemdSystemPath)
}
