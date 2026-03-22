package portable

import (
	"context"
	"log/slog"
)

// User-space tunnel runnable by the portable orchestrator.
type Transport interface {
	Name() string
	// Names of transports that must be up before this one (start order).
	Dependencies() []string
	Run(ctx context.Context, log *slog.Logger, o Options) error
}

// Flags for portable `run`.
type Options struct {
	ConfigPath string

	SSHPort   int
	UDPGWPort int
	SSHUser   string
	SSHPass   string

	WssPort       int
	StunnelAccept int

	// Orchestrator: udpgw already running (ssh stack).
	ExternalUDPGW bool
}
