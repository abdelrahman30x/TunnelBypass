package ssh

import "tunnelbypass/core/installer"

// EnsureSshStunnelServer forwards to installer (stunnel + SSH stack).
func EnsureSshStunnelServer(sslPort int, username, password string, updatePassword bool, isAdmin bool) error {
	return installer.EnsureSshStunnelServer(sslPort, username, password, updatePassword, isAdmin)
}
