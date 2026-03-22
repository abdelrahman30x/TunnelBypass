package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/portable"
	"tunnelbypass/core/provision"
	"tunnelbypass/core/svcinstall"
	"tunnelbypass/core/transport"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/cfg"
	"tunnelbypass/internal/elevate"
	"tunnelbypass/internal/runtimeenv"
	"tunnelbypass/internal/tblog"
	"tunnelbypass/internal/uicolors"
)

func externalPortFor(transport string, spec cfg.RunSpec) (int, string) {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "hysteria", "wireguard":
		return spec.Port, "udp"
	default:
		return spec.Port, "tcp"
	}
}

func conflictCommandHint(spec cfg.RunSpec) string {
	cmd := "tunnelbypass run --type " + strings.ToLower(strings.TrimSpace(spec.Transport))
	if spec.Behavior.Portable {
		cmd += " portable"
	}
	if strings.TrimSpace(spec.Paths.DataDir) != "" {
		cmd += " --data-dir " + spec.Paths.DataDir
	}
	return cmd
}

func transportInstallsOSService(transport string) bool {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "reality", "vless", "hysteria", "wireguard", "wss", "tls":
		return true
	default:
		return false
	}
}

func tbNoElevateEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("TB_NO_ELEVATE")))
	return v == "1" || v == "true" || v == "yes"
}

// Run provisions configs then either installs an OS service (server layout) or runs in-process (portable).
func Run(ctx context.Context, spec cfg.RunSpec) error {
	inf := runtimeenv.Detect()
	if inf.LikelyContainer {
		spec.Behavior.Portable = true
		spec.Behavior.Daemon = false
	}
	cfg.FillDefaults(&spec)
	if err := cfg.Validate(spec); err != nil {
		return err
	}

	if strings.TrimSpace(spec.Paths.DataDir) != "" {
		installer.SetDataRootOverride(strings.TrimSpace(spec.Paths.DataDir))
	}
	if spec.Behavior.Portable && strings.TrimSpace(spec.Paths.DataDir) == "" {
		installer.SetDataRootOverride(installer.PortableDefaultDataDir())
	}
	if strings.TrimSpace(spec.Paths.LogsDir) != "" {
		installer.SetLogsRootOverride(strings.TrimSpace(spec.Paths.LogsDir))
	}
	p, netw := externalPortFor(spec.Transport, spec)
	cf := portable.BuildConflict(spec.Transport, p, conflictCommandHint(spec), netw, strings.TrimSpace(spec.Paths.DataDir))
	if cf.Kind == portable.ConflictSameServiceSameConfig {
		fmt.Println("[!] Existing compatible service detected; reusing current runtime.")
		fmt.Println("[+] Check status: tunnelbypass status")
		return nil
	}
	if cf.Kind != portable.ConflictNone {
		if usePrettyResultUI(spec) {
			fmt.Print(FormatPortConflictPretty(cf))
			return fmt.Errorf("listen port %d already in use (stop the service above or choose another port)", cf.Port)
		}
		return errors.New(portable.FormatConflict(cf))
	}

	// OS service + firewall path needs elevated privileges; re-exec via UAC/sudo before provisioning.
	if !spec.Behavior.Portable && transportInstallsOSService(spec.Transport) && !inf.LikelyContainer &&
		spec.Behavior.AutoStart && !spec.Behavior.GenerateOnly &&
		!spec.Behavior.NoElevate && !tbNoElevateEnv() && !elevate.IsAdmin() {
		fmt.Fprintln(os.Stderr, "[*] Administrator/root is required to install the OS service and firewall rule; requesting elevation...")
		if err := elevate.Elevate(); err != nil {
			return fmt.Errorf("elevation failed or was cancelled: %w\n(hint: use --no-elevate or TB_NO_ELEVATE=1 for user-mode only, or `tunnelbypass run portable ...`)", err)
		}
		return nil
	}

	log := tblog.Sub("engine")
	opt := types.ConfigOptions{
		Transport:   spec.Transport,
		ServerAddr:  strings.TrimSpace(spec.Server.Address),
		Port:        spec.Port,
		UUID:        strings.TrimSpace(spec.Auth.UUID),
		Sni:         strings.TrimSpace(spec.SNI),
		Host:        strings.TrimSpace(spec.Server.Address),
		SSHUser:     strings.TrimSpace(spec.Auth.SSHUser),
		SSHPassword: strings.TrimSpace(spec.Auth.SSHPass),
	}

	res, err := provision.ByTransport(log, spec.Transport, opt, "", "")
	if err != nil {
		return err
	}

	// Sync the actual backend port discovered/ensured during provisioning
	if res.SSHPort > 0 {
		spec.SSH.Port = res.SSHPort
	}

	if spec.Behavior.GenerateOnly || !spec.Behavior.AutoStart {
		PrintResult(spec, res)
		return nil
	}

	infRun := runtimeenv.Detect()
	serviceAfterProvision := !spec.Behavior.Portable && transportInstallsOSService(spec.Transport) && !infRun.LikelyContainer
	if serviceAfterProvision {
		PrintResult(spec, res)
		if err := svcinstall.InstallRunTransportService(spec.Transport, opt, elevate.IsAdmin()); err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("[+] Service/supervisor is running in the background; this process exits.")
		fmt.Println("[+] Verify listener: tunnelbypass status")
		return nil
	}

	pOpts := portable.Options{
		SSHPort:   spec.SSH.Port,
		UDPGWPort: spec.UDPGW.Port,
		SSHUser:   spec.Auth.SSHUser,
		SSHPass:   spec.Auth.SSHPass,
	}
	if spec.Transport == "wss" {
		pOpts.WssPort = spec.Port
	}
	if spec.Transport == "tls" {
		pOpts.StunnelAccept = spec.Port
	}
	if spec.Transport == "reality" || spec.Transport == "hysteria" {
		pOpts.ConfigPath = res.ServerConfigPath
	}

	runOne := func(c context.Context) error {
		return portable.RunNamed(c, tblog.Sub("run"), cfg.RunnerTransportFor(spec.Transport), pOpts)
	}
	PrintResult(spec, res)
	if spec.Behavior.Daemon {
		RunDaemonLoop(ctx, spec.Transport, runOne)
		return nil
	}
	return runOne(ctx)
}

func usePrettyResultUI(spec cfg.RunSpec) bool {
	if spec.Behavior.Portable {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(os.Getenv("TB_UI_PRETTY_RESULT"))) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func PrintResult(spec cfg.RunSpec, res transport.Result) {
	if usePrettyResultUI(spec) {
		printPrettyResult(spec, res)
		return
	}
	endpoint := spec.Server.Address
	if endpoint == "" {
		endpoint = "127.0.0.1"
	}
	fmt.Printf("\nTransport: %s\n", spec.Transport)
	fmt.Printf("Server endpoint: %s\n", endpoint)
	fmt.Printf("Listen port: %d\n", spec.Port)
	if sni := strings.TrimSpace(spec.SNI); sni != "" {
		fmt.Printf("Tunnel hostname (SNI): %s\n", sni)
	}
	if res.SharingLink != "" {
		fmt.Printf("\n--- Client: copy this sharing link ---\n%s\n---\n", res.SharingLink)
	}
	tunnelSSH := spec.Transport == "ssh" || spec.Transport == "tls" || spec.Transport == "wss"
	if tunnelSSH && spec.SSH.Port > 0 {
		fmt.Printf("SSH port: %d\n", spec.SSH.Port)
	}
	if tunnelSSH && spec.UDPGW.Port > 0 {
		fmt.Printf("UDPGW port: %d\n", spec.UDPGW.Port)
	}
	if res.ServerConfigPath != "" {
		fmt.Printf("Server config: %s\n", res.ServerConfigPath)
	}
	if res.ClientConfigPath != "" {
		fmt.Printf("Client config: %s\n", res.ClientConfigPath)
	}
	if res.InstructionPath != "" {
		fmt.Printf("Instructions: %s\n", res.InstructionPath)
	}
	if tunnelSSH {
		if strings.TrimSpace(spec.Auth.SSHUser) != "" {
			fmt.Printf("SSH user: %s\n", spec.Auth.SSHUser)
		}
		if strings.TrimSpace(spec.Auth.SSHPass) != "" {
			fmt.Printf("SSH password: %s\n", spec.Auth.SSHPass)
		}
	}
	fmt.Printf("Data dir: %s\n", installer.GetBaseDir())
	fmt.Printf("Logs dir: %s\n", filepath.Join(installer.GetBaseDir(), "logs"))
	if strings.TrimSpace(spec.Paths.LogsDir) != "" {
		fmt.Printf("Logs dir (override): %s\n", spec.Paths.LogsDir)
	}
}

// printPrettyClientTunnel is the compact ssh/tls/wss summary (one box, no instruction file dump).
func printPrettyClientTunnel(spec cfg.RunSpec, _ transport.Result, endpoint, transportName string) {
	dataDir := installer.GetBaseDir()
	sshPort := spec.SSH.Port
	if sshPort <= 0 {
		sshPort = 22
	}
	sni := strings.TrimSpace(spec.SNI)

	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
	fmt.Printf("%s║        %sCLIENT — copy server, login, and commands%s        ║%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorBold, uicolors.ColorCyan, uicolors.ColorReset)
	fmt.Printf("%s╚══════════════════════════════════════════════════════════════╝%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)

	typeLabel := transportName
	switch transportName {
	case "tls":
		typeLabel = "tls (stunnel)"
	case "wss":
		typeLabel = "wss (wstunnel)"
	}
	fmt.Printf("  %sType:%s       %s%s%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorCyan, typeLabel, uicolors.ColorReset)
	fmt.Printf("  %sServer:%s     %s%s%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, endpoint, uicolors.ColorReset)
	if transportName == "ssh" {
		fmt.Printf("  %sSSH port:%s   %s%d%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, spec.Port, uicolors.ColorReset)
	} else {
		fmt.Printf("  %sListen port:%s %s%d%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, spec.Port, uicolors.ColorReset)
		fmt.Printf("  %sSSH (server):%s %s%d%s %s(internal)%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, sshPort, uicolors.ColorReset, uicolors.ColorGray, uicolors.ColorReset)
	}
	if (transportName == "tls" || transportName == "wss") && sni != "" {
		fmt.Printf("  %sSNI / host:%s  %s%s%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorCyan, sni, uicolors.ColorReset)
	}
	if spec.UDPGW.Port > 0 {
		fmt.Printf("  %sUDPGW:%s       %s%d%s %s(optional)%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, spec.UDPGW.Port, uicolors.ColorReset, uicolors.ColorGray, uicolors.ColorReset)
	}
	if strings.TrimSpace(spec.Auth.SSHUser) != "" {
		fmt.Printf("  %sUser:%s       %s%s%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, spec.Auth.SSHUser, uicolors.ColorReset)
	}
	if strings.TrimSpace(spec.Auth.SSHPass) != "" {
		fmt.Printf("  %sPassword:%s   %s%s%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorYellow, spec.Auth.SSHPass, uicolors.ColorReset)
	}

	fmt.Printf("\n  %sCommands%s\n", uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset)
	switch transportName {
	case "ssh":
		fmt.Printf("  %s·%s %sssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p %d %s@%s%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, spec.Port, spec.Auth.SSHUser, endpoint, uicolors.ColorReset)
	case "tls":
		fmt.Printf("  %s·%s %sstunnel %s%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, filepath.Join(installer.GetConfigDir("stunnel"), "stunnel-client.conf"), uicolors.ColorReset)
		fmt.Printf("  %s·%s %sssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 2222 %s@127.0.0.1%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, spec.Auth.SSHUser, uicolors.ColorReset)
	case "wss":
		// Modern syntax: -L tcp://localPort:remoteHost:remotePort
		// Use -H "Host: <SNI>" as recommended for stealth (fake SNI / Host Header)
		cmd := fmt.Sprintf("wstunnel client -L tcp://127.0.0.1:2222:127.0.0.1:%d wss://%s:%d", sshPort, endpoint, spec.Port)
		if sni != "" {
			cmd += fmt.Sprintf(" -H \"Host: %s\" --tls-sni-override %s", sni, sni)
		}
		fmt.Printf("  %s·%s %s%s%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, cmd, uicolors.ColorReset)
		fmt.Printf("  %s·%s %sssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 2222 %s@127.0.0.1%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, spec.Auth.SSHUser, uicolors.ColorReset)
	}

	fmt.Printf("\n  %s(Installed on this machine: %s)%s\n", uicolors.ColorGray, dataDir, uicolors.ColorReset)
}

func printPrettyResult(spec cfg.RunSpec, res transport.Result) {
	endpoint := spec.Server.Address
	if endpoint == "" {
		endpoint = "127.0.0.1"
	}
	transportName := strings.ToLower(strings.TrimSpace(spec.Transport))
	if transportName == "ssh" || transportName == "tls" || transportName == "wss" {
		printPrettyClientTunnel(spec, res, endpoint, transportName)
		return
	}

	dataDir := installer.GetBaseDir()
	logsDir := filepath.Join(dataDir, "logs")

	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", uicolors.ColorBold, uicolors.ColorReset)
	fmt.Printf("║             %sSERVER DEPLOYMENT - SUMMARY%s                    ║\n", uicolors.ColorBold, uicolors.ColorReset)
	fmt.Printf("%s╚══════════════════════════════════════════════════════════════╝%s\n", uicolors.ColorBold, uicolors.ColorReset)
	fmt.Printf("  %sTransport: %s%s%s\n", uicolors.ColorBold, uicolors.ColorCyan, transportName, uicolors.ColorReset)
	fmt.Printf("  %sServer/Endpoint: %s%s%s (Port: %d)\n", uicolors.ColorBold, uicolors.ColorGreen, endpoint, uicolors.ColorReset, spec.Port)
	if strings.TrimSpace(spec.SNI) != "" {
		fmt.Printf("  %sTunnel hostname: %s%s%s\n", uicolors.ColorBold, uicolors.ColorCyan, spec.SNI, uicolors.ColorReset)
	}
	if res.ServerConfigPath != "" {
		fmt.Printf("  %sServer Config:   %s%s%s\n", uicolors.ColorBold, uicolors.ColorGray, res.ServerConfigPath, uicolors.ColorReset)
	}
	if res.ClientConfigPath != "" {
		fmt.Printf("  %sClient Export:   %s%s%s\n", uicolors.ColorBold, uicolors.ColorCyan, res.ClientConfigPath, uicolors.ColorReset)
	}
	if res.InstructionPath != "" {
		fmt.Printf("  %sInstruction file:%s %s%s%s\n", uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset, uicolors.ColorBold, res.InstructionPath, uicolors.ColorReset)
	}
	fmt.Printf("  %sData dir:        %s%s%s\n", uicolors.ColorBold, uicolors.ColorGray, dataDir, uicolors.ColorReset)
	fmt.Printf("  %sLogs Directory:  %s%s%s\n", uicolors.ColorBold, uicolors.ColorBlue, logsDir, uicolors.ColorReset)
	if strings.TrimSpace(spec.Paths.LogsDir) != "" {
		fmt.Printf("  %sLogs Override:   %s%s%s\n", uicolors.ColorBold, uicolors.ColorBlue, spec.Paths.LogsDir, uicolors.ColorReset)
	}

	switch transportName {
	case "reality", "hysteria":
		if res.SharingLink != "" {
			fmt.Printf("\n  %s════════════════════════════════════════════════════════════%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
			fmt.Printf("  %s                [ SHARING LINK - COPY THIS ]%s\n", uicolors.ColorBold+uicolors.ColorGreen, uicolors.ColorReset)
			fmt.Printf("  %s════════════════════════════════════════════════════════════%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
			fmt.Printf("  %s%s%s\n", uicolors.ColorBold+uicolors.ColorGreen, res.SharingLink, uicolors.ColorReset)
			fmt.Printf("  %s════════════════════════════════════════════════════════════%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
		}
		fmt.Printf("\n  %s[ SCAN FOR MOBILE APPS ]%s\n", uicolors.ColorYellow, uicolors.ColorReset)
		fmt.Printf("  %sUse your client app import / subscription from the sharing link above.%s\n", uicolors.ColorGray, uicolors.ColorReset)
	case "wireguard":
		fmt.Printf("\n  %s[ WIREGUARD CLIENT ]%s\n", uicolors.ColorYellow, uicolors.ColorReset)
		if res.ClientConfigPath != "" {
			fmt.Printf("  %sClient file:%s %s%s%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorCyan, res.ClientConfigPath, uicolors.ColorReset)
		}
		if res.SharingLink != "" {
			fmt.Printf("  %sWG URL:%s %s%s%s\n", uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset, uicolors.ColorBold, res.SharingLink, uicolors.ColorReset)
		}
		fmt.Printf("  %sImport this config/URL in your WireGuard app.%s\n", uicolors.ColorGray, uicolors.ColorReset)
		fmt.Printf("\n  %s[ SCAN FOR MOBILE APPS ]%s\n", uicolors.ColorYellow, uicolors.ColorReset)
		fmt.Printf("  %sScan WG QR from your app if you generated one under configs/wireguard.%s\n", uicolors.ColorGray, uicolors.ColorReset)
	default:
		if res.SharingLink != "" {
			fmt.Printf("\n  %s════════════════════════════════════════════════════════════%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
			fmt.Printf("  %s                [ SHARING LINK - COPY THIS ]%s\n", uicolors.ColorBold+uicolors.ColorGreen, uicolors.ColorReset)
			fmt.Printf("  %s════════════════════════════════════════════════════════════%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
			fmt.Printf("  %s%s%s\n", uicolors.ColorBold+uicolors.ColorGreen, res.SharingLink, uicolors.ColorReset)
			fmt.Printf("  %s════════════════════════════════════════════════════════════%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset)
		}
	}
}
