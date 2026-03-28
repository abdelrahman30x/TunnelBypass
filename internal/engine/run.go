package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"tunnelbypass/core/installer"
	tbssh "tunnelbypass/core/ssh"
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
	"tunnelbypass/internal/utils"
)

func externalPortFor(transport string, spec cfg.RunSpec) (int, string) {
	t := strings.ToLower(strings.TrimSpace(transport))
	switch t {
	case "udpgw":
		if spec.UDPGW.Port > 0 {
			return spec.UDPGW.Port, "tcp"
		}
		return 7300, "tcp"
	case "hysteria", "wireguard":
		return spec.Port, "udp"
	case "ssh", "wss", "tls":
		// For SSH-based transports, use SSH.Port (the backend SSH port)
		// This is the internal port that needs conflict checking
		if spec.SSH.Port > 0 {
			return spec.SSH.Port, "tcp"
		}
		// If SSH.Port is not set but Port is, use Port
		if spec.Port > 0 {
			return spec.Port, "tcp"
		}
		return 0, "tcp"
	default:
		return spec.Port, "tcp"
	}
}

func conflictCommandHint(spec cfg.RunSpec) string {
	cmd := utils.AppName() + " run --type " + strings.ToLower(strings.TrimSpace(spec.Transport))
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
		fmt.Printf("[+] Check status: %s status\n", utils.AppName())
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
		!spec.Behavior.NoElevate && !elevate.IsAdmin() {
		fmt.Fprintln(os.Stderr, "[*] Administrator/root is required to install the OS service and firewall rule; requesting elevation...")
		if err := elevate.Elevate(); err != nil {
			return fmt.Errorf("elevation failed or was cancelled: %w\n(hint: use --no-elevate for user-mode only, or `%s run portable ...`)", err, utils.AppName())
		}
		return nil
	}

	log := tblog.Sub("engine")
	opt := types.ConfigOptions{
		Transport:      spec.Transport,
		ServerAddr:     strings.TrimSpace(spec.Server.Address),
		Port:           spec.Port,
		UUID:           strings.TrimSpace(spec.Auth.UUID),
		Sni:            strings.TrimSpace(spec.SNI),
		Host:           strings.TrimSpace(spec.Server.Address),
		SSHUser:        strings.TrimSpace(spec.Auth.SSHUser),
		SSHPassword:    strings.TrimSpace(spec.Auth.SSHPass),
		SSHBackendPort: spec.SSH.Port, // Pass SSH port to provisioning
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
		fmt.Printf("[+] Verify listener: %s status\n", utils.AppName())
		return nil
	}

	pOpts := portable.Options{
		SSHPort:   spec.SSH.Port,
		UDPGWPort: spec.UDPGW.Port,
		SSHUser:   spec.Auth.SSHUser,
		SSHPass:   spec.Auth.SSHPass,
	}
	if cfg.NormalizeTransport(spec.Transport) == "ssh" && spec.UDPGW.External {
		pOpts.ExternalUDPGW = true
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
	return !spec.Behavior.Portable
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
	
	// Get internal and external SSH ports
	internalPort := installer.GetSSHBackendPort()
	externalPort := installer.GetSSHExternalPort()
	
	// For TLS, port config can reflect local stunnel accept / forwarder ports.
	// For WSS, internal is the server SSH backend through the tunnel; "external" is the
	// client-side local port (wstunnel -L / ssh -p), not the server's forwarder ExternalPort.
	if transportName == "tls" {
		portCfg, err := installer.LoadSSHPortConfig()
		if err == nil {
			if portCfg.InternalPort > 0 {
				internalPort = portCfg.InternalPort
			}
			if portCfg.ExternalPort > 0 {
				externalPort = portCfg.ExternalPort
			}
		}
	} else if transportName == "wss" {
		portCfg, err := installer.LoadSSHPortConfig()
		if err == nil && portCfg.InternalPort > 0 {
			internalPort = portCfg.InternalPort
		}
		externalPort = tbssh.WSSClientLocalSSHPort()
	}
	
	// Fallback if not set
	if internalPort <= 0 {
		internalPort = spec.SSH.Port
	}
	if externalPort <= 0 {
		externalPort = internalPort
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
		internalNote := "(for WSS)"
		if transportName == "tls" {
			internalNote = "(stunnel → this port on server)"
		}
		fmt.Printf("  %sSSH (internal):%s %s%d%s %s%s%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, internalPort, uicolors.ColorReset, uicolors.ColorGray, internalNote, uicolors.ColorReset)
		if transportName == "tls" {
			fmt.Printf("  %sSSH (server forwarder):%s %s%d%s %s(optional direct: ssh -p %d … @%s)%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, externalPort, uicolors.ColorReset, uicolors.ColorGray, externalPort, endpoint, uicolors.ColorReset)
			fmt.Printf("  %sAfter stunnel (client):%s %s127.0.0.1:%d%s %s(ssh -p %d … @127.0.0.1)%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, tbssh.TLSClientLocalSSHPort(), uicolors.ColorReset, uicolors.ColorGray, tbssh.TLSClientLocalSSHPort(), uicolors.ColorReset)
		} else {
			fmt.Printf("  %sSSH (external):%s %s%d%s %s(for clients)%s\n", uicolors.ColorGray, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, externalPort, uicolors.ColorReset, uicolors.ColorGray, uicolors.ColorReset)
		}
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
		tlsLocal := tbssh.TLSClientLocalSSHPort()
		fmt.Printf("  %s·%s %sstunnel %s%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, filepath.Join(installer.GetConfigDir("stunnel"), "stunnel-client.conf"), uicolors.ColorReset)
		fmt.Printf("  %s·%s %sssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p %d %s@127.0.0.1%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, tlsLocal, spec.Auth.SSHUser, uicolors.ColorReset)
	case "wss":
		// Modern syntax: -L tcp://localPort:remoteHost:remotePort
		// Use -H "Host: <SNI>" as recommended for stealth (fake SNI / Host Header)
		// wstunnel connects to internal SSH port, client connects to external port via forwarder
		cmd := fmt.Sprintf("wstunnel client -L tcp://127.0.0.1:%d:127.0.0.1:%d wss://%s:%d", externalPort, internalPort, endpoint, spec.Port)
		if sni != "" {
			cmd += fmt.Sprintf(" -H \"Host: %s\" --tls-sni-override %s", sni, sni)
		}
		fmt.Printf("  %s·%s %s%s%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, cmd, uicolors.ColorReset)
		fmt.Printf("  %s·%s %sssh -D 1080 -N -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p %d %s@127.0.0.1%s\n", uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, externalPort, spec.Auth.SSHUser, uicolors.ColorReset)
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
		if spec.Port > 0 {
			fmt.Printf("  %sHandshake needs UDP/%d reachable from the client: allow it in the cloud provider firewall/security group and on the host (tunnelbypass opens rules when run as root).%s\n", uicolors.ColorGray, spec.Port, uicolors.ColorReset)
			fmt.Printf("  %sIf you changed keys or re-ran the wizard, re-import the new client config on every device.%s\n", uicolors.ColorGray, uicolors.ColorReset)
		}
		if runtime.GOOS == "linux" {
			fmt.Printf("  %sServer check: wg show wg_server — if handshakes fail, peer rx/last handshake stay empty.%s\n", uicolors.ColorGray, uicolors.ColorReset)
		}
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
