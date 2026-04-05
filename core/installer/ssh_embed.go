package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tbssh "tunnelbypass/core/ssh"
	"tunnelbypass/internal/tblog"
	"tunnelbypass/internal/utils"
)

var (
	embedMu      sync.Mutex
	embedRunning bool
	embedCancel  context.CancelFunc // Store cancel function to stop embedded server

	// sshServerForwarder controls whether EnsureSSHPortConfig allocates ExternalPort and
	// a TCP forwarder. False for WSS-only: SSH is only exposed to 127.0.0.1:InternalPort.
	sshServerForwarder = true
)

// SetSSHServerForwarder sets whether the server should allocate/listen on ExternalPort (forwarder).
// WSS provisioning sets this to false so the host does not open an extra client-facing SSH port.
func SetSSHServerForwarder(enabled bool) {
	sshServerForwarder = enabled
}

func sshServerForwarderEnabled() bool {
	return sshServerForwarder
}

// TCP port for SSH forwards (22 system, or embedded listen).
func GetSSHBackendPort() int {
	return tbssh.BackendPort()
}

// GetSSHExternalPort returns the external (client-facing) SSH port.
func GetSSHExternalPort() int {
	return tbssh.ExternalPort()
}

// SSHEmbedActive reports whether the embedded SSH server is used.
func SSHEmbedActive() bool {
	return tbssh.EmbedActive()
}

func useSystemSSHBackend() {
	tbssh.UseSystemBackend()
}

func setEmbedBackend(port int) {
	tbssh.SetEmbedBackend(port)
}

func setExternalPort(port int) {
	tbssh.SetExternalPort(port)
}

func sshServerMode() string {
	return "auto"
}

func embedSSHListenPreference() int {
	return tbssh.ListenPreference()
}

func externalSSHPortPreference() int {
	return tbssh.ExternalPortPreference()
}

func shouldTryPackageOpenSSH() bool {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		return false
	}
	return true
}

// LoadSSHPortConfig loads the SSH port configuration from disk.
// Creates default config if not exists.
func LoadSSHPortConfig() (tbssh.PortConfig, error) {
	keyDir := GetConfigDir("ssh")
	_ = os.MkdirAll(keyDir, 0755)
	return tbssh.LoadPortConfig(keyDir)
}

// SaveSSHPortConfig saves the SSH port configuration to disk.
func SaveSSHPortConfig(cfg tbssh.PortConfig) error {
	keyDir := GetConfigDir("ssh")
	_ = os.MkdirAll(keyDir, 0755)
	return cfg.Save(keyDir)
}

// EnsureSSHPortConfig ensures a valid port configuration exists.
// Assigns dynamic internal port if needed; allocates ExternalPort only when server forwarder is enabled.
func EnsureSSHPortConfig(username string) (tbssh.PortConfig, error) {
	return ensureSSHPortConfig(username, sshServerForwarderEnabled())
}

// ensureSSHPortConfig implements port selection. When serverForwarder is false, ExternalPort stays 0
// (no allocation, no listener): clients use transports (e.g. wstunnel) to InternalPort only.
func ensureSSHPortConfig(username string, serverForwarder bool) (tbssh.PortConfig, error) {
	cfg, err := LoadSSHPortConfig()
	if err != nil {
		cfg = tbssh.DefaultPortConfig()
	}

	// Set username if not already set
	if cfg.Username == "" {
		u := strings.TrimSpace(username)
		if u == "" {
			u = "tunnelbypass"
		}
		cfg.Username = u
	}

	if !serverForwarder {
		cfg.ExternalPort = 0
	} else {
		// Ensure external port is set and available
		if cfg.ExternalPort <= 0 {
			cfg.ExternalPort = externalSSHPortPreference()
			// If still 0 (dynamic allocation requested), assign a free port
			if cfg.ExternalPort <= 0 {
				cfg.ExternalPort = EnsureFreeTCPPort(0, "ssh_external")
				fmt.Printf("    [*] Assigned dynamic external SSH port: %d\n", cfg.ExternalPort)
			}
		}

		// Check if external port is available, if not try to find another
		if PortListening(cfg.ExternalPort) {
			newPort := EnsureFreeTCPPort(cfg.ExternalPort, "ssh_external")
			if newPort != cfg.ExternalPort {
				fmt.Printf("    [!] External SSH port %d is busy, using port %d instead\n", cfg.ExternalPort, newPort)
				cfg.ExternalPort = newPort
			}
		}
	}

	// Assign internal port if not set (dynamic allocation)
	if cfg.InternalPort <= 0 {
		cfg.InternalPort = EnsureFreeTCPPort(0, "ssh_internal")
		fmt.Printf("    [*] Assigned dynamic internal SSH port: %d\n", cfg.InternalPort)
	}
	// Avoid embedded listener on 22 (system sshd default) unless explicitly allowed.
	if cfg.InternalPort == 22 && !tbssh.EmbedListenAllowPort22() {
		cfg.InternalPort = EnsureFreeTCPPort(0, "ssh_internal")
		fmt.Printf("    [*] Internal SSH port moved off 22 (system sshd default)\n")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	// Save configuration for persistence across restarts
	if err := SaveSSHPortConfig(cfg); err != nil {
		fmt.Printf("    [!] Warning: failed to save port config: %v\n", err)
	}

	// Update global state
	setEmbedBackend(cfg.InternalPort)
	setExternalPort(cfg.ExternalPort)

	return cfg, nil
}

// EnsureSSHServerWithAuth ensures SSH is reachable: system OpenSSH when possible, else embedded server.
// Username/password are used for embedded mode; empty password triggers a generated password file under configs/ssh.
func EnsureSSHServerWithAuth(username, password string) error {
	// If port 22 is busy, we assume it's the system SSH.
	// To avoid lockout or conflict, we MUST use the embedded server on a different port.
	if PortListening(22) {
		// If the saved internal SSH port is already listening (e.g. a service started it in a
		// separate process), adopt that existing instance.  Attempting to bind the same port from
		// this process would fail, falsely report success, and cause a password mismatch.
		if savedCfg, err := LoadSSHPortConfig(); err == nil &&
			savedCfg.InternalPort > 0 && PortListening(savedCfg.InternalPort) {
			setEmbedBackend(savedCfg.InternalPort)
			setExternalPort(savedCfg.ExternalPort)
			embedMu.Lock()
			embedRunning = true
			embedMu.Unlock()
			fmt.Printf("    [*] Embedded SSH already listening on port %d — adopting existing instance\n",
				savedCfg.InternalPort)
			return nil
		}
		return startEmbeddedSSHServer(username, password)
	}

	mode := sshServerMode()
	switch mode {
	case "system":
		err := ensureOpenSSHServerInstall()
		if err != nil {
			return err
		}
		// Only configure system SSH if we are root/admin.
		if username != "" {
			EnsureManagedSSHConfig(username)
		} else {
			EnsureSaneSSHConfig()
		}
		useSystemSSHBackend()
		return nil
	case "embed", "embedded":
		return startEmbeddedSSHServer(username, password)
	default: // auto
		// If port 22 was free, we can try to install system SSH only if we are root.
		if shouldTryPackageOpenSSH() {
			if err := ensureOpenSSHServerInstall(); err == nil && PortListening(22) {
				if username != "" {
					EnsureManagedSSHConfig(username)
				} else {
					EnsureSaneSSHConfig()
				}
				useSystemSSHBackend()
				return nil
			}
		}
		return startEmbeddedSSHServer(username, password)
	}
}

// prepareEmbeddedSSHServer prepares the embedded SSH configuration (port, credentials, keys)
// without starting the server. This is used when installing SSH as a service.
// Returns the internal port (for WSS), external port (for clients), username, and password.
func prepareEmbeddedSSHServer(username, password string) (internalPort int, externalPort int, user string, pass string, err error) {
	embedMu.Lock()
	defer embedMu.Unlock()
	if embedRunning {
		// Already running, return the current ports
		return GetSSHBackendPort(), GetSSHExternalPort(), "", "", nil
	}

	// Ensure port configuration exists
	cfg, err := EnsureSSHPortConfig(username)
	if err != nil {
		return 0, 0, "", "", fmt.Errorf("failed to ensure port config: %w", err)
	}

	u := cfg.Username
	if strings.TrimSpace(username) != "" {
		u = strings.TrimSpace(username)
	}

	pw := password
	if pw == "" {
		pw = readOrCreateEmbedPassword()
	} else {
		// Persist the provided password so subsequent wizard runs and service
		// installs can read the same value from embed_password.txt instead of
		// generating a fresh UUID that would not match the running service.
		passPath := filepath.Join(GetConfigDir("ssh"), "embed_password.txt")
		_ = os.MkdirAll(filepath.Dir(passPath), 0755)
		_ = os.WriteFile(passPath, []byte(pw), 0600)
	}

	keyDir := GetConfigDir("ssh")
	_ = os.MkdirAll(keyDir, 0755)

	fmt.Printf("    [*] Embedded SSH configured:\n")
	fmt.Printf("        - Internal port: %d (for WSS/transports)\n", cfg.InternalPort)
	if cfg.ExternalPort > 0 {
		fmt.Printf("        - External port: %d (TCP forwarder for direct clients)\n", cfg.ExternalPort)
	} else {
		fmt.Printf("        - External port: (none — no server forwarder; use tunnel to internal port)\n")
	}
	fmt.Printf("        - User: %s\n", u)

	return cfg.InternalPort, cfg.ExternalPort, u, pw, nil
}

func startEmbeddedSSHServer(username, password string) error {
	embedMu.Lock()
	if embedRunning {
		embedMu.Unlock()
		return nil
	}
	embedMu.Unlock()

	internalPort, externalPort, u, pw, err := prepareEmbeddedSSHServer(username, password)
	if err != nil {
		return err
	}

	keyDir := GetConfigDir("ssh")
	keyPath := filepath.Join(keyDir, "embed_host_key")

	// Start SSH on internal port only (localhost binding for security)
	cfg := tbssh.Config{
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", internalPort),
		Username:   u,
		Password:   pw,
		KeyPath:    keyPath,
		Logger:     tblog.Sub("tbssh"),
	}

	// Create cancellable context so we can stop the server later
	ctx, cancel := context.WithCancel(context.Background())

	// Capture early bind failures from the goroutine so we can distinguish
	// "port in use by another process" from "not ready yet".
	bindErrCh := make(chan error, 1)
	go func() {
		err := tbssh.Run(ctx, cfg)
		if err != nil && ctx.Err() == nil {
			select {
			case bindErrCh <- err:
			default:
			}
		}
	}()

	// Give the goroutine a moment to attempt the bind before we start polling.
	time.Sleep(80 * time.Millisecond)

	for i := 0; i < 100; i++ {
		// If the goroutine already returned an error, the port is not ours.
		select {
		case bindErr := <-bindErrCh:
			cancel()
			return fmt.Errorf("embedded SSH failed to bind port %d: %w", internalPort, bindErr)
		default:
		}

		if PortListening(internalPort) {
			setEmbedBackend(internalPort)
			setExternalPort(externalPort)
			embedMu.Lock()
			embedRunning = true
			embedCancel = cancel
			embedMu.Unlock()
			if externalPort > 0 {
				fmt.Printf("    [*] Embedded SSH listening on internal port %d (forwarder external: %d, user: %s)\n",
					internalPort, externalPort, u)
			} else {
				fmt.Printf("    [*] Embedded SSH listening on internal port %d (no forwarder, user: %s)\n",
					internalPort, u)
			}
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel() // Clean up context if server didn't start
	return fmt.Errorf("embedded SSH did not become ready on port %d", internalPort)
}

// StopEmbeddedSSHServer stops the embedded SSH server and releases the port.
// This should be called before installing the standalone systemd service.
func StopEmbeddedSSHServer() {
	embedMu.Lock()
	if !embedRunning {
		embedMu.Unlock()
		return
	}

	port := GetSSHBackendPort()

	fmt.Printf("    [*] Stopping embedded SSH server to release port for systemd service...\n")

	if embedCancel != nil {
		embedCancel()
		embedCancel = nil
	}
	embedRunning = false
	embedMu.Unlock()

	time.Sleep(500 * time.Millisecond)

	// Windows: the loopback listener can linger briefly; the replacement service must bind the same port.
	// Wait until nothing accepts on the port, then add a short cooldown (TIME_WAIT / SCM startup).
	if runtime.GOOS == "windows" && port > 0 {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if !PortListening(port) {
				break
			}
			time.Sleep(150 * time.Millisecond)
		}
		time.Sleep(2 * time.Second)
	}

	fmt.Printf("    [*] Embedded SSH server stopped, port released\n")
}

// ReadOrCreateEmbedSSHPassword loads or generates the embedded SSH password file under configs/ssh.
func ReadOrCreateEmbedSSHPassword() string {
	return readOrCreateEmbedPassword()
}

func readOrCreateEmbedPassword() string {
	path := filepath.Join(GetConfigDir("ssh"), "embed_password.txt")
	if b, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" {
			return s
		}
	}
	p := utils.GenerateUUID()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte(p), 0600)
	return p
}

// InstallEmbedSSHService installs the embedded SSH as a background OS service.
// It should be called AFTER EnsureSSHServerWithAuth has determined the SSH port.
// The service will be started automatically by the service manager (systemd on Linux).
// After successful installation, the in-process SSH server is no longer needed as the service takes over.
func InstallEmbedSSHService() error {
	if !SSHEmbedActive() {
		fmt.Printf("    [*] SSH embed not active, skipping service install.\n")
		return nil
	}
	internalPort := GetSSHBackendPort()
	if internalPort <= 0 {
		return fmt.Errorf("invalid SSH internal port: %d", internalPort)
	}
	return installSSHServiceInternal(internalPort, "", "", false, 0)
}

// installSSHServiceInternal installs the SSH service with specific credentials.
// externalUDPGW adds --external-udpgw (UDPGW provided by TunnelBypass-UDPGW service).
// udpgwPort is the TCP port UDPGW listens on when externalUDPGW is true (0 = omit flag; engine defaults to 7300).
func installSSHServiceInternal(internalPort int, username, password string, externalUDPGW bool, udpgwPort int) error {
	exe, err := resolveServiceExe()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	baseDir := GetBaseDir()

	u := strings.TrimSpace(username)
	pw := strings.TrimSpace(password)
	if u == "" {
		u = "tunnelbypass"
	}
	if pw == "" {
		pw = readOrCreateEmbedPassword()
	}

	// For systemd service, we use 'run ssh --ssh-port X' (system mode, not portable)
	// The SSH will listen on 127.0.0.1:internalPort
	args := []string{"run", "ssh", "--ssh-port", fmt.Sprintf("%d", internalPort), "--ssh-user", u, "--ssh-password", pw}
	if externalUDPGW {
		args = append(args, "--external-udpgw")
		if udpgwPort > 0 {
			args = append(args, "--udpgw-port", strconv.Itoa(udpgwPort))
		}
	}

	fmt.Printf("    [*] Installing SSH service: internal_port=%d dir=%s\n", internalPort, baseDir)

	err = CreateService("TunnelBypass-SSH", "TunnelBypass-SSH (Embedded)", exe, args, baseDir)
	if err != nil {
		return fmt.Errorf("failed to create SSH service: %w", err)
	}

	fmt.Printf("    [*] Embedded SSH has been installed as a background OS service.\n")

	// Windows (WinSCM/WinSW): child may need extra time before loopback accepts connections.
	if runtime.GOOS == "windows" {
		time.Sleep(3 * time.Second)
	} else {
		time.Sleep(1 * time.Second)
	}

	// Verify the service is running
	if !serviceRunning("TunnelBypass-SSH") {
		fmt.Printf("    [!] Warning: TunnelBypass-SSH service may not have started.\n")
		printSSHServicePostInstallHints()
	} else {
		fmt.Printf("    [*] SSH service confirmed running.\n")
	}
	return nil
}

// InstallSSHForwarderService installs the port forwarder service that maps
// external port (e.g., 2222) to internal SSH port (e.g., 33506).
func InstallSSHForwarderService(externalPort, internalPort int) error {
	if externalPort <= 0 || internalPort <= 0 {
		return fmt.Errorf("invalid forwarder ports: external=%d internal=%d", externalPort, internalPort)
	}
	exe, err := resolveServiceExe()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	baseDir := GetBaseDir()

	// Create forwarder service arguments
	args := []string{
		"forwarder",
		"--listen", fmt.Sprintf("127.0.0.1:%d", externalPort),
		"--target", fmt.Sprintf("127.0.0.1:%d", internalPort),
	}

	fmt.Printf("    [*] Installing SSH forwarder service: %d -> %d\n", externalPort, internalPort)

	err = CreateService(
		"TunnelBypass-SSH-Forwarder",
		"TunnelBypass-SSH-Forwarder (Port Proxy)",
		exe,
		args,
		baseDir,
	)
	if err != nil {
		return fmt.Errorf("failed to create forwarder service: %w", err)
	}

	fmt.Printf("    [*] SSH forwarder has been installed as a background OS service.\n")

	// Give the service a moment to start
	time.Sleep(500 * time.Millisecond)

	// Verify the service is running
	if !serviceRunning("TunnelBypass-SSH-Forwarder") {
		fmt.Printf("    [!] Warning: TunnelBypass-SSH-Forwarder service may not have started.\n")
	} else {
		fmt.Printf("    [*] SSH forwarder service confirmed running.\n")
	}
	return nil
}

func serviceRunning(name string) bool {
	if runtime.GOOS == "linux" {
		return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("sc", "query", name).CombinedOutput()
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToUpper(string(out)), "RUNNING")
	}
	return true
}

// InstallEmbedSSHServiceWithPrepare prepares and installs the embedded SSH as a background OS service.
// Set externalUDPGW true when TunnelBypass-UDPGW was installed first (TLS path).
// udpgwPort is the TCP port UDPGW listens on (from EnsureSSHUDPGW); use 0 when externalUDPGW is false.
func InstallEmbedSSHServiceWithPrepare(username, password string, externalUDPGW bool, udpgwPort int) error {
	return installSSHServiceAsStandalone(username, password, true, externalUDPGW, udpgwPort)
}

// InstallSSHServiceAsStandalone installs SSH as a proper standalone systemd service.
// This is the FIXED version that ensures complete independence from CLI.
// The service will:
//   - Start on boot
//   - Auto-restart on failure
//   - Log to journald (visible via journalctl -u TunnelBypass-SSH)
//   - Run independently of the CLI process
func InstallSSHServiceAsStandalone(username, password string) error {
	return installSSHServiceAsStandalone(username, password, true, false, 0)
}

// installSSHServiceAsStandalone installs embedded SSH for WSS/TLS (and full standalone on Linux).
// Linux uses a native systemd unit; other platforms use CreateService (WinSW / launchd / user supervisor).
// externalUDPGW: set true when TunnelBypass-UDPGW is already installed (WSS/TLS wizard path).
func installSSHServiceAsStandalone(username, password string, installForwarder bool, externalUDPGW bool, udpgwPort int) error {
	if runtime.GOOS == "linux" {
		return installSSHServiceAsStandaloneLinux(username, password, installForwarder, externalUDPGW, udpgwPort)
	}
	return installSSHServiceAsStandaloneGeneric(username, password, installForwarder, externalUDPGW, udpgwPort)
}

// installSSHServiceAsStandaloneGeneric registers TunnelBypass-SSH via the portable service stack
// (same mechanism as other transports on Windows and Linux).
func installSSHServiceAsStandaloneGeneric(username, password string, installForwarder bool, externalUDPGW bool, udpgwPort int) error {
	cfg, err := ensureSSHPortConfig(username, installForwarder)
	if err != nil {
		return fmt.Errorf("failed to ensure port config: %w", err)
	}

	u := cfg.Username
	if strings.TrimSpace(username) != "" {
		u = strings.TrimSpace(username)
	}

	pw := password
	if pw == "" {
		pw = readOrCreateEmbedPassword()
	}

	fmt.Printf("\n")
	fmt.Printf("    [*] Installing SSH Service (%s)\n", runtime.GOOS)
	fmt.Printf("        - Internal Port: %d\n", cfg.InternalPort)
	if cfg.ExternalPort > 0 {
		fmt.Printf("        - External Port: %d (TCP forwarder)\n", cfg.ExternalPort)
	} else {
		fmt.Printf("        - External Port: (none — tunnel-only, no host forwarder)\n")
	}
	fmt.Printf("        - User: %s\n", u)

	if err := installSSHServiceInternal(cfg.InternalPort, u, pw, externalUDPGW, udpgwPort); err != nil {
		return err
	}
	verifyTimeout := 45 * time.Second
	if runtime.GOOS == "windows" {
		verifyTimeout = 120 * time.Second
	}
	if err := verifyEmbeddedSSHListening(cfg.InternalPort, verifyTimeout); err != nil {
		return fmt.Errorf("SSH service failed to listen: %w", err)
	}

	if installForwarder && cfg.ExternalPort > 0 {
		if err := InstallSSHForwarderService(cfg.ExternalPort, cfg.InternalPort); err != nil {
			fmt.Printf("    [!] Warning: forwarder service installation failed: %v\n", err)
		}
	} else {
		fmt.Printf("    [*] Skipping SSH forwarder (not used for this transport).\n")
	}

	fmt.Printf("\n")
	fmt.Printf("    [✓] SSH Service Installed Successfully\n")
	printSSHServiceManagementHints()
	return nil
}

func installSSHServiceAsStandaloneLinux(username, password string, installForwarder bool, externalUDPGW bool, udpgwPort int) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root to install systemd service")
	}

	// Step 1: Ensure port configuration (generate if needed)
	cfg, err := ensureSSHPortConfig(username, installForwarder)
	if err != nil {
		return fmt.Errorf("failed to ensure port config: %w", err)
	}

	u := cfg.Username
	if strings.TrimSpace(username) != "" {
		u = strings.TrimSpace(username)
	}

	pw := password
	if pw == "" {
		pw = readOrCreateEmbedPassword()
	}

	fmt.Printf("\n")
	fmt.Printf("    [*] Installing SSH Service\n")
	fmt.Printf("        - Internal Port: %d\n", cfg.InternalPort)
	if cfg.ExternalPort > 0 {
		fmt.Printf("        - External Port: %d (TCP forwarder)\n", cfg.ExternalPort)
	} else {
		fmt.Printf("        - External Port: (none — tunnel-only, no host forwarder)\n")
	}
	fmt.Printf("        - User: %s\n", u)

	// Step 2: Create the systemd service unit file DIRECTLY
	if err := createSSHSystemdUnit(cfg.InternalPort, u, pw, externalUDPGW, udpgwPort); err != nil {
		return fmt.Errorf("failed to create systemd unit: %w", err)
	}

	// Step 3: Reload systemd and start service
	if err := startAndEnableSSHService(); err != nil {
		return fmt.Errorf("failed to start SSH service: %w", err)
	}

	// Step 4: Verify service is actually running
	if err := verifySSHServiceRunning(30 * time.Second); err != nil {
		return fmt.Errorf("SSH service failed to start: %w", err)
	}

	// Step 5: Install forwarder service (external -> internal port mapping), unless WSS/tunnel-only
	if installForwarder && cfg.ExternalPort > 0 {
		if err := InstallSSHForwarderService(cfg.ExternalPort, cfg.InternalPort); err != nil {
			fmt.Printf("    [!] Warning: forwarder service installation failed: %v\n", err)
		}
	} else {
		fmt.Printf("    [*] Skipping SSH forwarder (not used for this transport).\n")
	}

	fmt.Printf("\n")
	fmt.Printf("    [✓] SSH Service Installed Successfully\n")
	printSSHServiceManagementHintsLinux()

	return nil
}

func verifyEmbeddedSSHListening(port int, timeout time.Duration) error {
	if port <= 0 {
		return fmt.Errorf("invalid SSH internal port: %d", port)
	}
	poll := 300 * time.Millisecond
	if runtime.GOOS == "windows" {
		poll = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if PortListening(port) {
			return nil
		}
		time.Sleep(poll)
	}
	return fmt.Errorf("timeout waiting for port %d to listen", port)
}

func printSSHServicePostInstallHints() {
	switch runtime.GOOS {
	case "windows":
		fmt.Printf("        Check logs: %s\n", filepath.Join(GetBaseDir(), "logs"))
		fmt.Printf("        Or Services (services.msc) → TunnelBypass-SSH\n")
	case "linux":
		fmt.Printf("        Check with: systemctl status TunnelBypass-SSH\n")
		fmt.Printf("        Or view logs: journalctl -u TunnelBypass-SSH -n 50\n")
	default:
		fmt.Printf("        Check logs: %s\n", filepath.Join(GetBaseDir(), "logs"))
	}
}

func printSSHServiceManagementHints() {
	switch runtime.GOOS {
	case "windows":
		fmt.Printf("\n    Management: Services (services.msc) or `sc query TunnelBypass-SSH`; logs: %s\n", filepath.Join(GetBaseDir(), "logs"))
	case "linux":
		fmt.Printf("\n    Management: systemctl status / journalctl -u TunnelBypass-SSH\n")
	default:
		fmt.Printf("\n    Management: logs under %s\n", filepath.Join(GetBaseDir(), "logs"))
	}
}

func printSSHServiceManagementHintsLinux() {
	fmt.Printf("\n")
	fmt.Printf("    Management Commands:\n")
	fmt.Printf("        systemctl status TunnelBypass-SSH\n")
	fmt.Printf("        journalctl -u TunnelBypass-SSH -f\n")
	fmt.Printf("        systemctl stop TunnelBypass-SSH\n")
	fmt.Printf("        systemctl start TunnelBypass-SSH\n")
	fmt.Printf("\n")
}

// resolveServiceExe returns the path the systemd unit should use for ExecStart.
// If the current binary lives under /root or /home (which ProtectHome=true blocks),
// copy it to /usr/local/bin so the service can access it.
func resolveServiceExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable symlink: %w", err)
	}

	// ProtectHome=true hides /root, /home, and /run/user from the service process.
	// If the binary is under any of those paths, install a copy to /usr/local/bin.
	protected := []string{"/root/", "/home/", "/run/user/"}
	needsCopy := false
	for _, p := range protected {
		if strings.HasPrefix(exe, p) {
			needsCopy = true
			break
		}
	}

	if needsCopy {
		dest := "/usr/local/bin/tunnelbypass"
		fmt.Printf("    [*] Binary is under a protected path (%s); copying to %s for service use\n", exe, dest)
		src, err := os.Open(exe)
		if err != nil {
			return "", fmt.Errorf("failed to open binary for copy: %w", err)
		}
		defer src.Close()
		tmp := dest + ".tmp"
		dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return "", fmt.Errorf("failed to create %s: %w", tmp, err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			os.Remove(tmp)
			return "", fmt.Errorf("failed to copy binary: %w", err)
		}
		dst.Close()
		if err := os.Rename(tmp, dest); err != nil {
			return "", fmt.Errorf("failed to move binary to %s: %w", dest, err)
		}
		return dest, nil
	}
	return exe, nil
}

// createSSHSystemdUnit creates the systemd unit file directly.
func createSSHSystemdUnit(internalPort int, username, password string, externalUDPGW bool, udpgwPort int) error {
	exe, err := resolveServiceExe()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	baseDir := GetBaseDir()
	unitPath := "/etc/systemd/system/TunnelBypass-SSH.service"

	credPath := filepath.Join(GetConfigDir("ssh"), ".service_credentials")
	credContent := fmt.Sprintf("TUNNELBYPASS_SSH_USER=%s\nTUNNELBYPASS_SSH_PASSWORD=%s\n", username, password)
	if err := os.WriteFile(credPath, []byte(credContent), 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	execStart := fmt.Sprintf("%s run ssh --ssh-port %d", exe, internalPort)
	if externalUDPGW {
		execStart += " --external-udpgw"
		if udpgwPort > 0 {
			execStart += fmt.Sprintf(" --udpgw-port %d", udpgwPort)
		}
	}

	afterLine := "After=network-online.target"
	wantsUnit := ""
	if externalUDPGW {
		afterLine = "After=network-online.target TunnelBypass-UDPGW.service"
		wantsUnit = "Wants=TunnelBypass-UDPGW.service\n"
	}

	content := fmt.Sprintf(`[Unit]
Description=TunnelBypass Embedded SSH Server
Documentation=https://github.com/tunnelbypass/tunnelbypass
%s
Wants=network-online.target
%s
[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
Restart=always
RestartSec=5
StartLimitInterval=60
StartLimitBurst=3

StandardOutput=journal
StandardError=journal
StandardInput=null
SyslogIdentifier=tunnelbypass-ssh

LimitNOFILE=65536
LimitNPROC=4096

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=%s/configs/ssh %s/logs

EnvironmentFile=%s

[Install]
WantedBy=multi-user.target
`, afterLine, wantsUnit, execStart, baseDir, baseDir, baseDir, credPath)

	if err := os.WriteFile(unitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	fmt.Printf("    [*] Created systemd unit: %s\n", unitPath)
	return nil
}

// startAndEnableSSHService reloads systemd, enables and starts the SSH service.
func startAndEnableSSHService() error {
	// Reload systemd to pick up new unit file
	cmd := exec.Command("systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %v, output: %s", err, output)
	}
	fmt.Printf("    [*] Reloaded systemd\n")

	// Stop any existing service first (clean start)
	exec.Command("systemctl", "stop", "TunnelBypass-SSH").Run()
	time.Sleep(500 * time.Millisecond)

	// Enable the service (auto-start on boot)
	cmd = exec.Command("systemctl", "enable", "TunnelBypass-SSH")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable failed: %v, output: %s", err, output)
	}
	fmt.Printf("    [*] Enabled service for auto-start on boot\n")

	// Start the service
	cmd = exec.Command("systemctl", "start", "TunnelBypass-SSH")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl start failed: %v, output: %s", err, output)
	}
	fmt.Printf("    [*] Started service\n")

	return nil
}

// verifySSHServiceRunning waits for the SSH service to be active and listening.
func verifySSHServiceRunning(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	checkInterval := 500 * time.Millisecond

	fmt.Printf("    [*] Verifying SSH service is ready...")

	for time.Now().Before(deadline) {
		// Check if service is active
		cmd := exec.Command("systemctl", "is-active", "--quiet", "TunnelBypass-SSH")
		if err := cmd.Run(); err == nil {
			// Service is active, now check if it's actually listening
			cfg, _ := LoadSSHPortConfig()
			if cfg.InternalPort > 0 && PortListening(cfg.InternalPort) {
				fmt.Printf(" OK (port %d)\n", cfg.InternalPort)
				return nil
			}
		}

		time.Sleep(checkInterval)
		fmt.Print(".")
	}

	fmt.Printf(" FAILED\n")

	// Get service status for debugging
	fmt.Printf("\n    [!] Service status:\n")
	cmd := exec.Command("systemctl", "status", "TunnelBypass-SSH", "--no-pager")
	output, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", string(output))

	// Get recent logs
	fmt.Printf("    [!] Recent logs:\n")
	cmd = exec.Command("journalctl", "-u", "TunnelBypass-SSH", "-n", "20", "--no-pager")
	logs, _ := cmd.CombinedOutput()
	fmt.Printf("%s\n", string(logs))

	return fmt.Errorf("timeout waiting for SSH service to be ready")
}

// GetSSHConnectionInfo returns connection information for clients.
func GetSSHConnectionInfo() (externalPort int, username string, err error) {
	cfg, err := LoadSSHPortConfig()
	if err != nil {
		return 0, "", err
	}
	return cfg.ExternalPort, cfg.Username, nil
}

// GetSSHServiceStatus returns the current status of the SSH service.
// Returns: active (bool), listening (bool), port (int), error
func GetSSHServiceStatus() (active bool, listening bool, port int, err error) {
	cfg, err := LoadSSHPortConfig()
	if err != nil {
		return false, false, 0, err
	}

	listening = PortListening(cfg.InternalPort)

	if runtime.GOOS == "linux" {
		cmd := exec.Command("systemctl", "is-active", "--quiet", "TunnelBypass-SSH")
		active = cmd.Run() == nil
	} else {
		// Best-effort: if the port is listening, treat the service as active.
		active = listening
	}

	return active, listening, cfg.InternalPort, nil
}

// PrintSSHServiceStatus prints the current status of the SSH service.
func PrintSSHServiceStatus() {
	active, listening, port, err := GetSSHServiceStatus()
	if err != nil {
		fmt.Printf("SSH Service Status: ERROR - %v\n", err)
		return
	}

	status := "STOPPED"
	if active {
		status = "ACTIVE"
	}
	if listening {
		status += " (LISTENING)"
	} else if active {
		status += " (NOT LISTENING)"
	}

	fmt.Printf("SSH Service Status: %s on port %d\n", status, port)

	if active {
		switch runtime.GOOS {
		case "linux":
			fmt.Printf("  Check logs: journalctl -u TunnelBypass-SSH -n 20\n")
		default:
			fmt.Printf("  Check logs: %s\n", filepath.Join(GetBaseDir(), "logs"))
		}
	}
}
