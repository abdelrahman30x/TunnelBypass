package destprobe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"tunnelbypass/tools/host_catalog"
)

const DefaultTimeout = 5 * time.Second

type Result struct {
	Host       string
	Address    string
	TCP        bool
	TLS        bool
	HTTPS      bool
	StatusCode int
}

func ProbeHost(ctx context.Context, rawHost string) (Result, error) {
	host := host_catalog.NormalizeHost(rawHost)
	if host == "" {
		return Result{}, fmt.Errorf("invalid hostname")
	}
	addr := host_catalog.RealityDestAddressForHost(host)
	res := Result{Host: host, Address: addr}

	if err := probeTCP(ctx, addr); err != nil {
		return res, fmt.Errorf("tcp ping %s: %w", addr, err)
	}
	res.TCP = true

	if err := probeTLS(ctx, addr, host); err != nil {
		return res, fmt.Errorf("tls certificate %s: %w", host, err)
	}
	res.TLS = true

	status, err := probeHTTPS(ctx, addr, host)
	if err != nil {
		return res, fmt.Errorf("https response %s: %w", host, err)
	}
	res.HTTPS = true
	res.StatusCode = status
	return res, nil
}

func ProbeHostWithTimeout(rawHost string, timeout time.Duration) (Result, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ProbeHost(ctx, rawHost)
}

func probeTCP(ctx context.Context, addr string) error {
	d := &net.Dialer{Timeout: DefaultTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

func probeTLS(ctx context.Context, addr, serverName string) error {
	d := &net.Dialer{Timeout: DefaultTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return err
	}
	return nil
}

func probeHTTPS(ctx context.Context, addr, host string) (int, error) {
	client := httpClientForAddr(addr, host)
	status, err := doHTTPS(ctx, client, http.MethodHead, host)
	if err == nil {
		return status, nil
	}
	// Some origins reject HEAD but still prove there is a real HTTPS server.
	return doHTTPS(ctx, client, http.MethodGet, host)
}

func httpClientForAddr(addr, serverName string) *http.Client {
	d := &net.Dialer{Timeout: DefaultTimeout}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp", addr)
		},
		TLSClientConfig: &tls.Config{
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2: true,
	}
	return &http.Client{Transport: tr, Timeout: DefaultTimeout}
}

func doHTTPS(ctx context.Context, client *http.Client, method, host string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://"+host+"/", nil)
	if err != nil {
		return 0, err
	}
	req.Host = host
	req.Header.Set("User-Agent", "TunnelBypass-dest-probe/1")
	resp, err := client.Do(req)
	if err != nil {
		if strings.EqualFold(method, http.MethodHead) {
			return 0, err
		}
		return 0, err
	}
	defer resp.Body.Close()
	if method == http.MethodGet {
		_, _ = io.CopyN(io.Discard, resp.Body, 1024)
	}
	if resp.StatusCode < 100 || resp.StatusCode > 599 {
		return 0, fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}
