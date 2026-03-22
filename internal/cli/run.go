package cli

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"tunnelbypass/internal/cfg"
	"tunnelbypass/internal/engine"
)

const exitConfig = 2

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}

func stripPortableToken(args []string) (filtered []string, portable bool) {
	for _, a := range args {
		if strings.EqualFold(strings.TrimSpace(a), "portable") {
			portable = true
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered, portable
}

func executeRun(rawArgs []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage: tunnelbypass run [flags] [portable] <transport>\n")
		fmt.Fprintf(out, "       tunnelbypass run [flags] <transport> [portable]\n\n")
		fmt.Fprintf(out, "Default: system/server data layout (same as wizard: e.g. C:\\TunnelBypass on Windows).\n")
		fmt.Fprintf(out, "Pass --portable or the word portable to use per-user data dir and typical foreground CLI layout.\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
	}

	portableF := fs.Bool("portable", false, "Per-user data directory (LocalAppData / XDG); same as placing the word portable in args")
	systemData := fs.Bool("system-data", false, "Keep system-wide data directory (default for run; explicit no-op unless combined with --portable)")
	dataDir := fs.String("data-dir", "", "Data root for configs, binaries, logs, and run/")
	daemon := fs.Bool("daemon", false, "Restart transport on exit until interrupted")
	configPath := fs.String("config", "", "Path to unified JSON config file")
	udpgwP := fs.Int("udpgw-port", 0, "UDP-over-SSH helper TCP port (0 = default 7300)")
	sshPort := fs.Int("ssh-port", 0, "Embedded SSH listen port (0 = env/default)")
	sshUser := fs.String("ssh-user", "", "SSH user for embed / instructions")
	sshPass := fs.String("ssh-password", "", "SSH password")
	typeFlag := fs.String("type", "", "Transport name (optional if positional or spec sets it)")
	specPath := fs.String("spec", "", "JSON or YAML run spec file")
	sniFlag := fs.String("sni", "", "SNI / tunnel hostname (Reality, Hysteria, WSS, TLS)")
	serverFlag := fs.String("server", "", "Server public address for clients (default: detect public IP)")
	uuidFlag := fs.String("uuid", "", "UUID or auth secret; use 'auto' to generate")
	portFlag := fs.Int("port", 0, "Listen port (transport-specific default if 0)")
	logsDirFlag := fs.String("logs-dir", "", "Log directory (default: <data-dir>/logs; overrides TB_LOGS_DIR)")
	generateOnly := fs.Bool("dry-run", false, "Generate configs only; do not start the tunnel")
	noElevate := fs.Bool("no-elevate", false, "Do not auto-elevate to Administrator/root for OS service install and firewall (user-mode only); override TB_NO_ELEVATE")
	autoStart := fs.Bool("auto-start", true, "Start transport after generation")

	_ = fs.Parse(rawArgs)
	posArgs, portableWord := stripPortableToken(fs.Args())

	var specFile cfg.SpecFile
	if strings.TrimSpace(*specPath) != "" {
		s, err := cfg.LoadSpec(strings.TrimSpace(*specPath))
		if err != nil {
			slog.Error("run: spec file", "path", *specPath, "err", err)
			return exitConfig
		}
		specFile = s
	}

	transport := ""
	if len(posArgs) > 0 {
		transport = strings.TrimSpace(posArgs[0])
	}
	if strings.TrimSpace(*typeFlag) != "" {
		tf := strings.TrimSpace(*typeFlag)
		if transport != "" && !strings.EqualFold(transport, tf) {
			slog.Error("run: --type disagrees with positional transport", "type", tf, "positional", transport)
			return exitConfig
		}
		transport = tf
	}
	if transport == "" && strings.TrimSpace(specFile.Transport) != "" {
		transport = strings.TrimSpace(specFile.Transport)
	}
	if strings.TrimSpace(specFile.Transport) != "" && !strings.EqualFold(strings.TrimSpace(specFile.Transport), transport) {
		slog.Error("run: spec.transport disagrees with selected transport", "spec", specFile.Transport, "transport", transport)
		return exitConfig
	}
	rspec := cfg.RunSpec{}
	if strings.TrimSpace(*configPath) != "" {
		loaded, err := cfg.LoadJSONFile(strings.TrimSpace(*configPath))
		if err != nil {
			slog.Error("run: load config", "path", *configPath, "err", err)
			return exitConfig
		}
		rspec = loaded
	}
	cfg.ApplySpecFileToRunSpec(&rspec, specFile)

	if transport == "" && strings.TrimSpace(rspec.Transport) != "" {
		transport = strings.TrimSpace(rspec.Transport)
	}
	if transport == "" {
		fmt.Fprintln(os.Stderr, "tunnelbypass run: missing transport (e.g. ssh) or use --type/--spec/--config")
		fs.Usage()
		return exitConfig
	}
	rspec.Transport = transport
	if strings.TrimSpace(*sniFlag) != "" {
		rspec.SNI = strings.TrimSpace(*sniFlag)
	}
	if strings.TrimSpace(*serverFlag) != "" {
		rspec.Server.Address = strings.TrimSpace(*serverFlag)
	}
	if strings.TrimSpace(*uuidFlag) != "" {
		rspec.Auth.UUID = strings.TrimSpace(*uuidFlag)
	}
	if *portFlag != 0 {
		rspec.Port = *portFlag
	}
	if *sshPort != 0 {
		rspec.SSH.Port = *sshPort
	}
	if *udpgwP != 0 {
		rspec.UDPGW.Enabled = true
		rspec.UDPGW.Port = *udpgwP
	}
	if strings.TrimSpace(*sshUser) != "" {
		rspec.Auth.SSHUser = strings.TrimSpace(*sshUser)
	}
	if strings.TrimSpace(*sshPass) != "" {
		rspec.Auth.SSHPass = strings.TrimSpace(*sshPass)
	}
	fromFile := strings.TrimSpace(*configPath) != "" || strings.TrimSpace(*specPath) != ""
	if !fromFile {
		rspec.Behavior.Portable = false
		if *systemData {
			rspec.Behavior.Portable = false
		}
	}
	if *portableF || portableWord {
		rspec.Behavior.Portable = true
	}
	if *daemon {
		rspec.Behavior.Daemon = true
	}
	if *generateOnly {
		rspec.Behavior.GenerateOnly = true
	}
	if *noElevate {
		rspec.Behavior.NoElevate = true
	}
	rspec.Behavior.AutoStart = *autoStart
	if strings.TrimSpace(*dataDir) != "" {
		rspec.Paths.DataDir = strings.TrimSpace(*dataDir)
	}
	if strings.TrimSpace(*logsDirFlag) != "" {
		rspec.Paths.LogsDir = strings.TrimSpace(*logsDirFlag)
	}

	nc := notifyContext()
	defer nc.cancel()
	if err := engine.Run(nc.ctx, rspec); err != nil && nc.ctx.Err() == nil {
		slog.Error("run failed", "transport", rspec.Transport, "err", err)
		return 1
	}
	return 0
}
