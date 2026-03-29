package cli

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/layout"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/cfg"
	"tunnelbypass/internal/elevate"
	"tunnelbypass/internal/engine"
	"tunnelbypass/internal/runtimeenv"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

func wizardSkipElevate() bool {
	return layout.DataRootOverride() != ""
}

func printContainerWizardWarning() {
	if !runtimeenv.InContainer() {
		return
	}
	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("║  %s[!] Container / Docker detected%s                              ║\n", ColorBold+ColorRed, ColorReset)
	fmt.Printf("║  %sDo not rely on wizard “services” here — use foreground run.%s ║\n", ColorGray, ColorReset)
	fmt.Printf("║  %sDetached processes are a poor fit for container PID 1 / CI.%s ║\n", ColorGray, ColorReset)
	fmt.Printf("║  %sUse instead:%s %s%s run <transport>%s                 ║\n", ColorGray, ColorReset, ColorBold+ColorGreen, utils.AppName(), ColorReset)
	fmt.Printf("║  %sExample:%s %sdocker run ... run wss%s                         ║\n", ColorGray, ColorReset, ColorCyan, ColorReset)
	fmt.Printf("║  %sService install in containers is not supported here.%s              ║\n", ColorGray, ColorReset)
	fmt.Printf("%s╚══════════════════════════════════════════════════════════════╝%s\n\n", ColorBold+ColorYellow, ColorReset)
}

// runningViaGoRun detects `go run` temp binaries; UAC re-exec breaks on that path.
func runningViaGoRun() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	p := strings.ToLower(filepath.Clean(exe))
	return strings.Contains(p, "go-build")
}

func runWizard() {
	reader := bufio.NewReader(os.Stdin)
	printLogo()

	skipAutoServiceMenuOnce := false

	for {
		if skipAutoServiceMenuOnce {
			skipAutoServiceMenuOnce = false
		} else {
			all := findInstalledServices()
			if len(all) == 1 {
				shouldExit := showInstalledMenu(reader, all[0])
				if shouldExit {
					return
				}
				skipAutoServiceMenuOnce = true
			} else if len(all) > 1 {
				selected := selectInstalledService(reader, all, "Installed services detected")
				if selected != "" {
					shouldExit := showInstalledMenu(reader, selected)
					if shouldExit {
						return
					}
					skipAutoServiceMenuOnce = true
				}
			}
		}

		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s             %s✦  MAIN MENU  ✦%s             %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("  %s[1]%s  %sSetup / Reinstall Tunnel%s  %s(Interactive Wizard)%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset, ColorGray, ColorReset)
		fmt.Printf("  %s[2]%s  %sDiagnostic Tools%s          %s(Hosts)%s\n", ColorBold+ColorWhite, ColorReset, ColorCyan, ColorReset, ColorGray, ColorReset)
		fmt.Printf("  %s[3]%s  %sHow to Use / Help%s\n", ColorBold+ColorWhite, ColorReset, ColorBlue, ColorReset)
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[Q]%s  %sExit Application%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)

		choice := prompt(reader, fmt.Sprintf("\n%sSelect Option: %s", ColorBold+ColorYellow, ColorReset))

		switch strings.ToLower(choice) {
		case "1":
			if !elevate.IsAdmin() && !wizardSkipElevate() {
				if runningViaGoRun() {
					fmt.Printf("\n%s[!] You are using `go run`. Skipping elevation — otherwise this window would close and the wizard would not continue here.%s\n",
						ColorYellow, ColorReset)
					fmt.Printf("    %sFor installs that need Administrator/root:%s build a binary then run it elevated:%s\n",
						ColorGray, ColorReset, ColorReset)
					if runtime.GOOS == "windows" {
						fmt.Printf("      %sgo build -o tunnelbypass.exe ./cmd%s\n", ColorBold+ColorCyan, ColorReset)
						fmt.Printf("      %s.\\tunnelbypass.exe%s  (right-click %sRun as administrator%s, or use an elevated prompt)\n\n",
							ColorBold, ColorReset, ColorGray, ColorReset)
					} else {
						fmt.Printf("      %sgo build -o tunnelbypass ./cmd%s\n", ColorBold+ColorCyan, ColorReset)
						fmt.Printf("      %s./tunnelbypass%s  (run with sudo or as root for required permissions)\n\n",
							ColorBold, ColorReset)
					}
				} else {
					_ = os.Setenv("TUNNELBYPASS_AUTORUN_SETUP", "1")
					err := elevate.Elevate()
					if err != nil {
						fmt.Printf("\n%s[!] Elevation not available (%v). Continuing with current user; data dir: %s%s\n",
							ColorYellow, err, installer.GetBaseDir(), ColorReset)
					}
				}
			}
			if runSetupWizard(reader) {
				skipAutoServiceMenuOnce = true
			}
		case "2":
			runToolsMenu(reader)
		case "3":
			runHelpMenu(reader)
		case "q", "exit":
			fmt.Printf("\n%sGoodbye!%s\n", ColorCyan, ColorReset)
			return
		default:
			fmt.Printf("\n%sInvalid choice. Try again.%s\n", ColorRed, ColorReset)
		}
	}
}

// menuSep40 is ASCII-only so classic Windows CMD and UTF-8 terminals both render a clean line.
const menuSep40 = "----------------------------------------"

func stealthTag(level string) string {
	switch level {
	case "high":
		return ColorBold + ColorGreen + "[HIGH]" + ColorReset
	case "med":
		return ColorBold + ColorYellow + "[MED] " + ColorReset
	default:
		return ColorBold + ColorRed + "[LOW] " + ColorReset
	}
}

func printTunnelModeMenu() {
	const sep = "  ─────────────────────────────────────────────────"
	fmt.Printf("\n%s  ╔═════════════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
	fmt.Printf("%s  ║%s            %s✦  TUNNEL SETUP WIZARD  ✦%s            %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
	fmt.Printf("%s  ╚═════════════════════════════════════════════════╝%s\n", ColorTeal+ColorBold, ColorReset)
	fmt.Printf("  %sSelect tunnel mode  —  higher stealth = harder to block by DPI%s\n", ColorGray, ColorReset)

	fmt.Printf("\n  %s★ RECOMMENDED%s\n", ColorBold+ColorGreen, ColorReset)
	fmt.Printf("%s%s%s\n", ColorGray, sep, ColorReset)
	fmt.Printf("  %s[1]%s  %sReality%s      %s(VLESS + XTLS Vision)%s  Stealth: %s  %sTCP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorGreen, ColorReset,
		ColorGray, ColorReset, stealthTag("high"), ColorGray, ColorReset, itemDisabledSuffix("reality"))
	fmt.Printf("  %s[2]%s  %sQUIC%s         %s(Hysteria v2)%s           Stealth: %s  %sUDP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorMagenta, ColorReset,
		ColorGray, ColorReset, stealthTag("high"), ColorGray, ColorReset, itemDisabledSuffix("hysteria"))

	fmt.Printf("\n  %s◈ BALANCED%s\n", ColorBold+ColorCyan, ColorReset)
	fmt.Printf("%s%s%s\n", ColorGray, sep, ColorReset)
	fmt.Printf("  %s[3]%s  %sWSS Xray%s     %s(WebSocket + TLS)%s       Stealth: %s  %sTCP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorCyan, ColorReset,
		ColorGray, ColorReset, stealthTag("med"), ColorGray, ColorReset, itemDisabledSuffix("vless-ws"))
	fmt.Printf("  %s[4]%s  %sWireGuard%s    %s(VPN Tunnel)%s            Stealth: %s  %sUDP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorBlue, ColorReset,
		ColorGray, ColorReset, stealthTag("low"), ColorGray, ColorReset, itemDisabledSuffix("wireguard"))

	fmt.Printf("\n  %s◇ SIMPLE%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("%s%s%s\n", ColorGray, sep, ColorReset)
	fmt.Printf("  %s[5]%s  %sWSTunnel%s     %s(Raw over WebSocket)%s    Stealth: %s  %sTCP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorYellow, ColorReset,
		ColorGray, ColorReset, stealthTag("med"), ColorGray, ColorReset, itemDisabledSuffix("wss"))
	fmt.Printf("  %s[6]%s  %sTLS%s          %s(stunnel)%s               Stealth: %s  %sTCP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorWhite, ColorReset,
		ColorGray, ColorReset, stealthTag("low"), ColorGray, ColorReset, itemDisabledSuffix("tls"))
	fmt.Printf("  %s[7]%s  %sSSH Tunnel%s   %s(legacy)%s                Stealth: %s  %sTCP%s%s\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorWhite, ColorReset,
		ColorGray, ColorReset, stealthTag("low"), ColorGray, ColorReset, itemDisabledSuffix("ssh"))

	fmt.Printf("%s%s%s\n", ColorGray, sep, ColorReset)
	fmt.Printf("  %s[B]%s  Back to Main Menu    %s[Q]%s  Exit\n",
		ColorBold+ColorWhite, ColorReset, ColorBold+ColorWhite, ColorReset)
}

// promptWSSInstallationScreen explains what the WSS (wstunnel) path actually installs and asks to proceed.
// This project does not install Xray VLESS-WSS, Nginx, or Certbot; those are out of scope here.
func promptWSSInstallationScreen(reader *bufio.Reader) bool {
	wssTitleSep := "========================================"
	fmt.Printf("\n%s%s%s\n", ColorBold+ColorCyan, wssTitleSep, ColorReset)
	fmt.Printf("%sWSS (WebSocket + TLS) Installation%s\n", ColorBold+ColorCyan, ColorReset)
	fmt.Printf("%s%s%s\n\n", ColorBold+ColorCyan, wssTitleSep, ColorReset)

	fmt.Printf("%sSelected Mode:%s WSS (WebSocket + TLS - wstunnel)\n", ColorBold, ColorReset)
	fmt.Printf("%sStealth Level :%s %s****%s (Balanced)\n\n", ColorBold, ColorReset, ColorYellow+ColorBold, ColorReset)

	fmt.Printf("%s%s%s\n", ColorGray, menuSep40, ColorReset)
	fmt.Printf("%sWhat will be installed:%s\n\n", ColorBold, ColorReset)

	fmt.Printf("%s1. wstunnel (WSS server)%s\n", ColorBold, ColorReset)
	fmt.Printf("   %s* TunnelBypass downloads/uses the wstunnel binary%s\n", ColorGray, ColorReset)
	fmt.Printf("   %s* WebSocket + TLS listener on your chosen port (e.g. 443)%s\n", ColorGray, ColorReset)
	fmt.Printf("   %s* TLS cert/key under the app data dir (self-signed for your SNI/domain)%s\n\n", ColorGray, ColorReset)

	fmt.Printf("%s2. Embedded SSH tunnel%s\n", ColorBold, ColorReset)
	fmt.Printf("   %s* SSH serves the tunneled TCP; wstunnel restricts to localhost SSH%s\n\n", ColorGray, ColorReset)

	fmt.Printf("%s3. OS service (when installed as admin/root)%s\n", ColorBold, ColorReset)
	fmt.Printf("   %s* Registers %sTunnelBypass-WSS%s (plus SSH/UDPGW peers as needed)%s\n\n", ColorGray, ColorCyan, ColorGray, ColorReset)

	fmt.Printf("%s%s%s\n", ColorGray, menuSep40, ColorReset)
	fmt.Printf("%sHow it works:%s\n\n", ColorBold, ColorReset)
	fmt.Printf("  %sClient -> WSS (TLS) -> wstunnel -> SSH (loopback) -> your tunnel%s\n\n", ColorGray, ColorReset)
	fmt.Printf("  %s- Traffic looks like HTTPS upgrade (WebSocket)%s\n", ColorGray, ColorReset)
	fmt.Printf("  %s- Path/header payload-style routing is not configured by this CLI path%s\n\n", ColorGray, ColorReset)

	fmt.Printf("%s%s%s\n", ColorGray, menuSep40, ColorReset)
	fmt.Printf("%sWhat you need:%s\n\n", ColorBold, ColorReset)
	fmt.Printf("  %s- A domain or hostname for TLS CN/SNI (recommended)%s\n", ColorGray, ColorReset)
	fmt.Printf("  %s- The listen port reachable from clients (often 443)%s\n", ColorGray, ColorReset)
	fmt.Printf("  %s- Administrator / root for installing Windows/Linux services%s\n\n", ColorGray, ColorReset)

	fmt.Printf("%s%s%s\n", ColorGray, menuSep40, ColorReset)
	fmt.Printf("%sConfiguration (next steps in this wizard):%s\n\n", ColorBold, ColorReset)
	fmt.Printf("  %s- Domain / SNI (host catalog or custom)%s\n", ColorGray, ColorReset)
	fmt.Printf("  %s- Listen port%s\n", ColorGray, ColorReset)
	fmt.Printf("  %s- SSH user & password (for the embedded SSH)%s\n\n", ColorGray, ColorReset)

	fmt.Printf("%s%s%s\n", ColorGray, menuSep40, ColorReset)
	fmt.Printf("%sNotes:%s\n\n", ColorBold, ColorReset)
	fmt.Printf("  %s[+]%s Strong fit for WSS + SSH tunneling via wstunnel\n", ColorGreen+ColorBold, ColorReset)
	fmt.Printf("  %s[+]%s Works on many networks where HTTPS/WSS are allowed\n", ColorGreen+ColorBold, ColorReset)
	fmt.Printf("  %s[!]%s For production HTTPS with a public CA, use your own reverse proxy / certs (not automated here)\n\n", ColorYellow+ColorBold, ColorReset)

	fmt.Printf("%s%s%s\n", ColorGray, menuSep40, ColorReset)
	ans := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("\n%sProceed with automatic installation? (y/n): %s", ColorBold, ColorReset))))
	fmt.Printf("%s%s%s\n", ColorBold+ColorCyan, wssTitleSep, ColorReset)
	if ans == "y" || ans == "yes" {
		return true
	}
	return false
}

func runSetupWizard(reader *bufio.Reader) bool {
	baseDir := installer.GetBaseDir()
	_ = os.MkdirAll(filepath.Join(baseDir, "configs"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "logs"), 0755)

	printTunnelModeMenu()

	menuChoice := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("\n%sChoice: %s", ColorBold, ColorReset))))
	if menuChoice == "b" || menuChoice == "back" {
		return false
	}
	if menuChoice == "q" || menuChoice == "exit" {
		fmt.Printf("\n%sGoodbye!%s\n", ColorCyan, ColorReset)
		os.Exit(0)
	}

	var internalChoice string
	switch menuChoice {
	case "1":
		internalChoice = "1" // reality
	case "2":
		internalChoice = "6" // hysteria
	case "3":
		internalChoice = "8" // vless-ws (Xray)
	case "4":
		internalChoice = "2" // wireguard
	case "5":
		internalChoice = "7" // wss (wstunnel)
	case "6":
		internalChoice = "4" // tls
	case "7":
		internalChoice = "3" // ssh
	default:
		internalChoice = ""
	}

	transport := wizardChoiceToTransport(internalChoice)
	if transport == "" {
		fmt.Printf("%sInvalid choice.%s\n", ColorRed, ColorReset)
		return false
	}
	if cfg.IsDisabled(transport) {
		fmt.Printf("\n%s[!] Protocol %q is temporarily disabled (known issues).%s\n", ColorRed, transport, ColorReset)
		fmt.Printf("    %sChoose another option, e.g. Reality (1), QUIC (2), WSS Xray (3), or TLS (6).%s\n", ColorGray, ColorReset)
		prompt(reader, fmt.Sprintf("\n%sPress Enter to return to selection...%s", ColorGray, ColorReset))
		return false
	}
	var wsPathInput string
	var uuidCustom string
	if transport == "wss" {
		if !promptWSSInstallationScreen(reader) {
			return false
		}
	}

	var sni string
	if internalChoice == "2" {
		fmt.Printf("\n%s[2] Tunnel hostname (SNI / bug host)%s %sskipped for this tunnel type%s\n", ColorYellow, ColorReset, ColorGray, ColorReset)
		sni = ""
	} else {
		fmt.Printf("\n%s[2] Tunnel hostname (SNI / bug host) — optional%s\n", ColorYellow, ColorReset)
		fmt.Printf("    %sHost categories:%s\n", ColorGray, ColorReset)
		categories := host_catalog.CategoryOrder()
		if len(categories) == 0 {
			fmt.Printf("    %s[!] No host categories configured.%s\n", ColorYellow, ColorReset)
		}
		for i, c := range categories {
			fmt.Printf("    %s%2d)%s %s%s%s\n", ColorCyan, i+1, ColorReset, ColorGreen, host_catalog.CategoryLabel(c), ColorReset)
		}
		catChoice := strings.TrimSpace(strings.ToLower(prompt(reader, fmt.Sprintf("\n    %sCategory choice [1-%d]: %s", ColorBold, len(categories), ColorReset))))
		selectedCategory := "custom"
		if len(categories) > 0 {
			selectedCategory = categories[0]
		}
		if catChoice != "" {
			if idx, err := strconv.Atoi(catChoice); err == nil && idx >= 1 && idx <= len(categories) {
				selectedCategory = categories[idx-1]
			}
		}
		hosts := host_catalog.HostsByCategory(selectedCategory)
		if len(hosts) == 0 && len(categories) > 0 {
			for _, cat := range categories {
				if h := host_catalog.HostsByCategory(cat); len(h) > 0 {
					hosts = h
					selectedCategory = cat
					break
				}
			}
		}
		fmt.Printf("\n    %sSelected category:%s %s%s%s\n", ColorGray, ColorReset, ColorBold, host_catalog.CategoryLabel(selectedCategory), ColorReset)
		for i, domain := range hosts {
			fmt.Printf("    %s%2d)%s %s%s%s\n", ColorCyan, i+1, ColorReset, ColorGreen, domain, ColorReset)
		}
		fmt.Printf("    %sc)%s %sCustom host%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)
		fmt.Printf("    %sn)%s %sSkip (no host from list)%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)

		sniChoice := prompt(reader, fmt.Sprintf("\n    %sChoice: %s", ColorBold, ColorReset))
		if strings.ToLower(sniChoice) == "c" {
			sni = prompt(reader, fmt.Sprintf("    %sCustom hostname: %s", ColorBold, ColorReset))
		} else if strings.ToLower(sniChoice) == "n" || sniChoice == "" {
			sni = ""
		} else {
			idx, _ := strconv.Atoi(sniChoice)
			if idx > 0 && idx <= len(hosts) {
				sni = hosts[idx-1]
			}
		}
	}

	fmt.Printf("\n%s[3] Detecting Server Public IP...%s ", ColorYellow, ColorReset)
	detectedIP := utils.GetPublicIP()
	flushInput(reader) // Clear pending Enter keystrokes
	if detectedIP != "" {
		fmt.Printf("%sFound: %s%s\n", ColorGreen, detectedIP, ColorReset)
	} else {
		fmt.Printf("%sFailed to detect.%s\n", ColorRed, ColorReset)
		detectedIP = prompt(reader, fmt.Sprintf("    %sEnter Server IP Address manually: %s", ColorBold, ColorReset))
	}

	if transport == "vless-ws" {
		wsPathInput = strings.TrimSpace(prompt(reader, fmt.Sprintf("\n%sWebSocket path (e.g. /ws, /api) [%s]: %s", ColorYellow, "/", ColorBold)))
		wsPathInput = vless.NormalizeWSPath(wsPathInput)
	}

	defaultPort := 443
	switch transport {
	case "hysteria":
		defaultPort = 8443
	case "wireguard":
		defaultPort = 51820
	case "ssh":
		defaultPort = 22
	}
	portInput := prompt(reader, fmt.Sprintf("\n%s[4] Listen port [%d]: %s", ColorYellow, defaultPort, ColorReset))
	port := cfg.ParsePortOrDefault(portInput, defaultPort)

	var sshUser, sshPass string
	if transport == "ssh" || transport == "tls" || transport == "wss" {
		sshUser = strings.TrimSpace(prompt(reader, fmt.Sprintf("%s[5] SSH user [tunnelbypass]: %s", ColorYellow, ColorReset)))
		if sshUser == "" {
			sshUser = "tunnelbypass"
		}
		sshPass = strings.TrimSpace(prompt(reader, fmt.Sprintf("%s[6] SSH password [auto]: %s", ColorYellow, ColorReset)))
		if sshPass == "" {
			sshPass = "auto"
		}
	}

	installAsService := transport == "reality" || transport == "hysteria" || transport == "wireguard" || transport == "vless-ws" || transport == "wss" || transport == "tls"

	if (transport == "ssh" || transport == "wss" || transport == "tls") &&
		(strings.EqualFold(strings.TrimSpace(sshPass), "auto") || strings.TrimSpace(sshPass) == "") {
		// Use the persistent password stored in embed_password.txt (creating it on first run).
		// This ensures the same password is used across wizard runs and running services,
		// preventing "Permission denied" when the service was started with a previous password.
		sshPass = installer.ReadOrCreateEmbedSSHPassword()
	}

	rspec := cfg.RunSpec{
		Transport: transport,
		Port:      port,
		SNI:       strings.TrimSpace(sni),
		WSPath:    wsPathInput,
	}
	rspec.Server.Address = strings.TrimSpace(detectedIP)
	if transport == "vless-ws" && uuidCustom != "" {
		rspec.Auth.UUID = uuidCustom
	} else {
		rspec.Auth.UUID = "auto"
	}
	rspec.Auth.SSHUser = sshUser
	rspec.Auth.SSHPass = sshPass
	switch transport {
	case "ssh":
		rspec.SSH.Port = port
	default:
		rspec.SSH.Port = 0
	}
	rspec.UDPGW.Enabled = true
	rspec.UDPGW.Port = 7300
	rspec.Behavior.Portable = false
	rspec.Behavior.AutoStart = true
	rspec.Paths.DataDir = baseDir
	if installAsService {
		rspec.Behavior.GenerateOnly = true
		rspec.Behavior.AutoStart = false
	}
	if err := engine.Run(context.Background(), rspec); err != nil {
		fmt.Printf("\n%s✗ Setup failed: %v%s\n", ColorRed, err, ColorReset)
		prompt(reader, fmt.Sprintf("\n%sPress Enter to return to Main Menu...%s", ColorGray, ColorReset))
		return false
	}
	if installAsService {
		opt := types.ConfigOptions{
			Transport:   rspec.Transport,
			Port:        rspec.Port,
			SSHUser:     rspec.Auth.SSHUser,
			SSHPassword: rspec.Auth.SSHPass,
			WSPath:      strings.TrimSpace(rspec.WSPath),
		}
		if err := runTryInstallService(slog.Default(), rspec.Transport, opt, elevate.IsAdmin()); err != nil {
			fmt.Printf("\n%s✗ Service install failed: %v%s\n", ColorRed, err, ColorReset)
			fmt.Printf("%s[!] Configs generated successfully, but service was not installed.%s\n", ColorYellow, ColorReset)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to return to Main Menu...%s", ColorGray, ColorReset))
			return false
		}
		fmt.Printf("\n%s✓ Service installed and started successfully.%s\n", ColorBold+ColorGreen, ColorReset)
		fmt.Printf("%s[+]%s %sTunnel will remain running even after closing this CLI.%s\n", ColorBold+ColorCyan, ColorReset, ColorCyan, ColorReset)

		preferred := preferredServiceNameForTransport(rspec.Transport)
		installed := findInstalledServices()
		selected := ""
		for _, s := range installed {
			if strings.EqualFold(s, preferred) {
				selected = s
				break
			}
		}
		if selected == "" && len(installed) == 1 {
			selected = installed[0]
		}
		if selected != "" {
			_ = showInstalledMenu(reader, selected)
			return true
		}
	}
	fmt.Printf("\n%s✓ Setup completed successfully.%s\n", ColorGreen, ColorReset)
	prompt(reader, fmt.Sprintf("\n%sPress Enter to return to Main Menu...%s", ColorGray, ColorReset))
	return true
}

func wizardChoiceToTransport(choice string) string {
	switch choice {
	case "1":
		return "reality"
	case "2":
		return "wireguard"
	case "3":
		return "ssh"
	case "4":
		return "tls"
	case "6":
		return "hysteria"
	case "7":
		return "wss"
	case "8":
		return "vless-ws"
	default:
		return ""
	}
}

func preferredServiceNameForTransport(transport string) string {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "reality":
		return "TunnelBypass-VLESS"
	case "hysteria":
		return "TunnelBypass-Hysteria"
	case "wireguard":
		return "TunnelBypass-WireGuard"
	case "wss":
		return "TunnelBypass-WSS"
	case "vless-ws":
		return "TunnelBypass-VLESS-WS"
	case "tls":
		return "TunnelBypass-SSL"
	default:
		return ""
	}
}

func itemDisabledSuffix(t string) string {
	if cfg.IsDisabled(t) {
		return "  " + ColorRed + "(DISABLED)" + ColorReset
	}
	return ""
}
