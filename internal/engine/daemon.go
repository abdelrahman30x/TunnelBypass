package engine

import (
	"context"

	"tunnelbypass/core/portable"
	"tunnelbypass/internal/supervisor"
	"tunnelbypass/internal/tblog"
)

func RunDaemonLoop(parent context.Context, transport string, fn func(context.Context) error) {
	pol := supervisor.DefaultPolicy()
	pol.InitialDelays = portable.ParseInitialBackoffDurations()
	pol.OnScheduledRestart = func(uint64) {
		portable.RecordSupervisorRestart(transport)
	}
	log := tblog.Sub("run")
	var m supervisor.Metrics
	supervisor.Loop(parent, log, pol, &m, fn)
}
