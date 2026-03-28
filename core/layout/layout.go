// Package layout: data directory paths and overrides (SetDataRootOverride / CLI).
package layout

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var dataRootOverride string
var logsRootOverride string

func normalizePathInput(dir string) string {
	d := strings.TrimSpace(dir)
	if d == "" {
		return ""
	}
	// Accept Unix-style absolute input on Windows (e.g. "/data") as "<SystemDrive>\\data".
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(d, "/") || strings.HasPrefix(d, `\`) {
			drive := os.Getenv("SystemDrive")
			if drive == "" {
				drive = "C:"
			}
			d = filepath.Join(drive+`\`, strings.TrimLeft(filepath.FromSlash(d), `\`))
		} else {
			d = filepath.FromSlash(d)
		}
	}
	return filepath.Clean(d)
}

// Data root override for configs and binaries; empty clears.
func SetDataRootOverride(dir string) {
	dataRootOverride = normalizePathInput(dir)
}

// Log directory override; empty uses <base>/logs.
func SetLogsRootOverride(dir string) {
	logsRootOverride = normalizePathInput(dir)
}

// Log directory: override or <base>/logs.
func GetLogsDir() string {
	if logsRootOverride != "" {
		return logsRootOverride
	}
	return filepath.Join(GetBaseDir(), "logs")
}

// Current data root override, or empty.
func DataRootOverride() string {
	return dataRootOverride
}

// Default user data directory when portable mode is active.
func PortableDefaultDataDir() string {
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			home, _ := os.UserHomeDir()
			local = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(local, "TunnelBypass")
	}
	xdg := os.Getenv("XDG_DATA_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "tunnelbypass")
	}
	return filepath.Join(xdg, "tunnelbypass")
}

// Install root: SetDataRootOverride, or OS default (use --data-dir / --portable from CLI).
func GetBaseDir() string {
	if dataRootOverride != "" {
		return dataRootOverride
	}
	if runtime.GOOS == "windows" {
		drive := os.Getenv("SystemDrive")
		if drive == "" {
			drive = "C:"
		}
		if !strings.HasSuffix(drive, `\`) {
			drive += `\`
		}
		return filepath.Join(drive, "TunnelBypass")
	}
	return "/usr/local/etc/tunnelbypass"
}

// <base>/configs/<transport> (e.g. .../configs/hysteria).
func GetConfigDir(transport string) string {
	dir := filepath.Join(GetBaseDir(), "configs")
	t := strings.TrimSpace(strings.ToLower(transport))
	if t == "" {
		return dir
	}
	return filepath.Join(dir, t)
}

// True when a data-root override selects a portable-style tree (explicit path set via API).
func PortableLayoutActive() bool {
	return dataRootOverride != ""
}
