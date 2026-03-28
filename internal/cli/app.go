package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/portable"
	"tunnelbypass/core/portforward"
	"tunnelbypass/core/udpgw"
	"tunnelbypass/internal/debug"
	"tunnelbypass/internal/elevate"
	"tunnelbypass/internal/health"
	"tunnelbypass/internal/tblog"
	"tunnelbypass/internal/terminal"
	"tunnelbypass/tools/host_catalog"
)

var version = "v1.2.1"

// SetVersion sets the user-visible release string before Main (from cmd, or tests).
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func getDefaultConfigPath() string {
	baseDir := installer.GetBaseDir()
	serverConfig := filepath.Join(installer.GetConfigDir("vless"), "server.json")
	if _, err := os.Stat(serverConfig); err == nil {
		return serverConfig
	}
	return filepath.Join(baseDir, "configs", "config.json")
}

var (
	configFlag    = flag.String("config", getDefaultConfigPath(), "Path to config file")
	verReq        = flag.Bool("version", false, "Show version")
	udpgwPort     = flag.Int("udpgw-port", 7300, "UDPGW listen port")
	debugFlag     = flag.Bool("debug", false, "Verbose log output")
	forwarderListen = flag.String("listen", "127.0.0.1:0", "Forwarder listen address (0 = dynamic allocation)")
	forwarderTarget = flag.String("target", "127.0.0.1:0", "Forwarder target address")
)

// Main is the tunnelbypass CLI entry (called from cmd).
func Main() {
	terminal.EnableVTProcessing()
	tblog.Init()
	if v := strings.TrimSpace(os.Getenv("TUNNELBYPASS_DATA_DIR")); v != "" {
		installer.SetDataRootOverride(v)
	}

	if i := subcommandIndex("run"); i >= 0 {
		var ra []string
		if i+1 < len(os.Args) {
			ra = os.Args[i+1:]
		}
		runCommand(ra)
		return
	}
	if i := subcommandIndex("generate"); i >= 0 {
		var ga []string
		if i+1 < len(os.Args) {
			ga = os.Args[i+1:]
		}
		generateCommand(ga)
		return
	}
	if i := subcommandIndex("uninstall"); i >= 0 {
		var ua []string
		if i+1 < len(os.Args) {
			ua = os.Args[i+1:]
		}
		runUninstallCLI(ua)
		return
	}
	if i := subcommandIndex("deps-tree"); i >= 0 {
		tblog.Init()
		var args []string
		if i+1 < len(os.Args) {
			args = os.Args[i+1:]
		}
		runDepsTree(args)
		return
	}
	if subcommandIndex("health") >= 0 || subcommandIndex("status") >= 0 {
		tblog.Init()
		health.Report(os.Stdout)
		return
	}

	shouldElevate := false
	if len(os.Args) > 1 && isPrivilegedCommand(os.Args[1]) {
		shouldElevate = true
	}
	if shouldElevate && !elevate.IsAdmin() {
		fmt.Printf("%s[!] TunnelBypass %s - Administrator privileges required.%s\n", ColorYellow, version, ColorReset)
		err := elevate.Elevate()
		if err != nil {
			log.Fatalf("Failed to elevate: %v", err)
		}
		return
	}

	flag.Usage = printUsage
	flag.Parse()

	debug.Init(*debugFlag)
	tblog.ApplyDebug(debug.Enabled())
	debug.ConfigureLog()
	debug.Logf("version=%s args=%q", version, os.Args)
	debug.Logf("default config path=%s", *configFlag)

	if *verReq {
		fmt.Println("TunnelBypass Version:", version)
		return
	}

	if len(os.Args) < 2 {
		if os.Getenv("TUNNELBYPASS_AUTORUN_SETUP") == "1" && elevate.IsAdmin() {
			_ = os.Unsetenv("TUNNELBYPASS_AUTORUN_SETUP")
			runSetupDirect()
			return
		}
		runWizard()
		return
	}

	command := flag.Arg(0)

	switch command {
	case "wizard":
		runWizard()
	case "hosts":
		runHosts()
	case "xray-svc":
		runXrayService()
	case "hysteria-svc":
		runHysteriaService()
	case "udpgw-svc":
		runUdpgwService()
	case "forwarder":
		runForwarderService()
	default:
		printUsage()
	}
}

func runSetupDirect() {
	reader := bufio.NewReader(os.Stdin)
	runSetupWizard(reader)
}

func runXrayService() {
	s, err := installer.GetXrayService("Xray-Tunnel-Service", *configFlag)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	err = s.Run()
	if err != nil {
		log.Fatalf("Service run failed: %v", err)
	}
}

func runHysteriaService() {
	s, err := installer.GetHysteriaService("Hysteria-Tunnel-Service", *configFlag)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	err = s.Run()
	if err != nil {
		log.Fatalf("Service run failed: %v", err)
	}
}

func runUdpgwService() {
	nc := notifyContext()
	defer nc.cancel()
	if err := udpgw.Run(nc.ctx, udpgw.Options{Port: *udpgwPort, Logger: tblog.Sub("udpgw")}); err != nil && nc.ctx.Err() == nil {
		log.Fatalf("UDPGW service failed: %v", err)
	}
}

func runForwarderService() {
	nc := notifyContext()
	defer nc.cancel()

	listenAddr := *forwarderListen
	targetAddr := *forwarderTarget

	if targetAddr == "127.0.0.1:0" {
		// Try to load from config
		cfg, err := installer.LoadSSHPortConfig()
		if err == nil && cfg.InternalPort > 0 {
			targetAddr = fmt.Sprintf("127.0.0.1:%d", cfg.InternalPort)
		} else {
			log.Fatalf("Forwarder target not specified and could not load from config")
		}
	}

	forwarder := portforward.New(portforward.Config{
		ListenAddr: listenAddr,
		TargetAddr: targetAddr,
		Logger:     tblog.Sub("forwarder"),
	})

	fmt.Printf("Starting port forwarder: %s -> %s\n", listenAddr, targetAddr)
	if err := forwarder.Run(nc.ctx); err != nil && nc.ctx.Err() == nil {
		log.Fatalf("Forwarder service failed: %v", err)
	}
}

type notifyCtx struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func notifyContext() notifyCtx {
	sigs := []os.Signal{os.Interrupt}
	if runtime.GOOS != "windows" {
		sigs = append(sigs, syscall.SIGTERM)
	}
	c, stop := signal.NotifyContext(context.Background(), sigs...)
	return notifyCtx{ctx: c, cancel: stop}
}

func runCommand(args []string) {
	os.Exit(executeRun(args))
}

func runDepsTree(args []string) {
	mermaid := false
	var pos []string
	for _, a := range args {
		if a == "--mermaid" || a == "-mermaid" {
			mermaid = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		pos = append(pos, a)
	}
	root := ""
	if len(pos) > 0 {
		root = pos[0]
	}
	var out string
	var err error
	if mermaid {
		out, err = portable.FormatDepsTreeMermaid(root)
	} else {
		out, err = portable.FormatDepsTreeText(root)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func isPrivilegedCommand(cmd string) bool {
	switch cmd {
	case "wizard", "connect", "uninstall":
		return true
	}
	return false
}

func printUsage() {
	fmt.Printf("%sTunnelBypass %s%s - Unified Protocol Installer & Manager\n", ColorBold, version, ColorReset)
	fmt.Println("Usage:")
	fmt.Println("  tunnelbypass [flags] <command>")
	fmt.Println("\nCommands:")
	fmt.Println("  wizard  - Configure and install tunnel (Reality/VLESS, Hysteria, …) as a service (WireGuard temporarily disabled)")
	fmt.Println("            Note: SSH, WSS, and TLS protocols are temporarily disabled.")
	fmt.Println("  hosts   - View tunnel host catalog")
	fmt.Println("  run     - Run transport (default: system data dir like wizard; add 'portable' or --portable for per-user); see run -help")
	fmt.Println("  generate - Generate configs only (same engine as run)")
	fmt.Println("  deps-tree - Show portable transport dependency graph (--mermaid for flowchart)")
	fmt.Println("  uninstall - Remove an OS service and TunnelBypass configs (--service or --type; --data-dir; --yes)")
	fmt.Println("  status  - Local health snapshot (pid files, common ports)")
	fmt.Println("  health  - Same as status")
	fmt.Println("\nFlags:")
	flag.PrintDefaults()
	fmt.Println("\nUse flags (e.g. --debug, --portable, --data-dir) and config files under the data directory.")
}

func subcommandIndex(name string) int {
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == name {
			return i
		}
	}
	return -1
}

func runHosts() {
	fmt.Printf("\n%s╔══════════════════════════════════════════╗%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("║           %sTUNNEL HOST CATALOG%s            ║\n", ColorBold, ColorReset)
	fmt.Printf("%s╚══════════════════════════════════════════╝%s\n", ColorBold+ColorYellow, ColorReset)
	for i, domain := range host_catalog.DefaultHosts() {
		fmt.Printf("  %s%2d)%s %s\n", ColorCyan, i+1, ColorReset, domain)
	}
	fmt.Println()
}
