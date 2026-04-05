package svcinstall

import (
	"fmt"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/types"
)

// installSshTlsStack installs TunnelBypass-UDPGW, embedded SSH with --external-udpgw (same as TLS/WSS),
// then TunnelBypass-SSH-TLS (Xray). Required so UDPGW works with SSH-over-TLS deployments.
func installSshTlsStack(opt types.ConfigOptions, isAdmin bool) error {
	port := opt.Port
	if port <= 0 {
		port = 2053
	}

	if installer.ContainerSkipNativeServices() {
		fmt.Printf("    [!] Container: skipped UDPGW + embedded SSH + SSH-TLS OS services.\n")
		fmt.Printf("        Use: tunnelbypass run portable ssh-tls --port %d (see docs).\n", port)
		return nil
	}

	// Port was already chosen by provision / ApplyPortAllocation; do not re-allocate here (must match server.json).

	u := strings.TrimSpace(opt.SSHUser)
	if u == "" {
		u = "tunnelbypass"
	}
	pw := strings.TrimSpace(opt.SSHPassword)

	if u != "" {
		if err := installer.EnsureWindowsUser(u, pw, true, isAdmin); err != nil {
			fmt.Printf("    [!] Warning: Windows user creation failed: %v\n", err)
		}
	}

	fmt.Printf("\n    [*] Installing UDPGW + embedded SSH for SSH-TLS (Netmod / SSH-over-TLS)...\n")
	installer.StopEmbeddedSSHServer()

	udpgwPort, err := installer.EnsureSSHUDPGW(7300)
	if err != nil {
		return fmt.Errorf("UDPGW service: %w", err)
	}

	if err := installer.InstallEmbedSSHServiceWithPrepare(u, pw, true, udpgwPort); err != nil {
		return fmt.Errorf("embedded SSH service: %w", err)
	}

	cfg := filepath.Join(installer.GetConfigDir("ssh-tls"), "server.json")
	if err := vless.InstallXrayService("TunnelBypass-SSH-TLS", cfg, port, opt); err != nil {
		return fmt.Errorf("xray ssh-tls: %w", err)
	}
	return nil
}
