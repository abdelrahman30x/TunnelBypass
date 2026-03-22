package installer

import (
	"context"
	"fmt"
	"os"
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

// TCP port for SSH forwards (22 system, or embedded listen).
func GetSSHBackendPort() int {
	return tbssh.BackendPort()
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

func sshServerMode() string {
	m := strings.ToLower(strings.TrimSpace(os.Getenv("TB_SSH_SERVER")))
	if m == "" {
		return "auto"
	}
	return m
}

func embedSSHListenPreference() int {
	return tbssh.ListenPreference()
}

func shouldTryPackageOpenSSH() bool {
	if runtime.GOOS == "linux" && os.Geteuid() != 0 {
		return false
	}
	return true
}

// EnsureSSHServerWithAuth ensures SSH is reachable: system OpenSSH when possible, else embedded server.
// Username/password are used for embedded mode; empty password triggers a generated password file under configs/ssh.
func EnsureSSHServerWithAuth(username, password string) error {
	// If port 22 is busy, we assume it's the system SSH. 
	// To avoid lockout or conflict, we MUST use the embedded server on a different port.
	if PortListening(22) {
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

func startEmbeddedSSHServer(username, password string) error {
	embedMu.Lock()
	defer embedMu.Unlock()
	if embedRunning {
		return nil
	}

	u := strings.TrimSpace(username)
	if u == "" {
		u = strings.TrimSpace(os.Getenv("TB_EMBED_SSH_USER"))
	}
	if u == "" {
		u = "tunnelbypass"
	}

	pw := password
	if pw == "" {
		pw = readOrCreateEmbedPassword()
	}

	port := EnsureFreeTCPPort(embedSSHListenPreference(), "sshembed")
	keyDir := GetConfigDir("ssh")
	_ = os.MkdirAll(keyDir, 0755)
	keyPath := filepath.Join(keyDir, "embed_host_key")

	cfg := tbssh.Config{
		ListenAddr: fmt.Sprintf("0.0.0.0:%d", port),
		Username:   u,
		Password:   pw,
		KeyPath:    keyPath,
		Logger:     tblog.Sub("tbssh"),
	}

	go func() {
		_ = tbssh.Run(context.Background(), cfg)
	}()

	for i := 0; i < 100; i++ {
		if PortListening(port) {
			setEmbedBackend(port)
			embedRunning = true
			fmt.Printf("    [*] Embedded SSH listening on port %d (user %q)\n", port, u)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("embedded SSH did not become ready on port %d", port)
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
	fmt.Printf("    [*] Embedded SSH password saved to %s\n", path)
	return p
}
