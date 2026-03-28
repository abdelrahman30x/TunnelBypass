package portable

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	tbssh "tunnelbypass/core/ssh"
)

func probeTCPWithRetry(ctx context.Context, addr string, log *slog.Logger, label string) error {
	timeout := 3000 * time.Millisecond
	retries := 5
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

// sshDependencyListenAddr resolves 127.0.0.1:port for embedded SSH (same rules as ssh_stack:
// port 22 is avoided when it would conflict with system sshd; dynamic ports are read from run metadata).
func sshDependencyListenAddr(ctx context.Context, o Options) (string, error) {
	p := o.SSHPort
	if p <= 0 {
		p = tbssh.ListenPreference()
	}
	p = tbssh.SanitizeEmbeddedListenPort(p)
	if p > 0 {
		return fmt.Sprintf("127.0.0.1:%d", p), nil
	}
	base := installer.GetBaseDir()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m, err := ReadRunMeta(base, "ssh"); err == nil && m.Ports != nil {
			if pp, ok := m.Ports["ssh"]; ok && pp > 0 {
				return fmt.Sprintf("127.0.0.1:%d", pp), nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("ssh listen port not available (portable-ssh.meta)")
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
		addr, err := sshDependencyListenAddr(context.Background(), o)
		if err != nil {
			return false, err.Error()
		}
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
	ms := 15000
	if ms < 3000 {
		ms = 3000
	}
	return time.Duration(ms) * time.Millisecond
}

// startRegistryProbeLoop updates registry.json with periodic probe results until ctx is done.
func startRegistryProbeLoop(ctx context.Context, transport string, o Options) {
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

// ParseInitialBackoffDurations returns nil (custom backoff lists are not configured via environment).
func ParseInitialBackoffDurations() []time.Duration {
	return nil
}
