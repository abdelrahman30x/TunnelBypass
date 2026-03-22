package portable

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	tbssh "tunnelbypass/core/ssh"
	"tunnelbypass/internal/tblog"
)

func probeTCPWithRetry(ctx context.Context, addr string, log *slog.Logger, label string) error {
	timeout := time.Duration(tblog.IntFromEnv("TB_PROBE_TIMEOUT_MS", 3000)) * time.Millisecond
	retries := tblog.IntFromEnv("TB_PROBE_RETRIES", 5)
	if retries < 1 {
		retries = 1
	}
	var lastErr error
	for attempt := 0; attempt < retries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pctx, cancel := context.WithTimeout(ctx, timeout)
		d := net.Dialer{}
		c, err := d.DialContext(pctx, "tcp", addr)
		cancel()
		if err == nil {
			_ = c.Close()
			if log != nil {
				log.Debug("probe: tcp ok", "addr", addr, "label", label, "attempt", attempt+1)
			}
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	if log != nil {
		log.Warn("probe: tcp failed", "addr", addr, "label", label, "err", lastErr)
	}
	return fmt.Errorf("tcp probe %s after %d attempts: %w", addr, retries, lastErr)
}

// Best-effort local TCP probe for named transport.
func runProbeForTransport(name string, o Options) (ok bool, errStr string) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "udpgw":
		p := o.UDPGWPort
		if p <= 0 {
			p = 7300
		}
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		if err := probeTCPWithRetry(context.Background(), addr, nil, "udpgw"); err != nil {
			return false, err.Error()
		}
		return true, ""
	case "ssh":
		p := o.SSHPort
		if p <= 0 {
			p = tbssh.ListenPreference()
		}
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		if err := probeTCPWithRetry(context.Background(), addr, nil, "ssh"); err != nil {
			return false, err.Error()
		}
		up := o.UDPGWPort
		if up <= 0 {
			up = 7300
		}
		uaddr := fmt.Sprintf("127.0.0.1:%d", up)
		if err := probeTCPWithRetry(context.Background(), uaddr, nil, "udpgw"); err != nil {
			return false, "udpgw: " + err.Error()
		}
		return true, ""
	case "wss":
		p := o.WssPort
		if p <= 0 {
			p = 443
		}
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		if err := probeTCPWithRetry(context.Background(), addr, nil, "wss"); err != nil {
			return false, err.Error()
		}
		return true, ""
	default:
		return true, ""
	}
}

func probeInterval() time.Duration {
	ms := tblog.IntFromEnv("TB_PROBE_INTERVAL_MS", 15000)
	if ms < 3000 {
		ms = 3000
	}
	return time.Duration(ms) * time.Millisecond
}

// startRegistryProbeLoop updates registry.json with periodic probe results until ctx is done.
func startRegistryProbeLoop(ctx context.Context, transport string, o Options) {
	if strings.TrimSpace(os.Getenv("TB_DISABLE_PROBES")) == "1" {
		return
	}
	transport = strings.ToLower(strings.TrimSpace(transport))
	go func() {
		tick := time.NewTicker(probeInterval())
		defer tick.Stop()
		for {
			ok, errStr := runProbeForTransport(transport, o)
			RecordProbe(transport, ok, errStr)
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func normalizePortsForOrchestration(deps []string, o *Options) {
	if o == nil || !containsString(deps, "udpgw") {
		return
	}
	p := o.UDPGWPort
	if p <= 0 {
		p = 7300
	}
	o.UDPGWPort = installer.EnsureFreeTCPPort(p, "UDPGW")
}

// ParseInitialBackoffDurations parses TB_SVC_INITIAL_BACKOFF_MS like "1000,2000,5000".
func ParseInitialBackoffDurations() []time.Duration {
	s := strings.TrimSpace(os.Getenv("TB_SVC_INITIAL_BACKOFF_MS"))
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []time.Duration
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			continue
		}
		out = append(out, time.Duration(n)*time.Millisecond)
	}
	return out
}
