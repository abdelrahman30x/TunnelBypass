package installer

import (
	"errors"
	"testing"

	"tunnelbypass/core/types"
)

func TestLinuxAutopilotShouldEnable_NoAutoOptimize(t *testing.T) {
	o := linuxTransitOpts{NoAutoOptimize: true}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return 100, nil },
		MedianTCPMS:    func() (int, error) { return 999, nil },
	}
	if linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected false when NoAutoOptimize")
	}
}

func TestLinuxAutopilotShouldEnable_AlreadyOptimize(t *testing.T) {
	o := linuxTransitOpts{OptimizeNet: true}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return 100, nil },
		MedianTCPMS:    func() (int, error) { return 999, nil },
	}
	if linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected false when OptimizeNet already set")
	}
}

func TestLinuxAutopilotShouldEnable_HighJitter(t *testing.T) {
	o := linuxTransitOpts{}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return AutopilotJitterStdDevHighMs, nil },
		MedianTCPMS:    func() (int, error) { return 10, nil },
	}
	if !linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected true when jitter at threshold")
	}
}

func TestLinuxAutopilotShouldEnable_HighPingOnly(t *testing.T) {
	o := linuxTransitOpts{}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return 0, nil },
		MedianTCPMS:    func() (int, error) { return AutopilotPingMedianHighMs + 1, nil },
	}
	if !linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected true when median TCP above threshold")
	}
}

func TestLinuxAutopilotShouldEnable_PingAtThresholdNoTrigger(t *testing.T) {
	o := linuxTransitOpts{}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return 0, nil },
		MedianTCPMS:    func() (int, error) { return AutopilotPingMedianHighMs, nil },
	}
	if linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected false when median exactly at threshold (must exceed)")
	}
}

func TestLinuxAutopilotShouldEnable_ProbeErrors(t *testing.T) {
	o := linuxTransitOpts{}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return 0, errors.New("fail") },
		MedianTCPMS:    func() (int, error) { return 0, errors.New("fail") },
	}
	if linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected false when both probes fail")
	}
}

func TestLinuxAutopilotShouldEnable_JitterErrorButPingBad(t *testing.T) {
	o := linuxTransitOpts{}
	p := LinuxAutopilotProbes{
		JitterStdDevMs: func() (float64, error) { return 0, errors.New("fail") },
		MedianTCPMS:    func() (int, error) { return 500, nil },
	}
	if !linuxAutopilotShouldEnable(o, p) {
		t.Fatal("expected true when median TCP alone exceeds threshold")
	}
}

func TestEvaluateLinuxAutopilotWith_NilOpt(t *testing.T) {
	EvaluateLinuxAutopilotWith(nil, DefaultLinuxAutopilotProbes())
}

func TestEvaluateLinuxAutopilot_NilOpt(t *testing.T) {
	EvaluateLinuxAutopilot(nil)
}

func TestMergeRespectsEnvNoAutoOptimize(t *testing.T) {
	t.Setenv("TUNNELBYPASS_NO_AUTO_OPTIMIZE", "1")
	opt := types.ConfigOptions{}
	o := mergeLinuxTransitOpts(opt)
	if !o.NoAutoOptimize {
		t.Fatal("expected env to force NoAutoOptimize")
	}
}
