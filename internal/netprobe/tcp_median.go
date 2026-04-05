// Package netprobe provides small outbound TCP timing probes shared by health status
// and Linux autopilot without tying core/installer to internal/health.
package netprobe

import (
	"net"
	"sort"
	"time"
)

const (
	// DefaultCloudflareTCPAddr is used for median TCP dial latency (same host as Cloudflare DNS).
	DefaultCloudflareTCPAddr = "1.1.1.1:443"
	DefaultSamples           = 5
	DefaultTCPTimeout        = 3 * time.Second
	DefaultBetweenSamples    = 20 * time.Millisecond
)

// MedianTCPDialMS dials addr TCP n times and returns the median round-trip time in milliseconds.
func MedianTCPDialMS(addr string, n int) (int, error) {
	if n <= 0 {
		n = DefaultSamples
	}
	d := net.Dialer{Timeout: DefaultTCPTimeout}
	var samples []time.Duration
	for i := 0; i < n; i++ {
		t0 := time.Now()
		c, err := d.Dial("tcp", addr)
		if err != nil {
			return 0, err
		}
		_ = c.Close()
		samples = append(samples, time.Since(t0))
		if i+1 < n {
			time.Sleep(DefaultBetweenSamples)
		}
	}
	return int(medianDur(samples).Milliseconds()), nil
}

func medianDur(xs []time.Duration) time.Duration {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), xs...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}
