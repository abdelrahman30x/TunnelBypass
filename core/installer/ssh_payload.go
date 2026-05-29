package installer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/core/transports/sshpayload"
	"tunnelbypass/core/types"
)

const SSHPayloadServiceName = "TunnelBypass-SSH-Payload"

func EnsureSshPayloadServer(payloadPort int, payloadPath, username, password string, updatePassword bool, isAdmin bool) error {
	cfgPath := filepath.Join(GetConfigDir("ssh-payload"), "server.json")
	if cfg, err := sshpayload.LoadConfig(cfgPath); err == nil {
		if payloadPath == "" {
			payloadPath = cfg.PayloadPath
		}
		if payloadPort <= 0 {
			payloadPort = cfg.ListenPort
		}
		if strings.TrimSpace(username) == "" {
			username = cfg.SSHUser
		}
		if strings.TrimSpace(password) == "" {
			password = cfg.SSHPassword
		}
	}
	if payloadPort <= 0 {
		payloadPort = types.DefaultSSHPayloadListenPort
	}
	payloadPath = sshpayload.NormalizePayloadPath(payloadPath)
	if payloadPath == "" {
		return fmt.Errorf("ssh-payload: missing payload path; run setup again")
	}

	if ContainerSkipNativeServices() {
		fmt.Printf("    [!] Container: skipped embedded SSH + SSH Payload OS services.\n")
		fmt.Printf("        Start foreground with: tunnelbypass run portable ssh-payload --port %d --payload-path %s\n", payloadPort, payloadPath)
		return nil
	}

	u := strings.TrimSpace(username)
	if u == "" {
		u = "tunnelbypass"
	}
	pw := strings.TrimSpace(password)

	if u != "" {
		if err := EnsureWindowsUser(u, pw, updatePassword, isAdmin); err != nil {
			fmt.Printf("    [!] Warning: Windows user creation failed: %v\n", err)
		}
	}

	fmt.Printf("\n    [*] Installing embedded SSH for SSH Payload...\n")
	StopEmbeddedSSHServer()

	udpgwPort, err := EnsureSSHUDPGW(types.DefaultUDPGWPort)
	if err != nil {
		return fmt.Errorf("UDPGW service: %w", err)
	}

	if err := installSSHServiceAsStandalone(u, pw, false, true, udpgwPort); err != nil {
		return fmt.Errorf("embedded SSH service: %w", err)
	}

	portCfg, _ := LoadSSHPortConfig()
	sshBack := portCfg.InternalPort
	if sshBack <= 0 {
		sshBack = GetSSHBackendPort()
	}
	if sshBack <= 0 {
		return fmt.Errorf("ssh-payload: could not determine embedded SSH backend port")
	}

	exe, err := resolveServiceExe()
	if err != nil {
		return err
	}
	args := []string{
		"run",
		"--portable",
		"--data-dir", GetBaseDir(),
		"--port", strconv.Itoa(payloadPort),
		"--ssh-port", strconv.Itoa(sshBack),
		"--udpgw-port", strconv.Itoa(udpgwPort),
		"--payload-path", payloadPath,
		"ssh-payload",
	}
	if u != "" {
		args = append(args[:len(args)-1], "--ssh-user", u, args[len(args)-1])
	}
	if pw != "" {
		args = append(args[:len(args)-1], "--ssh-password", pw, args[len(args)-1])
	}

	if err := CreateService(
		SSHPayloadServiceName,
		SSHPayloadServiceName+" (HTTP Payload)",
		exe,
		args,
		GetBaseDir(),
	); err != nil {
		return err
	}

	_ = OpenFirewallPort(payloadPort, "tcp", SSHPayloadServiceName)
	if err := waitPortListening(payloadPort, 30*time.Second); err != nil {
		return fmt.Errorf("ssh-payload service failed to listen on port %d: %w", payloadPort, err)
	}
	fmt.Printf("    [*] SSH Payload server is active on port %d -> SSH backend 127.0.0.1:%d\n", payloadPort, sshBack)
	fmt.Printf("    [*] Payload path: %s\n", payloadPath)
	return nil
}

func waitPortListening(port int, total time.Duration) error {
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		if PortListening(port) {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for port %d", port)
}
