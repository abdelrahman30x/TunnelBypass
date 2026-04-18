package network

import "strings"

// HostMode is the network addressing strategy for this run.
type HostMode int

const (
	InternetMode HostMode = iota // default: public IP via HTTPS providers
	LANMode                      // --self-host: local network IP
)

// ParseHostMode converts a CLI/JSON string to a typed HostMode.
// Returns (mode, ok) — ok=false on unrecognised input.
func ParseHostMode(s string) (HostMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "lan", "self-host", "local":
		return LANMode, true
	case "", "internet", "public":
		return InternetMode, true
	default:
		return InternetMode, false
	}
}

// SecurityProfile describes how strictly the tunnel restricts LAN routing.
type SecurityProfile int

const (
	// StrictLAN (default): block tunnel from forwarding to other private IPs.
	StrictLAN SecurityProfile = iota
	// RelaxedLAN: allow tunnel traffic to reach any LAN host (--lan-relax).
	RelaxedLAN
)

// PrivateRoutingMode maps directly to what gets written into Xray routing rules.
type PrivateRoutingMode int

const (
	BlockPrivate PrivateRoutingMode = iota // geoip:private → block outbound
	AllowAllPrivate                        // omit geoip:private rule
)

// NetworkProfile is resolved once per run and passed read-only to all layers.
type NetworkProfile struct {
	Mode     HostMode
	Security SecurityProfile
	// PrimaryIP is the endpoint written into every client config and share link.
	PrimaryIP string
	// SelectedInterface is set by LAN detection (for trace logging).
	SelectedInterface string
	// ResolutionSource describes why this IP was chosen.
	ResolutionSource string
}

// PrivateRouting returns the routing policy to use for Xray-based transports.
func (p NetworkProfile) PrivateRouting() PrivateRoutingMode {
	if p.Mode == LANMode && p.Security == RelaxedLAN {
		return AllowAllPrivate
	}
	return BlockPrivate
}
