package installer

import (
	"fmt"
	"math"
	"net"
	"time"
)

const (
	jitterSamples     = 10
	jitterDialTimeout = 3 * time.Second
	jitterBetweenSample = 40 * time.Millisecond
)

// LinuxProbeJitterStdDevMs performs TCP connect probes to 8.8.8.8:443 and returns sample std dev in ms.
// Returns err if too few samples succeeded.
func LinuxProbeJitterStdDevMs() (float64, error) {
	var ms []float64
	for i := 0; i < jitterSamples; i++ {
		t0 := time.Now()
		c, err := net.DialTimeout("tcp", "8.8.8.8:443", jitterDialTimeout)
		if err != nil {
			time.Sleep(jitterBetweenSample)
			continue
		}
		_ = c.Close()
		ms = append(ms, float64(time.Since(t0).Milliseconds()))
		time.Sleep(jitterBetweenSample)
	}
	if len(ms) < 4 {
		return 0, fmt.Errorf("insufficient jitter samples")
	}
	return stdDevFloat(ms), nil
}

func stdDevFloat(xs []float64) float64 {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var acc float64
	for _, x := range xs {
		d := x - mean
		acc += d * d
	}
	return math.Sqrt(acc / float64(len(xs)))
}
