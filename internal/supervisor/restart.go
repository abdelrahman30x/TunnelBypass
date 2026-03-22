// Package supervisor: restart/backoff for long-running child processes (CLI scope).
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

// ExitKind classifies why a supervised process stopped.
type ExitKind int

const (
	ExitUnknown ExitKind = iota
	ExitClean            // exit code 0
	ExitCrash            // non-zero exit or signal
	ExitManual           // operator cancelled / service Stop
)

// Policy configures backoff and crash-loop limits.
type Policy struct {
	InitialBackoff time.Duration // default 2s
	MaxBackoff     time.Duration // default 2m
	// MaxCrashLoops stops restart after this many consecutive "short" crashes (0 = unlimited).
	MaxCrashLoops int // default from env TB_SVC_MAX_CRASH_LOOPS or 12
	// Runs shorter than this duration count as crash-loop candidates (default 8s).
	ShortRun time.Duration
	// InitialDelays, if non-empty, are used as sleep durations before the 1st, 2nd, ... restart (before exponential backoff).
	InitialDelays []time.Duration
	// OnScheduledRestart is invoked after a failed attempt, just before sleeping (restartCount is 1-based after first failure).
	OnScheduledRestart func(restartCount uint64)
}

// Default restart/backoff policy; TB_SVC_MAX_CRASH_LOOPS overrides crash cap.
func DefaultPolicy() Policy {
	p := Policy{
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     2 * time.Minute,
		MaxCrashLoops:  12,
		ShortRun:       8 * time.Second,
	}
	if v := os.Getenv("TB_SVC_MAX_CRASH_LOOPS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			if n == 0 {
				p.MaxCrashLoops = 0 // unlimited
			} else if n > 0 {
				p.MaxCrashLoops = n
			}
		}
	}
	return p
}

// Exponential backoff delay before the next restart, capped by policy.
func NextBackoff(prev time.Duration, p Policy) time.Duration {
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = 2 * time.Second
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = 2 * time.Minute
	}
	if prev <= 0 {
		return p.InitialBackoff
	}
	next := time.Duration(float64(prev) * 1.6)
	if next > p.MaxBackoff {
		return p.MaxBackoff
	}
	return next
}

// ClassifyExit maps wait errors to ExitKind (best-effort).
func ClassifyExit(err error) ExitKind {
	if err == nil {
		return ExitClean
	}
	if errors.Is(err, context.Canceled) {
		return ExitManual
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ExitCrash
	}
	return ExitCrash
}

// Metrics holds optional counters (lightweight observability).
type Metrics struct {
	Restarts       atomic.Uint64
	CrashLoops     atomic.Uint64
	LastExitKind   atomic.Uint32 // stores ExitKind
	ShortRunStreak atomic.Uint32
}

func (m *Metrics) SetLastKind(k ExitKind) {
	m.LastExitKind.Store(uint32(k))
}

// Runs fn in a restart loop until ctx done, policy exhausted, or fn stops.
// Each attempt gets a context cancelled before backoff sleep.
func Loop(ctx context.Context, log *slog.Logger, p Policy, m *Metrics, fn func(attemptCtx context.Context) error) {
	if log == nil {
		log = slog.Default()
	}
	if m == nil {
		m = &Metrics{}
	}
	if p.ShortRun <= 0 {
		p.ShortRun = 8 * time.Second
	}

	backoff := time.Duration(0)
	var shortStreak uint32

	for {
		if ctx.Err() != nil {
			return
		}

		attemptCtx, cancel := context.WithCancel(ctx)
		start := time.Now()
		err := fn(attemptCtx)
		cancel()

		dur := time.Since(start)
		kind := ClassifyExit(err)

		if ctx.Err() != nil {
			m.SetLastKind(ExitManual)
			log.Info("supervisor: stopping", "reason", "context_cancelled")
			return
		}
		if kind == ExitManual {
			m.SetLastKind(ExitManual)
			log.Info("supervisor: stopping", "reason", "manual_or_cancelled")
			return
		}

		m.SetLastKind(kind)
		switch kind {
		case ExitClean:
			log.Warn("supervisor: child exited cleanly (will restart)", "run_duration", dur)
			shortStreak = 0
			m.ShortRunStreak.Store(0)
			backoff = 0
		case ExitCrash:
			log.Error("supervisor: child crashed or failed", "run_duration", dur, "err", err,
				"hint", "check binary path, config JSON, disk space, and logs under <data>/logs/")
			if dur < p.ShortRun {
				shortStreak++
				m.ShortRunStreak.Store(shortStreak)
				m.CrashLoops.Add(1)
			} else {
				shortStreak = 0
				m.ShortRunStreak.Store(0)
				backoff = 0
			}
			if p.MaxCrashLoops > 0 && shortStreak >= uint32(p.MaxCrashLoops) {
				log.Error("supervisor: crash loop limit reached; stopping restarts",
					"consecutive_short_runs", shortStreak,
					"threshold", p.ShortRun,
					"limit", p.MaxCrashLoops,
					"hint", "fix config or binary, then restart the service or run tunnelbypass xray-svc manually")
				return
			}
		}

		m.Restarts.Add(1)
		restarts := m.Restarts.Load()
		var sleep time.Duration
		idx := int(restarts) - 1
		if idx >= 0 && idx < len(p.InitialDelays) {
			sleep = p.InitialDelays[idx]
		} else {
			backoff = NextBackoff(backoff, p)
			sleep = backoff
		}
		if p.OnScheduledRestart != nil {
			p.OnScheduledRestart(restarts)
		}
		log.Info("supervisor: backing off before restart", "sleep", sleep, "exit_kind", kind.String(), "short_run_streak", shortStreak)

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleep):
		}
	}
}

func (k ExitKind) String() string {
	switch k {
	case ExitClean:
		return "clean"
	case ExitCrash:
		return "crash"
	case ExitManual:
		return "manual"
	default:
		return "unknown"
	}
}
