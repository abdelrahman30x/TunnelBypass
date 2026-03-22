package portable

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/svcman"
)

func pidServiceName(transport string) string {
	return "portable-" + transport
}

func RunNamed(ctx context.Context, log *slog.Logger, name string, o Options) error {
	t, err := lookup(name)
	if err != nil {
		return err
	}
	if log == nil {
		log = slog.Default()
	}
	base := installer.GetBaseDir()
	svc := pidServiceName(t.Name())
	_ = svcman.RemovePIDFile(base, svc)
	if err := svcman.WritePID(base, svc, os.Getpid()); err != nil {
		log.Warn("portable: pid file", "err", err)
	}
	defer func() { _ = svcman.RemovePIDFile(base, svc) }()

	deps, depErr := OrderedDependencies(name)
	if depErr != nil {
		return depErr
	}
	if len(deps) > 0 {
		prevUDPGW := o.UDPGWPort
		normalizePortsForOrchestration(deps, &o)
		if prevUDPGW > 0 && o.UDPGWPort != prevUDPGW {
			log.Info("internal port auto-assigned", "component", "udpgw", "requested", prevUDPGW, "assigned", o.UDPGWPort)
		}
		if prevUDPGW <= 0 && o.UDPGWPort > 0 {
			log.Info("internal port assigned", "component", "udpgw", "assigned", o.UDPGWPort)
		}
	}
	WriteTransportStart(t.Name(), os.Getpid(), nil, deps)

	probeCtx, probeCancel := context.WithCancel(ctx)
	defer probeCancel()
	startRegistryProbeLoop(probeCtx, t.Name(), o)
	defer ClearTransport(t.Name())

	var runErr error
	if len(deps) > 0 {
		runErr = RunOrchestrated(ctx, log, strings.ToLower(strings.TrimSpace(name)), o)
	} else {
		runErr = t.Run(ctx, log, o)
	}

	if runErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s: %w", t.Name(), runErr)
	}
	return nil
}
