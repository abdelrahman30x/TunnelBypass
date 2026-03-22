package ssh

import "tunnelbypass/core/installer"

// EnsureSshWstunnelServer forwards to installer (wstunnel + SSH stack).
func EnsureSshWstunnelServer(wssPort int, username, password string, updatePassword bool, isAdmin bool) error {
	return installer.EnsureSshWstunnelServer(wssPort, username, password, updatePassword, isAdmin)
}
