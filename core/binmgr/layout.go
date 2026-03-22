package binmgr

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Directory for third-party binaries under baseDir; portable uses <base>/bin/<tool>.
func SystemBinaryDir(name string, baseDir string, portable bool) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if portable {
		return filepath.Join(baseDir, "bin", name)
	}
	if runtime.GOOS == "windows" {
		drive := os.Getenv("SystemDrive")
		if drive == "" {
			drive = "C:"
		}
		if !strings.HasSuffix(drive, `\`) {
			drive += `\`
		}
		switch name {
		case "xray":
			return filepath.Join(drive, "xray")
		case "hysteria":
			return filepath.Join(drive, "hysteria")
		case "wstunnel":
			return filepath.Join(drive, "wstunnel")
		case "stunnel":
			return filepath.Join(drive, "stunnel")
		case "winsw":
			return filepath.Join(drive, "winsw")
		case "wireguard-msi":
			return filepath.Join(drive, "wireguard-cache")
		case "openssl":
			return filepath.Join(drive, "openssl")
		default:
			return filepath.Join(drive, "tunnelbypass-bin")
		}
	}
	switch name {
	case "openssl":
		return "/usr/local/openssl"
	default:
		return filepath.Join("/opt/tunnelbypass/bin", name)
	}
}
