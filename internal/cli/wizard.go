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
	"tunnelbypass/core/types"
	"tunnelbypass/internal/cfg"
	"tunnelbypass/internal/elevate"
	"tunnelbypass/internal/engine"
	"tunnelbypass/internal/runtimeenv"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

func wizardSkipElevate() bool {
	if v := strings.TrimSpace(os.Getenv("TB_SKIP_ELEVATE")); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	return strings.TrimSpace(os.Getenv("TB_DATA_DIR")) != ""
}

func printContainerWizardWarning() {
	if !runtimeenv.InContainer() {
		return
	}
	fmt.Printf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("║  %s[!] Container / Docker detected%s                              ║\n", ColorBold+ColorRed, ColorReset)
	fmt.Printf("║  %sDo not rely on wizard “services” here — use foreground run.%s ║\n", ColorGray, ColorReset)
	fmt.Printf("║  %sDetached processes are a poor fit for container PID 1 / CI.%s ║\n", ColorGray, ColorReset)
	fmt.Printf("║  %sUse instead:%s %stunnelbypass run <transport>%s                 ║\n", ColorGray, ColorReset, ColorBold+ColorGreen, ColorReset)
	fmt.Printf("║  %sExample:%s %sdocker run ... run wss%s                         ║\n", ColorGray, ColorReset, ColorCyan, ColorReset)
	fmt.Printf("║  %sOverride (not recommended):%s TB_ALLOW_SVC_IN_CONTAINER=1      ║\n", ColorGray, ColorReset)
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
			nonUD := filterOutUDPGW(all)
			if len(all) > 0 && len(nonUD) == 0 {
				offerUninstallOrphanUDPGW(reader)
			} else if len(all) == 1 {
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

		fmt.Printf("\n%s[ MAIN MENU ]%s\n", ColorBold+ColorYellow, ColorReset)
		fmt.Printf("1) %sSetup/Reinstall Tunnel%s (Interactive Wizard)\n", ColorGreen, ColorReset)
		fmt.Printf("2) %sDiagnostic Tools%s (Hosts)\n", ColorBlue, ColorReset)
		fmt.Printf("3) %sHow to Use / Help%s\n", ColorCyan, ColorReset)
		fmt.Printf("q) %sExit Application%s\n", ColorRed, ColorReset)

		choice := prompt(reader, fmt.Sprintf("\n%sSelect Option: %s", ColorBold, ColorReset))

		switch strings.ToLower(choice) {
		case "1":
			if !elevate.IsAdmin() && !wizardSkipElevate() {
				if runningViaGoRun() {
					fmt.Printf("\n%s[!] You are using `go run`. Skipping UAC elevation — otherwise this window would close and the wizard would not continue here.%s\n",
						ColorYellow, ColorReset)
					fmt.Printf("    %sFor installs that need Administrator:%s build a binary then run it elevated:%s\n",
						ColorGray, ColorReset, ColorReset)
					fmt.Printf("      %sgo build -o tunnelbypass.exe ./cmd%s\n", ColorBold+ColorCyan, ColorReset)
					fmt.Printf("      %s.\\tunnelbypass.exe%s  (right-click %sRun as administrator%s, or use an elevated prompt)\n\n",
						ColorBold, ColorReset, ColorGray, ColorReset)
				} else {
					_ = os.Setenv("TB_AUTORUN_SETUP", "1")
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

func runSetupWizard(reader *bufio.Reader) bool {
	fmt.Printf("\n%s═══ TUNNEL SETUP WIZARD ═══%s\n", ColorBold+ColorGreen, ColorReset)

	baseDir := installer.GetBaseDir()
	_ = os.MkdirAll(filepath.Join(baseDir, "configs"), 0755)
	_ = os.MkdirAll(filepath.Join(baseDir, "logs"), 0755)

	fmt.Printf("\n%s[1] Select Tunnel Type%s\n", ColorYellow, ColorReset)
	fmt.Printf("    %sDPI bypass stars: more stars = better%s\n", ColorGray, ColorReset)
	fmt.Printf("    1) %sReality / XTLS%s -> %s★★★★★%s\n", ColorGreen, ColorReset, ColorYellow+ColorBold, ColorReset)
	wssDisabled := formatDisabled("wss")
	tlsDisabled := formatDisabled("tls")
	sshDisabled := formatDisabled("ssh")
	fmt.Printf("    2) %sWSS (wstunnel)%s  %s%s%s\n", ColorCyan, ColorReset, colorForDisabled("wss"), wssDisabled, ColorReset)
	fmt.Printf("    3) %sTLS (stunnel)%s   %s%s%s\n", ColorGray, ColorReset, colorForDisabled("tls"), tlsDisabled, ColorReset)
	fmt.Printf("    4) %sQUIC (Hysteria v2)%s -> %s★★%s\n", ColorMagenta, ColorReset, ColorBlue+ColorBold, ColorReset)
	fmt.Printf("    5) %sSSH%s             %s%s%s\n", ColorGray+ColorBold, ColorReset, colorForDisabled("ssh"), sshDisabled, ColorReset)
	fmt.Printf("    6) %sWireGuard%s -> %s★%s\n", ColorBlue, ColorReset, ColorGray+ColorBold, ColorReset)
	fmt.Printf("    %sb)%s %sBack to Main Menu%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)

	choice := strings.ToLower(prompt(reader, fmt.Sprintf("\n    %sChoice [1-6 or b]: %s", ColorBold, ColorReset)))
	if choice == "b" || choice == "back" {
		return false
	}
	switch choice {
	case "1":
		choice = "1" // Reality
	case "2":
		choice = "7" // WSS (wstunnel)
	case "3":
		choice = "4" // SSL (stunnel)
	case "4":
		choice = "6" // Hysteria
	case "5":
		choice = "3" // SSH
	case "6":
		choice = "2" // WireGuard
	}

	var sni string
	if choice == "2" {
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

	transport := wizardChoiceToTransport(choice)
	if transport == "" {
		fmt.Printf("%sInvalid choice.%s\n", ColorRed, ColorReset)
		return false
	}

	if cfg.IsDisabled(transport) {
		fmt.Printf("\n%s[!] Protocol %q is temporarily disabled for maintenance due to known bugs.%s\n", ColorRed, transport, ColorReset)
		fmt.Printf("    %sPlease choose another protocol like Reality (1) or WireGuard (6).%s\n", ColorGray, ColorReset)
		prompt(reader, fmt.Sprintf("\n%sPress Enter to return to selection...%s", ColorGray, ColorReset))
		return false
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

	overlayWindowsService := wizardOverlayWindowsService(transport)
	installAsService := transport == "reality" || transport == "hysteria" || transport == "wireguard" || overlayWindowsService

	if (transport == "ssh" || transport == "wss" || transport == "tls") &&
		(strings.EqualFold(strings.TrimSpace(sshPass), "auto") || strings.TrimSpace(sshPass) == "") {
		sshPass = utils.GenerateUUID()
	}

	rspec := cfg.RunSpec{
		Transport: transport,
		Port:      port,
		SNI:       strings.TrimSpace(sni),
	}
	rspec.Server.Address = strings.TrimSpace(detectedIP)
	rspec.Auth.UUID = "auto"
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
	_ = os.Setenv("TB_UI_PRETTY_RESULT", "1")
	defer os.Unsetenv("TB_UI_PRETTY_RESULT")

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
	case "tls":
		return "TunnelBypass-SSL"
	default:
		return ""
	}
}

func wizardOverlayWindowsService(transport string) bool {
	t := strings.ToLower(strings.TrimSpace(transport))
	if t != "wss" && t != "tls" {
		return false
	}
	if runtime.GOOS != "windows" {
		return false
	}
	if runtimeenv.InContainer() {
		return false
	}
	return elevate.IsAdmin()
}

func formatDisabled(t string) string {
	if cfg.IsDisabled(t) {
		return "(DISABLED)"
	}
	return ""
}

func colorForDisabled(t string) string {
	if cfg.IsDisabled(t) {
		return ColorRed
	}
	return ""
}
