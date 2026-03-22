package transport

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"tunnelbypass/core/types"
)

// ProvisionFunc generates server/client configs for one canonical transport name.
type ProvisionFunc func(log *slog.Logger, opt types.ConfigOptions, serverOut, clientOut string) (Result, error)

var (
	provMu      sync.RWMutex
	prov        = map[string]ProvisionFunc{}
	provAliases = map[string]string{}
)

// RegisterProvision registers the canonical name and optional aliases (e.g. vless -> reality).
func RegisterProvision(name string, aliases []string, fn ProvisionFunc) {
	provMu.Lock()
	defer provMu.Unlock()
	n := strings.ToLower(strings.TrimSpace(name))
	prov[n] = fn
	for _, a := range aliases {
		provAliases[strings.ToLower(strings.TrimSpace(a))] = n
	}
}

// CanonicalName maps aliases to the registered canonical transport key.
func CanonicalName(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if c, ok := provAliases[t]; ok {
		return c
	}
	return t
}

// Provision runs the registered provisioner for transport (after alias resolution).
func Provision(log *slog.Logger, transport string, opt types.ConfigOptions, serverOut, clientOut string) (Result, error) {
	t := strings.ToLower(strings.TrimSpace(transport))
	key := CanonicalName(t)
	provMu.RLock()
	fn, ok := prov[key]
	provMu.RUnlock()
	if !ok || fn == nil {
		var r Result
		return r, fmt.Errorf("provision: unknown transport %q", transport)
	}
	return fn(log, opt, serverOut, clientOut)
}
