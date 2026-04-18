package network

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// ValidateReachability checks that PrimaryIP is reachable before printing summary.
// Returns empty for UDP-only transports. Never fatal — returns a warning string.
func ValidateReachability(prof NetworkProfile, transport string, port int) string {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "hysteria", "wireguard":
		return ""
	}
	if port <= 0 || port > 65535 {
		return ""
	}
	addr := net.JoinHostPort(prof.PrimaryIP, fmt.Sprint(port))
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return fmt.Sprintf(
			"⚠️  TCP reach check failed for %s:%d — service may not be listening yet, or firewall blocks it.\n"+
				"    Source: %s | Interface: %s",
			prof.PrimaryIP, port, prof.ResolutionSource, prof.SelectedInterface)
	}
	_ = conn.Close()
	return ""
}
