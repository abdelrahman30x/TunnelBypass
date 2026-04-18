package network

import (
	"fmt"
	"log/slog"
	"net"
	"strings"

	"tunnelbypass/internal/debug"
)

// ProfileOptions configures a single ResolveProfile call.
type ProfileOptions struct {
	Mode        HostMode
	CLIServer   string
	EnvOverride string
	Security    SecurityProfile
	Logger      *slog.Logger
	// Trace enables verbose [network] debug steps (also requires Logger and debug.Enabled() at call site).
	Trace bool
}

// ResolveProfile resolves NetworkProfile with a logged priority chain.
func ResolveProfile(opts ProfileOptions) (NetworkProfile, error) {
	prof := NetworkProfile{Mode: opts.Mode, Security: opts.Security}
	trace := opts.Trace && debug.Enabled() && opts.Logger != nil
	log := opts.Logger

	if trace {
		sec := "StrictLAN"
		if opts.Security == RelaxedLAN {
			sec = "RelaxedLAN"
		}
		mode := "Internet"
		if opts.Mode == LANMode {
			mode = "LAN"
		}
		log.Debug("[network] mode=" + mode + " security=" + sec)
	}

	// --- Priority 1: explicit --server / spec address (not "auto") ---
	if ip := strings.TrimSpace(opts.CLIServer); ip != "" && !strings.EqualFold(ip, "auto") {
		if log != nil {
			log.Info("[network] server override from --server flag", "ip", ip)
		}
		if trace {
			log.Debug("[network] priority-1 --server flag: set")
		}
		prof.PrimaryIP = ip
		prof.ResolutionSource = "cli-flag"
		return prof, nil
	}
	if trace {
		if log != nil {
			log.Debug("[network] priority-1 --server flag: not set")
		}
	}

	if opts.Mode == InternetMode {
		if trace && log != nil {
			log.Debug("[network] InternetMode: public IP resolved later by provision layer")
		}
		prof.ResolutionSource = "public-ip-provider"
		return prof, nil
	}

	// --- LAN mode below ---

	if trace && log != nil {
		log.Debug("[network] priority-2 TUNNELBYPASS_LAN_IP: checking env")
	}
	if ip := strings.TrimSpace(opts.EnvOverride); ip != "" {
		if net.ParseIP(ip) == nil {
			return prof, fmt.Errorf("TUNNELBYPASS_LAN_IP=%q is not a valid IP", ip)
		}
		if log != nil {
			log.Info("[network] server from env TUNNELBYPASS_LAN_IP", "ip", ip)
		}
		if trace && log != nil {
			log.Debug("[network] priority-2 TUNNELBYPASS_LAN_IP: set")
		}
		prof.PrimaryIP = ip
		prof.ResolutionSource = "env-var"
		return prof, nil
	}
	if trace && log != nil {
		log.Debug("[network] priority-2 TUNNELBYPASS_LAN_IP: not set")
	}

	if trace && log != nil {
		log.Debug("[network] step-3 trying default gateway interface...")
	}
	dip, diface, dsource, derr := detectLANIP(log, trace)
	if derr != nil && log != nil {
		log.Warn("[network] LAN IP detection failed, falling back", "err", derr)
	}
	if dip != "" {
		prof.PrimaryIP = dip
		prof.SelectedInterface = diface
		prof.ResolutionSource = dsource
		if trace && log != nil {
			log.Debug("[network] final PrimaryIP="+prof.PrimaryIP+" source="+prof.ResolutionSource)
		}
		return prof, nil
	}

	if log != nil {
		log.Warn("[network] no LAN interface found, using 127.0.0.1 (single-machine)")
	}
	prof.PrimaryIP = "127.0.0.1"
	prof.ResolutionSource = "loopback-fallback"
	if trace && log != nil {
		log.Debug("[network] final PrimaryIP=127.0.0.1 source=loopback-fallback")
	}
	return prof, nil
}
