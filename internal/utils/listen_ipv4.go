package utils

import (
	"net"
	"strconv"
	"strings"
)

// ListenAddrNeedsIPv4WildcardFix reports whether a server listen address should be
// normalized to 0.0.0.0 for IPv4 parity on Linux (e.g. Debian/Kali when
// net.ipv6.bindv6only=1 isolates an IPv6 wildcard bind from IPv4 clients).
//
// True for: empty, "::", "[::]", IPv6 wildcard with port, and ":port" (host omitted;
// ambiguous and often binds IPv6-only on some distributions).
func ListenAddrNeedsIPv4WildcardFix(listen string) bool {
	listen = strings.TrimSpace(listen)
	if strings.HasPrefix(listen, ":") && !strings.Contains(listen, "[") {
		rest := strings.TrimPrefix(listen, ":")
		if rest == "" {
			return true
		}
		if _, err := strconv.Atoi(rest); err == nil {
			return true
		}
	}
	switch listen {
	case "", "::", "[::]", "::0":
		return true
	default:
	}
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "::", "::0":
		return true
	default:
		return false
	}
}

// ListenPortFromField extracts a TCP/UDP port from a listen field like ":443",
// "0.0.0.0:443", or "[::]:443". ok is false if no port can be parsed.
func ListenPortFromField(listen string) (port int, ok bool) {
	listen = strings.TrimSpace(listen)
	if strings.HasPrefix(listen, ":") && !strings.Contains(listen, "[") {
		rest := strings.TrimPrefix(listen, ":")
		if p, err := strconv.Atoi(rest); err == nil && p > 0 {
			return p, true
		}
		return 0, false
	}
	_, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return 0, false
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 {
		return 0, false
	}
	return p, true
}
