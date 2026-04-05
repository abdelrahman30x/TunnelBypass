package installer

import (
	"fmt"
	"os"
	"runtime"

	"tunnelbypass/core/types"
	"tunnelbypass/internal/netprobe"
)

const (
	// AutopilotJitterStdDevHighMs triggers optimize when TCP jitter sample std-dev to 8.8.8.8:443 is at or above this (ms).
	AutopilotJitterStdDevHighMs = 15.0
	// AutopilotPingMedianHighMs triggers optimize when median TCP dial time to 1.1.1.1:443 exceeds this (ms).
	AutopilotPingMedianHighMs = 200
)

// LinuxAutopilotProbes supplies network measurements for EvaluateLinuxAutopilotWith (tests inject mocks).
type LinuxAutopilotProbes struct {
	JitterStdDevMs func() (float64, error)
	MedianTCPMS    func() (int, error)
}

// DefaultLinuxAutopilotProbes uses live jitter + TCP median probes.
func DefaultLinuxAutopilotProbes() LinuxAutopilotProbes {
	return LinuxAutopilotProbes{
		JitterStdDevMs: LinuxProbeJitterStdDevMs,
		MedianTCPMS: func() (int, error) {
			return netprobe.MedianTCPDialMS(netprobe.DefaultCloudflareTCPAddr, netprobe.DefaultSamples)
		},
	}
}

// EvaluateLinuxAutopilot sets opt.LinuxOptimizeNet when jitter or median TCP latency to Cloudflare
// exceeds thresholds. It is a no-op unless running as root on Linux, or when NoAutoOptimize / env disables it,
// or when optimize is already requested. Call from engine before service install so opt reaches ApplyLinuxTransitNetworking.
func EvaluateLinuxAutopilot(opt *types.ConfigOptions) {
	if opt == nil {
		return
	}
	EvaluateLinuxAutopilotWith(opt, DefaultLinuxAutopilotProbes())
}

// linuxAutopilotShouldEnable implements threshold logic (used by tests without OS/root).
func linuxAutopilotShouldEnable(o linuxTransitOpts, p LinuxAutopilotProbes) bool {
	if o.NoAutoOptimize || o.OptimizeNet {
		return false
	}
	if p.JitterStdDevMs == nil || p.MedianTCPMS == nil {
		return false
	}
	badJitter := false
	if sd, err := p.JitterStdDevMs(); err == nil && sd >= AutopilotJitterStdDevHighMs {
		badJitter = true
	}
	badPing := false
	if ms, err := p.MedianTCPMS(); err == nil && ms > AutopilotPingMedianHighMs {
		badPing = true
	}
	return badJitter || badPing
}

// EvaluateLinuxAutopilotWith is like EvaluateLinuxAutopilot but uses p for tests.
func EvaluateLinuxAutopilotWith(opt *types.ConfigOptions, p LinuxAutopilotProbes) {
	if opt == nil {
		return
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return
	}
	o := mergeLinuxTransitOpts(*opt)
	if !linuxAutopilotShouldEnable(o, p) {
		return
	}
	opt.LinuxOptimizeNet = true
	fmt.Fprintf(os.Stderr, "[*] Network conditions suggest enabling optimizations; applying adaptive stack (sysctl/BBR) for this install.\n")
}
