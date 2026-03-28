package tbssh

import (
	"sync/atomic"
)

var (
	backendPort  int32 = 0 // 0 = use dynamic port allocation
	externalPort int32 = 0 // 0 = use dynamic port allocation
	embedActive  atomic.Bool
)

// BackendPort returns the internal SSH port (used by WSS/transports).
// This is typically a dynamic/random port for embedded SSH.
func BackendPort() int {
	return int(atomic.LoadInt32(&backendPort))
}

// ExternalPort returns the external SSH port (client-facing).
// This is typically a dynamic port that clients connect to via the forwarder.
// Connections to this port are forwarded to the internal BackendPort.
func ExternalPort() int {
	return int(atomic.LoadInt32(&externalPort))
}

// EmbedActive reports whether the embedded SSH server is in use.
func EmbedActive() bool {
	return embedActive.Load()
}

// UseSystemBackend marks backend as system sshd.
// Port should be set separately via SetEmbedBackend or will use dynamic allocation.
func UseSystemBackend() {
	embedActive.Store(false)
	atomic.StoreInt32(&backendPort, 0)
}

// SetEmbedBackend records embedded listen port for config writers.
func SetEmbedBackend(port int) {
	atomic.StoreInt32(&backendPort, int32(port))
	embedActive.Store(true)
}

// SetExternalPort records the external (client-facing) port.
func SetExternalPort(port int) {
	atomic.StoreInt32(&externalPort, int32(port))
}

// ListenPreference returns 0 (dynamic port allocation).
func ListenPreference() int {
	return 0
}

// EmbedListenAllowPort22 reports whether binding embedded SSH on port 22 is allowed.
// Default false so embedded SSH does not compete with system sshd on 22.
func EmbedListenAllowPort22() bool {
	return false
}

// SanitizeEmbeddedListenPort avoids binding embedded SSH to port 22 (system sshd default).
func SanitizeEmbeddedListenPort(preferred int) int {
	if preferred != 22 {
		return preferred
	}
	if EmbedListenAllowPort22() {
		return 22
	}
	return 0
}

// ExternalPortPreference returns 0 (dynamic allocation).
func ExternalPortPreference() int {
	return 0
}

// WSSClientLocalSSHPort is the local TCP port on the *client* machine for wstunnel/ssh -p.
// Default 22 for standard client commands.
func WSSClientLocalSSHPort() int {
	return 22
}

// TLSClientLocalSSHPort is the local TCP port on the *client* machine where stunnel accepts
// (must match stunnel-client.conf "accept" and ssh -p). Not the server's forwarder port.
func TLSClientLocalSSHPort() int {
	return 2222
}
