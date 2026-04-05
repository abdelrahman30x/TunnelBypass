package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"tunnelbypass/internal/netprobe"
)

const (
	probeTCPAddr     = netprobe.DefaultCloudflareTCPAddr
	probeHTTPURL     = "https://1.1.1.1/cdn-cgi/trace"
	probeSamples     = netprobe.DefaultSamples
	probeHTTPTimeout = 8 * time.Second
)

// PingCheckRequested returns true if argv contains --ping-check or -ping-check.
func PingCheckRequested(argv []string) bool {
	for _, a := range argv {
		if a == "--ping-check" || a == "-ping-check" {
			return true
		}
	}
	return false
}

// WritePingCheck writes outbound latency probes (TCP to Cloudflare, then HTTPS GET trace).
func WritePingCheck(w io.Writer) {
	_, _ = fmt.Fprintln(w, "\n--- Network ping check (1.1.1.1) ---")

	tcpMs, tcpErr := netprobe.MedianTCPDialMS(probeTCPAddr, probeSamples)
	if tcpErr != nil {
		_, _ = fmt.Fprintf(w, "Direct Ping: error (%v)\n", tcpErr)
	} else {
		_, _ = fmt.Fprintf(w, "Direct Ping: %dms (Server to World)\n", tcpMs)
	}

	httpMs, httpErr := httpGetMS(context.Background(), probeHTTPURL, probeHTTPTimeout)
	if httpErr != nil {
		_, _ = fmt.Fprintf(w, "Tunnel Latency: error (%v)\n", httpErr)
	} else {
		_, _ = fmt.Fprintf(w, "Tunnel Latency: %dms (You to World via Tunnel)\n", httpMs)
	}

	st := pingStatus(tcpMs, tcpErr, httpMs, httpErr)
	_, _ = fmt.Fprintf(w, "Status: [%s]\n", st)
}

func httpGetMS(ctx context.Context, url string, timeout time.Duration) (int, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	t0 := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return int(time.Since(t0).Milliseconds()), nil
}

func pingStatus(tcpMs int, tcpErr error, httpMs int, httpErr error) string {
	if tcpErr != nil && httpErr != nil {
		return "FAIL"
	}
	if tcpErr != nil {
		if httpErr == nil && httpMs < 3000 {
			return "DEGRADED"
		}
		return "FAIL"
	}
	if tcpMs >= 2000 {
		return "UNHEALTHY"
	}
	if tcpMs >= 500 || (httpErr == nil && httpMs >= 1500) {
		return "DEGRADED"
	}
	return "HEALTHY"
}
