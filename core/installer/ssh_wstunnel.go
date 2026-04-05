package installer

import (
	"fmt"
	"path/filepath"
	"runtime"
)

func EnsureSshWstunnelServer(wssPort int, username, password string, updatePassword bool, isAdmin bool) error {
	if wssPort <= 0 {
		wssPort = 443
	}

	if ContainerSkipNativeServices() {
		cfgDir := GetConfigDir("wstunnel")
		certPath := filepath.Join(cfgDir, "wss-cert.pem")
		keyPath := filepath.Join(cfgDir, "wss-key.pem")
		if err := EnsureSelfSignedCert(certPath, keyPath, "localhost"); err != nil {
			return err
		}
		fmt.Printf("    [!] Container: skipped embedded SSH + WSS user services (not foreground-safe).\n")
		fmt.Printf("        Start the tunnel with: tunnelbypass run portable wss --port %d (see README).\n", wssPort)
		return nil
	}

	wssPort = EnsureFreeTCPPort(wssPort, "WSS")

	if username != "" {
		if err := EnsureWindowsUser(username, password, updatePassword, isAdmin); err != nil {
			fmt.Printf("    [!] Warning: Windows user creation failed: %v\n", err)
		}
	}

	fmt.Printf("\n    [*] Installing embedded SSH as an OS service...\n")

	// Stop the embedded SSH server first to release the port for systemd service
	StopEmbeddedSSHServer()

	udpgwPort, err := EnsureSSHUDPGW(7300)
	if err != nil {
		return fmt.Errorf("UDPGW service: %w", err)
	}

	if err := installSSHServiceAsStandalone(username, password, false, true, udpgwPort); err != nil {
		return fmt.Errorf("failed to install SSH service: %w", err)
	}

	wstunnelPath, err := EnsureBinary("wstunnel")
	if err != nil {
		return err
	}

	cfgDir := GetConfigDir("wstunnel")
	certPath := filepath.Join(cfgDir, "wss-cert.pem")
	keyPath := filepath.Join(cfgDir, "wss-key.pem")
	if err := EnsureSelfSignedCert(certPath, keyPath, "localhost"); err != nil {
		return err
	}

	serviceName := "TunnelBypass-WSS"
	sshBack := fmt.Sprintf("127.0.0.1:%d", GetSSHBackendPort())
	args := []string{
		"server",
		"--restrict-to", sshBack,
		fmt.Sprintf("wss://0.0.0.0:%d", wssPort),
		"--tls-certificate", certPath,
		"--tls-private-key", keyPath,
	}

	if err := CreateService(
		serviceName,
		serviceName+" (WSTunnel)",
		wstunnelPath,
		args,
		GetBaseDir(),
	); err != nil {
		return err
	}

	_ = OpenFirewallPort(wssPort, "tcp", serviceName)

	fmt.Printf("    [*] WSS server is active on port %d -> SSH backend %s\n", wssPort, sshBack)
	if runtime.GOOS == "linux" {
		fmt.Printf("    [!] Recommendation: To hide your SSH server completely, you can now safely:\n")
		fmt.Printf("        1. Modify /etc/ssh/sshd_config to set 'ListenAddress 127.0.0.1'.\n")
		fmt.Printf("        2. Use a firewall to block external access to port 22.\n")
	}

	return nil
}
