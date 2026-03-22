package supervisor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNextBackoffCaps(t *testing.T) {
	p := Policy{InitialBackoff: 2 * time.Second, MaxBackoff: 10 * time.Second}
	var b time.Duration
	for i := 0; i < 20; i++ {
		b = NextBackoff(b, p)
		if b > p.MaxBackoff {
			t.Fatalf("iteration %d: backoff %v > max %v", i, b, p.MaxBackoff)
		}
	}
	if b != p.MaxBackoff {
		t.Fatalf("expected cap at max, got %v", b)
	}
}

func TestLoopUsesInitialDelays(t *testing.T) {
	ctx := context.Background()
	var attempts atomic.Int32
	p := Policy{
		InitialBackoff: time.Hour,
		MaxBackoff:     time.Hour,
		MaxCrashLoops:  10,
		ShortRun:       time.Millisecond,
		InitialDelays:  []time.Duration{20 * time.Millisecond, 20 * time.Millisecond},
	}
	var m Metrics
	Loop(ctx, nil, p, &m, func(c context.Context) error {
		n := attempts.Add(1)
		if n < 3 {
			return errors.New("fail")
		}
		return context.Canceled
	})
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}
