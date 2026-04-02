// Package health: local status snapshot (no outbound probes).
package health

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/portable"
	"tunnelbypass/core/svcman"
	"tunnelbypass/internal/utils"
)

func Report(w io.Writer) {
	base := installer.GetBaseDir()
	_, _ = fmt.Fprintf(w, "data_dir: %s\n", base)
	_, _ = fmt.Fprintf(w, "ssh_port_22_listening: %v\n", installer.PortListening(22))
	embedPort := installer.GetSSHBackendPort()
	if installer.SSHEmbedActive() {
		_, _ = fmt.Fprintf(w, "ssh_embed_active: true backend_port: %d\n", embedPort)
	} else {
		_, _ = fmt.Fprintf(w, "ssh_embed_active: false (backend_port field is system SSH target: %d)\n", embedPort)
	}
	_, _ = fmt.Fprintf(w, "udpgw_port_7300_listening: %v\n", installer.PortListening(7300))

	names := []string{
		"TunnelBypass-UDPGW",
		"TunnelBypass-WSS",
		"TunnelBypass-VLESS-WS",
		"TunnelBypass-VLESS-GRPC",
		"TunnelBypass-SSL",
	}
	for _, n := range names {
		pid := svcman.TryReadPIDFile(base, n)
		if pid == 0 {
			_, _ = fmt.Fprintf(w, "user_supervisor[%s]: not_running (no pid file)\n", n)
			continue
		}
		ok := svcman.IsPIDAlive(pid)
		_, _ = fmt.Fprintf(w, "user_supervisor[%s]: pid=%d alive=%v\n", n, pid, ok)
	}

	runDir := filepath.Join(base, "run")
	if ents, err := os.ReadDir(runDir); err == nil && len(ents) > 0 {
		_, _ = fmt.Fprintln(w, "\nrun/ (portable & pid files):")
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			full := filepath.Join(runDir, n)
			switch {
			case strings.HasSuffix(n, ".pid"):
				id := strings.TrimSuffix(n, ".pid")
				pid := svcman.TryReadPIDFile(base, id)
				alive := pid > 0 && svcman.IsPIDAlive(pid)
				_, _ = fmt.Fprintf(w, "  %s: pid=%d alive=%v\n", n, pid, alive)
			case strings.HasSuffix(n, ".meta"):
				b, err := os.ReadFile(full)
				if err != nil {
					_, _ = fmt.Fprintf(w, "  %s: read_error: %v\n", n, err)
					continue
				}
				b = utils.StripUTF8BOM(b)
				var m struct {
					Transport string         `json:"transport"`
					Ports     map[string]int `json:"ports"`
					Extra     map[string]any `json:"extra"`
					Updated   int64          `json:"updated_unix"`
				}
				if err := json.Unmarshal(b, &m); err != nil {
					_, _ = fmt.Fprintf(w, "  %s: json_error: %v\n", n, err)
					continue
				}
				_, _ = fmt.Fprintf(w, "  %s: transport=%q ports=%v updated_unix=%d\n", n, m.Transport, m.Ports, m.Updated)
			}
		}
	}
	printPortableRegistry(w)

	_, _ = fmt.Fprintln(w, "\nhints:")
	_, _ = fmt.Fprintln(w, "  - If native services are used, check OS service manager instead of pid files.")
	_, _ = fmt.Fprintln(w, "  - For crash loops see logs/TunnelBypass-Service.log (supervisor stops after repeated short crashes).")
}

func printPortableRegistry(w io.Writer) {
	rf, err := portable.ReadRegistry()
	if err != nil || len(rf.Transports) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "\nportable registry (run/registry.json) updated_unix=%d\n", rf.UpdatedUnix)
	_, _ = fmt.Fprintf(w, "%-10s %6s %6s %-8s %-18s %8s %s\n", "transport", "pid", "alive", "probe_ok", "last_probe", "crashes", "ports")
	for name, e := range rf.Transports {
		pid := e.PID
		alive := pid > 0 && svcman.IsPIDAlive(pid)
		probe := "?"
		if e.LastProbeOK != nil {
			probe = strconv.FormatBool(*e.LastProbeOK)
		}
		lp := "-"
		if e.LastProbeUnix > 0 {
			lp = time.Unix(e.LastProbeUnix, 0).Format(time.RFC3339)
		}
		ports := "-"
		if len(e.Ports) > 0 {
			ports = fmt.Sprintf("%v", e.Ports)
		}
		_, _ = fmt.Fprintf(w, "%-10s %6d %6v %-8s %-18s %8d %s\n", name, pid, alive, probe, lp, e.CrashCount, ports)
		if e.LastProbeError != "" {
			_, _ = fmt.Fprintf(w, "  └─ probe_err: %s\n", e.LastProbeError)
		}
	}
}

func Summary() string {
	var b strings.Builder
	Report(&b)
	return b.String()
}
