package cli

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/portable"
)

func stdinIsTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func runUninstallCLI(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	svc := fs.String("service", "", "OS service name (e.g. TunnelBypass-VLESS)")
	typ := fs.String("type", "", "Transport: reality|vless|vless-ws|hysteria|wireguard|wss|tls (derives service name)")
	dataDir := fs.String("data-dir", "", "Data root (same as run --data-dir)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	pos := fs.Args()
	if *typ == "" && len(pos) > 0 {
		*typ = pos[0]
	}
	serviceName := strings.TrimSpace(*svc)
	if serviceName == "" && strings.TrimSpace(*typ) != "" {
		serviceName = portable.OSServiceNameForTransport(*typ)
	}
	if serviceName == "" {
		fmt.Fprintln(os.Stderr, "uninstall: specify --service NAME or --type reality|vless|vless-ws|hysteria|wireguard|wss|tls")
		os.Exit(2)
	}
	if strings.TrimSpace(*dataDir) != "" {
		installer.SetDataRootOverride(strings.TrimSpace(*dataDir))
	}
	if !*yes {
		if !stdinIsTTY() {
			fmt.Fprintln(os.Stderr, "uninstall: use --yes when stdin is not a terminal")
			os.Exit(2)
		}
		fmt.Printf("Remove %s and TunnelBypass files for this data root? [y/N]: ", serviceName)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}
	if err := freshSetupCleanup(serviceName); err != nil {
		log.Fatalf("uninstall: %v", err)
	}
	fmt.Printf("Done: uninstalled %s.\n", serviceName)
}
