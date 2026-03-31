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

// publicIPv4HTTPClient uses IPv4-only outbound TCP so lookup services return the server's IPv4 address.
func publicIPv4HTTPClient() *http.Client {
	var tr *http.Transport
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = t.Clone()
	} else {
		tr = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		return d.DialContext(ctx, "tcp4", addr)
	}
	return &http.Client{Transport: tr}
}

// publicIPv4ProviderURLs are HTTPS endpoints returning the caller's IPv4 as plain text (tcp4 dial).
var publicIPv4ProviderURLs = []string{
	"https://v4.ident.me",
	"https://ipv4.icanhazip.com",
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	"https://ident.me",
}

func dnsLookupHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	_, err := net.DefaultResolver.LookupHost(ctx, "example.com")
	return err == nil
}

// GetPublicIP returns the server's public IPv4 address from HTTPS endpoints (parallel, short timeout).
// If DNS is initially broken, waits 2s (e.g. after resolvectl); retries once after another 2s on total failure.
// IPv6 is intentionally skipped so client URIs and all tunnel protocols stay in host:port form without brackets.
func GetPublicIP() string {
	if !dnsLookupHealthy() {
		time.Sleep(2 * time.Second)
	}
	if ip := fetchPublicIPv4FromProviders(); ip != "" {
		return ip
	}
	time.Sleep(2 * time.Second)
	return fetchPublicIPv4FromProviders()
}

func fetchPublicIPv4FromProviders() string {
	providers := publicIPv4ProviderURLs

	type result struct {
		ip  string
		err error
	}
	resChan := make(chan result, len(providers))

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	client := publicIPv4HTTPClient()

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
			parsed := net.ParseIP(ip)
			if parsed != nil && parsed.To4() != nil {
				resChan <- result{parsed.String(), nil}
			} else {
				resChan <- result{"", fmt.Errorf("not a public IPv4: %s", ip)}
			}
		}(url)
	}

	for i := 0; i < len(providers); i++ {
		res := <-resChan
		if res.err == nil && res.ip != "" {
			cancel()
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
