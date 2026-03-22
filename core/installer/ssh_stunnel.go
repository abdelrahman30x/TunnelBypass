package installer

import (
	"fmt"
	"path/filepath"

	tbssh "tunnelbypass/core/ssh"
)

func EnsureSshStunnelServer(sslPort int, username, password string, updatePassword bool, isAdmin bool) error {
	if sslPort <= 0 {
		sslPort = 443
	}

	if ContainerSkipNativeServices() {
		cfgDir := GetConfigDir("stunnel")
		certPath := filepath.Join(cfgDir, "ssl-cert.pem")
		keyPath := filepath.Join(cfgDir, "ssl-key.pem")
		if err := EnsureSelfSignedCert(certPath, keyPath, "localhost"); err != nil {
			return err
		}
		stunnelConf := filepath.Join(cfgDir, "stunnel-server.conf")
		sshBack := tbssh.ListenPreference()
		if err := WriteStunnelServerConfig(stunnelConf, sslPort, sshBack, certPath, keyPath); err != nil {
			return err
		}
		fmt.Printf("    [!] Container: skipped embedded SSH + stunnel user services (not foreground-safe).\n")
		fmt.Printf("        Start with: tunnelbypass run portable tls (listen port %d).\n", sslPort)
		return nil
	}

	sslPort = EnsureFreeTCPPort(sslPort, "SSL")

	if username != "" {
		if err := EnsureWindowsUser(username, password, updatePassword, isAdmin); err != nil {
			fmt.Printf("    [!] Warning: Windows user creation failed: %v\n", err)
		}
		EnsureManagedSSHConfig(username)
	} else {
		EnsureSaneSSHConfig()
	}

	if err := EnsureSSHServerWithAuth(username, password); err != nil {
		return err
	}

	stunnelPath, err := EnsureStunnel()
	if err != nil {
		return err
	}

	cfgDir := GetConfigDir("stunnel")
	certPath := filepath.Join(cfgDir, "ssl-cert.pem")
	keyPath := filepath.Join(cfgDir, "ssl-key.pem")
	if err := EnsureSelfSignedCert(certPath, keyPath, "localhost"); err != nil {
		return err
	}

	stunnelConf := filepath.Join(cfgDir, "stunnel-server.conf")
	if err := WriteStunnelServerConfig(stunnelConf, sslPort, GetSSHBackendPort(), certPath, keyPath); err != nil {
		return err
	}

	serviceName := "TunnelBypass-SSL"
	args := []string{stunnelConf}
	if err := CreateService(
		serviceName,
		serviceName+" (stunnel SSH-SSL)",
		stunnelPath,
		args,
		GetBaseDir(),
	); err != nil {
		return err
	}

	_ = OpenFirewallPort(sslPort, "tcp", serviceName)
	if err := EnsureSSHUDPGW(7300); err != nil {
		fmt.Printf("    [!] Warning: UDPGW setup failed: %v\n", err)
	}
	return nil
}
