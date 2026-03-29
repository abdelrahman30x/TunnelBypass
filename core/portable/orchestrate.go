package portable

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tbssh "tunnelbypass/core/ssh"
)

func isBindConflict(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "address already in use") ||
		strings.Contains(s, "only one usage of each socket address")
}

func depPortHint(depName string, o Options) int {
	switch depName {
	case "udpgw":
		if o.UDPGWPort > 0 {
			return o.UDPGWPort
		}
		return 7300
	case "ssh":
		p := o.SSHPort
		if p <= 0 {
			p = tbssh.ListenPreference()
		}
		p = tbssh.SanitizeEmbeddedListenPort(p)
		if p <= 0 {
			return 0
		}
		return p
	default:
		return 0
	}
}

func annotateDependencyError(depName string, o Options, err error) error {
	if !isBindConflict(err) {
		return err
	}
	p := depPortHint(depName, o)
	if p <= 0 {
		return err
	}
	return fmt.Errorf("%w\n[!] %s port %d is already in use\n[+] Try: tunnelbypass status", err, strings.ToUpper(depName), p)
}

func DepStartTimeout() time.Duration {
	ms := 120000
	if ms < 1000 {
		ms = 1000
	}
	return time.Duration(ms) * time.Millisecond
}

func depHasUdpgw(deps []string) bool {
	for _, d := range deps {
		if strings.ToLower(strings.TrimSpace(d)) == "udpgw" {
			return true
		}
	}
	return false
}

func sshDepOptions(deps []string, o Options) Options {
	o2 := o
	if depHasUdpgw(deps) {
		o2.ExternalUDPGW = true
	}
	return o2
}

// RunOrchestrated starts deps in order, then target; cancels everything on failure or probe timeout.
func RunOrchestrated(ctx context.Context, log *slog.Logger, target string, o Options) error {
	if log == nil {
		log = slog.Default()
	}
	deps, err := OrderedDependencies(target)
	if err != nil {
		return err
	}
	if len(deps) == 0 {
		return runTransport(ctx, log, target, o)
	}

	var depCancels []context.CancelFunc
	cancelAll := func() {
		for i := len(depCancels) - 1; i >= 0; i-- {
			depCancels[i]()
		}
		depCancels = nil
	}
	defer cancelAll()

	for _, depName := range deps {
		depName = strings.ToLower(strings.TrimSpace(depName))

		// If the dependency is already satisfied (e.g. UDPGW service already running),
		// skip starting it to avoid a bind conflict on Windows.
		if waitOneDependency(context.Background(), depName, o, log) == nil {
			if log != nil {
				log.Info("dependency already running, skipping start", "dep", depName)
			}
			continue
		}

		dctx, cancel := context.WithCancel(ctx)
		depCancels = append(depCancels, cancel)

		oDep := o
		if depName == "ssh" {
			oDep = sshDepOptions(deps, o)
		}

		errCh := make(chan error, 1)
		go func(name string) {
			dlog := log.With("transport", name, "role", "dependency")
			errCh <- runTransport(dctx, dlog, name, oDep)
		}(depName)

		waitCtx, waitCancel := context.WithTimeout(ctx, DepStartTimeout())
		waitErr := waitDependencyReadyOrError(waitCtx, depName, o, log, errCh)
		waitCancel()

		if waitErr != nil {
			cancelAll()
			drainErrCh(errCh)
			return waitErr
		}
	}

	oRun := o
	if target == "ssh" {
		oRun.ExternalUDPGW = true
	}

	runErr := runTransport(ctx, log.With("transport", target), target, oRun)
	cancelAll()
	return runErr
}

func drainErrCh(ch <-chan error) {
	select {
	case <-ch:
	default:
	}
}

func waitDependencyReadyOrError(ctx context.Context, depName string, o Options, log *slog.Logger, errCh <-chan error) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-errCh:
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if log != nil {
				log.Error("dependency exited before ready", "dep", depName, "err", err)
			}
			return fmt.Errorf("dependency %s: %w", depName, annotateDependencyError(depName, o, err))
		case <-ctx.Done():
			if log != nil {
				log.Error("dependency start timeout or cancelled", "dep", depName, "err", ctx.Err(),
					"timeout_ms", DepStartTimeout().Milliseconds())
			}
			return fmt.Errorf("dependency %s not ready: %w", depName, ctx.Err())
		case <-ticker.C:
			if err := waitOneDependency(context.Background(), depName, o, log); err == nil {
				return nil
			}
		}
	}
}

func runTransport(ctx context.Context, log *slog.Logger, name string, o Options) error {
	t, err := lookup(name)
	if err != nil {
		return err
	}
	return t.Run(ctx, log, o)
}

func waitOneDependency(ctx context.Context, dep string, o Options, log *slog.Logger) error {
	switch dep {
	case "udpgw":
		p := o.UDPGWPort
		if p <= 0 {
			p = 7300
		}
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		if err := probeTCPWithRetry(ctx, addr, log, "udpgw"); err != nil {
			return fmt.Errorf("udpgw not ready on port %d: %w", p, err)
		}
		return nil
	case "ssh":
		addr, err := sshDependencyListenAddr(ctx, o)
		if err != nil {
			return err
		}
		if err := probeTCPWithRetry(ctx, addr, log, "ssh"); err != nil {
			return fmt.Errorf("ssh not ready on %s: %w", addr, err)
		}
		up := o.UDPGWPort
		if up <= 0 {
			up = 7300
		}
		uaddr := fmt.Sprintf("127.0.0.1:%d", up)
		if err := probeTCPWithRetry(ctx, uaddr, log, "udpgw"); err != nil {
			return fmt.Errorf("udpgw for ssh not ready: %w", err)
		}
		return nil
	default:
		return nil
	}
}
