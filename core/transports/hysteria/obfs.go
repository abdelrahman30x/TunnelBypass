package hysteria

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"tunnelbypass/core/types"
)

// effectiveObfsPassword returns Salamander PSK (≥4 chars). User-set ObfsPassword wins; otherwise a
// deterministic key derived from UUID so server, client, and sharing URLs stay in sync without storing opt.
func effectiveObfsPassword(opt types.ConfigOptions) string {
	p := strings.TrimSpace(opt.ObfsPassword)
	if len(p) >= 4 {
		return p
	}
	u := strings.TrimSpace(opt.UUID)
	if u == "" {
		u = "tunnelbypass"
	}
	sum := sha256.Sum256([]byte("tb/hysteria/salamander/v1:" + u))
	return base64.RawURLEncoding.EncodeToString(sum[:18])
}
