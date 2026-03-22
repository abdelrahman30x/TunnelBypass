package ssh

import "tunnelbypass/core/installer"

// EnsureSSHServer forwards to installer.
func EnsureSSHServer() error {
	return installer.EnsureSSHServer()
}

// EnsureSSHServerWithAuth forwards to installer (embedded SSH needs credentials).
func EnsureSSHServerWithAuth(username, password string) error {
	return installer.EnsureSSHServerWithAuth(username, password)
}
