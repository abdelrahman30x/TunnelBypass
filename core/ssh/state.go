package tbssh

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

var (
	backendPort int32 = 22
	embedActive atomic.Bool
)

// TCP port for forwards: 22 (system sshd) or embedded listen port.
func BackendPort() int {
	return int(atomic.LoadInt32(&backendPort))
}

// EmbedActive reports whether the embedded SSH server is in use.
func EmbedActive() bool {
	return embedActive.Load()
}

// UseSystemBackend marks backend as system sshd on port 22.
func UseSystemBackend() {
	embedActive.Store(false)
	atomic.StoreInt32(&backendPort, 22)
}

// SetEmbedBackend records embedded listen port for config writers.
func SetEmbedBackend(port int) {
	atomic.StoreInt32(&backendPort, int32(port))
	embedActive.Store(true)
}

// TB_SSH_LISTEN or 2222.
func ListenPreference() int {
	if p := strings.TrimSpace(os.Getenv("TB_SSH_LISTEN")); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			return n
		}
	}
	return 2222
}
