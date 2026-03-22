package utils

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"
)

// TCP connect probe to host:port.
func CheckPortOpen(host string, port string, timeout time.Duration) bool {
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	if conn != nil {
		defer conn.Close()
		return true
	}
	return false
}

// First public IP from HTTPS endpoints (parallel, short timeout).
func GetPublicIP() string {
	providers := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
		"https://ident.me",
		"https://v4.ident.me",
	}

	type result struct {
		ip  string
		err error
	}
	resChan := make(chan result, len(providers))

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	client := &http.Client{}

	for _, url := range providers {
		go func(u string) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				resChan <- result{"", err}
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				resChan <- result{"", err}
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				resChan <- result{"", err}
				return
			}

			ip := strings.TrimSpace(string(body))
			if net.ParseIP(ip) != nil {
				resChan <- result{ip, nil}
			} else {
				resChan <- result{"", fmt.Errorf("invalid IP: %s", ip)}
			}
		}(url)
	}

	// Wait for the first success or all failures; cancel cancels remaining goroutines.
	for i := 0; i < len(providers); i++ {
		res := <-resChan
		if res.err == nil && res.ip != "" {
			cancel() // abort remaining in-flight requests
			return res.ip
		}
	}

	return ""
}

// IsPortAvailable is true if we can listen on 0.0.0.0:port (tcp or udp).
func IsPortAvailable(network string, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	switch network {
	case "tcp":
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		ln.Close()
		return true
	case "udp":
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false
		}
		pc.Close()
		return true
	default:
		return false
	}
}

const (
	portAllocMin      = 20000
	portAllocMax      = 49151
	portAllocAttempts = 100
)

// AllocatePort uses preferred if free, else a random port in [portAllocMin, portAllocMax]; 0 if none found.
func AllocatePort(network string, preferred int) int {
	if preferred >= 1 && preferred <= 65535 && IsPortAvailable(network, preferred) {
		return preferred
	}
	for i := 0; i < portAllocAttempts; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(portAllocMax-portAllocMin+1)))
		if err != nil {
			continue
		}
		port := portAllocMin + int(n.Int64())
		if IsPortAvailable(network, port) {
			return port
		}
	}
	return 0
}
