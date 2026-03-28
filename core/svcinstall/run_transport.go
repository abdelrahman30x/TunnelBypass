package svcinstall

import (
	"fmt"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/hysteria"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/transports/wireguard"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/runtimeenv"
)

// InstallRunTransportService registers an OS service (or user supervisor) for wizard-capable transports.
func InstallRunTransportService(transport string, opt types.ConfigOptions, isAdmin bool) error {
	inf := runtimeenv.Detect()
	if inf.LikelyContainer {
		return fmt.Errorf("container: service install disabled (use `run portable <transport>`)")
	}
	t := strings.ToLower(strings.TrimSpace(transport))
	switch t {
	case "reality", "vless":
		cfg := filepath.Join(installer.GetConfigDir("vless"), "server.json")
		return vless.InstallXrayService("TunnelBypass-VLESS", cfg, opt.Port)
	case "hysteria":
		cfg := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		return hysteria.InstallHysteriaService("TunnelBypass-Hysteria", cfg, opt.Port)
	case "wireguard":
		cfg := filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf")
		return wireguard.InstallWireGuardService("TunnelBypass-WireGuard", cfg, opt.Port)
	case "wss":
		u := strings.TrimSpace(opt.SSHUser)
		pw := strings.TrimSpace(opt.SSHPassword)
		if u == "" {
			u = "tunnelbypass"
		}
		return installer.EnsureSshWstunnelServer(opt.Port, u, pw, true, isAdmin)
	case "tls":
		u := strings.TrimSpace(opt.SSHUser)
		pw := strings.TrimSpace(opt.SSHPassword)
		if u == "" {
			u = "tunnelbypass"
		}
		return installer.EnsureSshStunnelServer(opt.Port, u, pw, true, isAdmin)
	case "ssh", "udpgw":
		return fmt.Errorf("transport %q has no OS service install path (use foreground run or portable)", t)
	default:
		return fmt.Errorf("unknown transport %q for service install", t)
	}
}
