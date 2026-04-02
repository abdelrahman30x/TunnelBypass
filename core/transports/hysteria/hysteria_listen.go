package hysteria

import (
	"strconv"
	"strings"
)

// ListenAddr returns the server listen field as "0.0.0.0:port" for IPv4-friendly bind.
func ListenAddr(port int) string {
	if port <= 0 {
		port = 443
	}
	return "0.0.0.0:" + strconv.Itoa(port)
}

// ClientServerAddr is the client "server" value (host:port).
func ClientServerAddr(endpoint string, port int) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "127.0.0.1"
	}
	if port <= 0 {
		port = 443
	}
	return endpoint + ":" + strconv.Itoa(port)
}

// ParseListenFirstPort returns the first UDP port from listen fields like ":443", "0.0.0.0:443", "0.0.0.0:20000-30000".
func ParseListenFirstPort(listen string) int {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return 0
	}
	i := strings.LastIndex(listen, ":")
	if i < 0 {
		return 0
	}
	rest := listen[i+1:]
	if strings.Contains(rest, "-") {
		parts := strings.SplitN(rest, "-", 2)
		p, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || p <= 0 {
			return 0
		}
		return p
	}
	p, err := strconv.Atoi(rest)
	if err != nil || p <= 0 {
		return 0
	}
	return p
}
