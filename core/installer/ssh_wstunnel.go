package installer

import (
	"fmt"
	"path/filepath"
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

	if err := EnsureSSHServerWithAuth(username, password); err != nil {
		return err
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
	if err := EnsureSSHUDPGW(7300); err != nil {
		fmt.Printf("    [!] Warning: UDPGW setup failed: %v\n", err)
	}
	return nil
}
