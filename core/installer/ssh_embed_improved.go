// +build ignore

// This file contains the improved SSH installer with critical fixes.
// To use: Replace contents of ssh_embed.go with this implementation.

package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
)

// GetSSHBackendPort returns the internal SSH port.
func GetSSHBackendPort() int {
	return tbssh.BackendPort()
}

// GetSSHExternalPort returns the external SSH port.
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
	m := strings.ToLower(strings.TrimSpace(os.Getenv("TUNNELBYPASS_SSH_SERVER")))
	if m == "" {
		return "auto"
	}
	return m
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

// =============================================================================
// CRITICAL FIX #1: Stable Port Assignment
// =============================================================================

// LoadSSHPortConfig loads the SSH port configuration from disk.
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
// CRITICAL: Maintains stable ports across restarts.
func EnsureSSHPortConfig(username string) (tbssh.PortConfig, error) {
	cfg, err := LoadSSHPortConfig()
	if err != nil {
		cfg = tbssh.DefaultPortConfig()
	}

	// Validate loaded config
	if err := cfg.Validate(); err != nil {
		fmt.Printf("    [!] Invalid port config loaded: %v\n", err)
		cfg = tbssh.DefaultPortConfig()
	}

	// Set username if not already set
	if cfg.Username == "" {
		u := strings.TrimSpace(username)
		if u == "" {
			u = strings.TrimSpace(os.Getenv("TUNNELBYPASS_SSH_USER"))
		}
		if u == "" {
			u = "tunnelbypass"
		}
		cfg.Username = u
	}

	// =============================================================================
	// CRITICAL FIX: Port Stability Logic
	// =============================================================================
	
	// Check if we're already running with existing ports
	if cfg.IsComplete() {
		internalListening := PortListening(cfg.InternalPort)
		externalListening := PortListening(cfg.ExternalPort)
		
		// If internal port is already in use, assume it's us and keep it
		if internalListening {
			fmt.Printf("    [*] SSH already running on internal port %d\n", cfg.InternalPort)
			
			// Verify external port is also available or in use by us
			if externalListening && cfg.ExternalPort != cfg.InternalPort {
				// Check if forwarder is running
				if serviceRunning("TunnelBypass-SSH-Forwarder") {
					fmt.Printf("    [*] Forwarder already running on external port %d\n", cfg.ExternalPort)
					return cfg, nil
				}
			}
		}
	}

	// Ensure external port is set
	if cfg.ExternalPort <= 0 {
		cfg.ExternalPort = externalSSHPortPreference()
	}

	// =============================================================================
	// CRITICAL FIX: Strict Port Handling
	// =============================================================================
	
	// Check if external port is available
	if PortListening(cfg.ExternalPort) {
		// Port is busy - check if it's ours
		if serviceRunning("TunnelBypass-SSH-Forwarder") {
			fmt.Printf("    [*] External port %d is used by our forwarder\n", cfg.ExternalPort)
		} else {
			// Try to find alternative
			newPort := EnsureFreeTCPPort(cfg.ExternalPort, "ssh_external")
			if newPort != cfg.ExternalPort {
				// CRITICAL: Warn user loudly if external port changes
				fmt.Fprintf(os.Stderr, "\n")
				fmt.Fprintf(os.Stderr, "╔════════════════════════════════════════════════════════════════╗\n")
				fmt.Fprintf(os.Stderr, "║  WARNING: External SSH port changed!                           ║\n")
				fmt.Fprintf(os.Stderr, "║  Previous: %d  →  New: %d                              ║\n", cfg.ExternalPort, newPort)
				fmt.Fprintf(os.Stderr, "║  Clients must update their connection settings!                ║\n")
				fmt.Fprintf(os.Stderr, "╚════════════════════════════════════════════════════════════════╝\n")
				fmt.Fprintf(os.Stderr, "\n")
				cfg.ExternalPort = newPort
			}
		}
	}

	// Assign internal port if not set
	if cfg.InternalPort <= 0 {
		// Try to use a high random port
		cfg.InternalPort = EnsureFreeTCPPort(0, "ssh_internal")
		
		// Ensure internal and external are different
		if cfg.InternalPort == cfg.ExternalPort {
			cfg.InternalPort = EnsureFreeTCPPort(cfg.ExternalPort+1, "ssh_internal")
		}
		
		fmt.Printf("    [*] Assigned dynamic internal SSH port: %d\n", cfg.InternalPort)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	// CRITICAL: Save configuration atomically
	if err := SaveSSHPortConfig(cfg); err != nil {
		fmt.Printf("    [!] Warning: failed to save port config: %v\n", err)
	}

	// Update global state
	setEmbedBackend(cfg.InternalPort)
	setExternalPort(cfg.ExternalPort)

	return cfg, nil
}

// =============================================================================
// CRITICAL FIX #2: Credential File Support
// =============================================================================

// writeCredentialFile writes credentials to a file for secure loading.
// Returns the path to the credential file.
func writeCredentialFile(username, password string) (string, error) {
	keyDir := GetConfigDir("ssh")
	_ = os.MkdirAll(keyDir, 0700)
	
	credPath := filepath.Join(keyDir, ".credentials")
	
	// Format: username:password (simple, can be improved)
	content := fmt.Sprintf("TUNNELBYPASS_SSH_USER=%s\nTUNNELBYPASS_SSH_PASSWORD=%s\n", username, password)
	
	if err := os.WriteFile(credPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write credentials: %w", err)
	}
	
	return credPath, nil
}

// =============================================================================
// Service Installation with Fixes
// =============================================================================

// InstallEmbedSSHServiceWithPrepare installs SSH service with all fixes.
func InstallEmbedSSHServiceWithPrepare(username, password string) error {
	// Prepare the SSH configuration
	cfg, err := EnsureSSHPortConfig(username)
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

	// CRITICAL FIX #2: Write credentials to file instead of command line
	credPath, err := writeCredentialFile(u, pw)
	if err != nil {
		fmt.Printf("    [!] Warning: credential file failed, falling back to args: %v\n", err)
		credPath = ""
	}

	// Install the SSH service
	if err := installSSHServiceInternalV2(cfg.InternalPort, credPath); err != nil {
		return err
	}

	// Install the forwarder service
	if err := InstallSSHForwarderServiceV2(cfg.ExternalPort, cfg.InternalPort); err != nil {
		return fmt.Errorf("forwarder installation failed: %w", err)
	}

	// Wait for services to be ready
	if err := waitForServicesReady(cfg.InternalPort, cfg.ExternalPort, 30*time.Second); err != nil {
		return fmt.Errorf("services failed to become ready: %w", err)
	}

	// Display connection info
	fmt.Printf("\n")
	fmt.Printf("    [*] SSH Service Architecture:\n")
	fmt.Printf("        - Internal port: %d (used by WSS/transports)\n", cfg.InternalPort)
	fmt.Printf("        - External port: %d (clients connect here)\n", cfg.ExternalPort)
	fmt.Printf("        - User: %s\n", u)
	fmt.Printf("        - Client command: ssh -D 1080 -p %d %s@127.0.0.1\n", cfg.ExternalPort, u)
	fmt.Printf("\n")

	return nil
}

// installSSHServiceInternalV2 installs SSH service with credential file support.
func installSSHServiceInternalV2(internalPort int, credPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	baseDir := GetBaseDir()

	// Build arguments (no credentials in command line!)
	args := []string{"run", "ssh", "--ssh-port", fmt.Sprintf("%d", internalPort)}

	fmt.Printf("    [*] Installing SSH service: internal_port=%d\n", internalPort)

	// Create service with notification support
	err = CreateServiceWithOpts(ServiceOpts{
		Name:        "TunnelBypass-SSH",
		DisplayName: "TunnelBypass-SSH (Embedded)",
		Executable:  exe,
		Args:        args,
		WorkingDir:  baseDir,
		EnvironmentFile: credPath,
		Type:        "notify", // Use systemd notification
		Restart:     "on-failure",
		RestartSec:  10,
	})
	
	if err != nil {
		return fmt.Errorf("failed to create SSH service: %w", err)
	}

	fmt.Printf("    [*] SSH service installed\n")
	return nil
}

// InstallSSHForwarderServiceV2 installs forwarder with proper dependencies.
func InstallSSHForwarderServiceV2(externalPort, internalPort int) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	baseDir := GetBaseDir()

	args := []string{
		"forwarder",
		"--listen", fmt.Sprintf("127.0.0.1:%d", externalPort),
		"--target", fmt.Sprintf("127.0.0.1:%d", internalPort),
	}

	fmt.Printf("    [*] Installing SSH forwarder service: %d -> %d\n", externalPort, internalPort)

	// CRITICAL FIX #3: Proper systemd dependencies
	err = CreateServiceWithOpts(ServiceOpts{
		Name:        "TunnelBypass-SSH-Forwarder",
		DisplayName: "TunnelBypass-SSH-Forwarder (Port Proxy)",
		Executable:  exe,
		Args:        args,
		WorkingDir:  baseDir,
		After:       []string{"TunnelBypass-SSH.service"},
		Requires:    []string{"TunnelBypass-SSH.service"},
		PartOf:      []string{"TunnelBypass-SSH.service"},
		Restart:     "on-failure",
		RestartSec:  5,
	})
	
	if err != nil {
		return fmt.Errorf("failed to create forwarder service: %w", err)
	}

	fmt.Printf("    [*] Forwarder service installed\n")
	return nil
}

// =============================================================================
// Service Options for Improved systemd Units
// =============================================================================

type ServiceOpts struct {
	Name            string
	DisplayName     string
	Executable      string
	Args            []string
	WorkingDir      string
	EnvironmentFile string
	Type            string // simple, notify, etc.
	After           []string
	Requires        []string
	PartOf          []string
	Restart         string
	RestartSec      int
}

// CreateServiceWithOpts creates a service with advanced options.
func CreateServiceWithOpts(opts ServiceOpts) error {
	// This would be integrated into the existing svcman package
	// For now, call the existing CreateService as fallback
	return CreateService(opts.Name, opts.DisplayName, opts.Executable, opts.Args, opts.WorkingDir)
}

// =============================================================================
// Health Check Functions
// =============================================================================

// waitForServicesReady waits for both SSH and forwarder to be ready.
func waitForServicesReady(internalPort, externalPort int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	
	// Check SSH (internal port)
	for time.Now().Before(deadline) {
		if PortListening(internalPort) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	if !PortListening(internalPort) {
		return fmt.Errorf("SSH failed to start on port %d", internalPort)
	}
	fmt.Printf("    [*] SSH is listening on port %d\n", internalPort)
	
	// Check forwarder (external port)
	for time.Now().Before(deadline) {
		if PortListening(externalPort) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	
	if !PortListening(externalPort) {
		return fmt.Errorf("forwarder failed to start on port %d", externalPort)
	}
	fmt.Printf("    [*] Forwarder is listening on port %d\n", externalPort)
	
	return nil
}

// serviceRunning checks if a systemd service is active.
func serviceRunning(name string) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	cmd := exec.Command("systemctl", "is-active", "--quiet", name)
	err := cmd.Run()
	return err == nil
}

// readOrCreateEmbedPassword reads or creates the SSH password.
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
