package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/hysteria"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/transports/wireguard"
	"tunnelbypass/tools/host_catalog"
)

func runToolsMenu(reader *bufio.Reader) {
	for {
		fmt.Printf("\n%s═══ DIAGNOSTIC TOOLS ═══%s\n", ColorBold+ColorBlue, ColorReset)
		fmt.Printf("  %s1)%s %sManage tunnel host catalog%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s2)%s %sCompletely Remove / Uninstall Service%s\n", ColorCyan, ColorReset, ColorRed, ColorReset)
		fmt.Printf("  %sb)%s %sBack to Main Menu%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)

		choice := prompt(reader, fmt.Sprintf("\n%sSelect Tool: %s", ColorBold, ColorReset))

		switch strings.ToLower(choice) {
		case "1":
			runHostCatalogMenu(reader)
		case "2":
			runManageServiceMenu(reader)
		case "b", "back":
			return
		default:
			fmt.Printf("\n%sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func runHostCatalogMenu(reader *bufio.Reader) {
	for {
		fmt.Printf("\n%s═══ HOST CATALOG ═══%s\n", ColorBold+ColorYellow, ColorReset)
		runHosts()
		fmt.Printf("%s1)%s %sAdd host%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("%s2)%s %sRemove host%s %s(by row number or hostname)%s\n", ColorCyan, ColorReset, ColorRed, ColorReset, ColorGray, ColorReset)
		fmt.Printf("%sb)%s %sBack%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)

		choice := strings.ToLower(prompt(reader, fmt.Sprintf("\n%sChoice: %s", ColorBold+ColorYellow, ColorReset)))
		switch choice {
		case "1":
			h := prompt(reader, "Host to add: ")
			added, err := host_catalog.AddHost(h)
			if err != nil {
				fmt.Printf("  %s✗ Failed to add host: %v%s\n", ColorRed, err, ColorReset)
			} else if !added {
				fmt.Printf("  %s[!] Host already exists or invalid.%s\n", ColorYellow, ColorReset)
			} else {
				fmt.Printf("  %s✓ Host added.%s\n", ColorGreen, ColorReset)
			}
		case "2":
			raw := strings.TrimSpace(prompt(reader, "Row number or host to remove: "))
			targetHost := raw
			if idx, err := strconv.Atoi(raw); err == nil {
				hosts := host_catalog.DefaultHosts()
				if idx > 0 && idx <= len(hosts) {
					targetHost = hosts[idx-1]
				} else {
					targetHost = ""
				}
			}
			if targetHost == "" {
				fmt.Printf("  %s[!] Invalid row number.%s\n", ColorYellow, ColorReset)
				continue
			}
			removed, err := host_catalog.RemoveHost(targetHost)
			if err != nil {
				fmt.Printf("  %s✗ Failed to remove host: %v%s\n", ColorRed, err, ColorReset)
			} else if !removed {
				fmt.Printf("  %s[!] Host not found.%s\n", ColorYellow, ColorReset)
			} else {
				fmt.Printf("  %s✓ Host removed.%s\n", ColorGreen, ColorReset)
			}
		case "b", "back":
			return
		default:
			fmt.Printf("  %sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func runHelpMenu(reader *bufio.Reader) {
	base := installer.GetBaseDir()
	fmt.Printf("\n%s═══ USAGE GUIDE ═══%s\n", ColorBold+ColorCyan, ColorReset)
	fmt.Printf("%sWelcome to TunnelBypass!%s\n", ColorBold+ColorGreen, ColorReset)

	fmt.Printf("\n%s1) Setup tunnel%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("   %s- Main Menu -> 1%s %s(Setup/Reinstall Tunnel)%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)
	fmt.Printf("   %s- Choose type%s: Reality / WireGuard / Hysteria / SSH / SSL\n", ColorCyan, ColorReset)
	fmt.Printf("   %s- App creates configs, installs service, and opens firewall rule.%s\n", ColorGray, ColorReset)

	fmt.Printf("\n%s2) Import in client apps%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("   %s- Use sharing links%s from summary screen\n", ColorCyan, ColorReset)
	fmt.Printf("   %s- Or scan QR PNG%s saved in %s%s\\configs%s\n", ColorCyan, ColorReset, ColorBold, base, ColorReset)
	fmt.Printf("   %s- WireGuard direct mode%s uses %swg_client.conf%s file\n", ColorCyan, ColorReset, ColorBold, ColorReset)

	fmt.Printf("\n%s3) Manage service%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("   %s- Diagnostic Tools -> 2%s to uninstall completely\n", ColorCyan, ColorReset)
	fmt.Printf("   %s- Auto service menu%s supports Restart / Stop / Fresh Setup\n", ColorCyan, ColorReset)

	fmt.Printf("\n%s4) Logs & troubleshooting%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("   %s- Logs path:%s %s%s\\logs%s\n", ColorCyan, ColorReset, ColorBold, base, ColorReset)
	fmt.Printf("   %s- Common check:%s service is %sRUNNING%s and port is open\n", ColorCyan, ColorReset, ColorGreen, ColorReset)

	fmt.Printf("\n%sQuick tips%s\n", ColorBold+ColorMagenta, ColorReset)
	fmt.Printf("   %s•%s If no internet on Hysteria, verify client URL has %sinsecure=1%s and retry with a different Port/Obfs settings\n", ColorMagenta, ColorReset, ColorBold, ColorReset)
	fmt.Printf("   %s•%s Keep SNI host valid and reachable\n", ColorMagenta, ColorReset)

	prompt(reader, fmt.Sprintf("\n%sPress Enter to return to Main Menu...%s", ColorGray, ColorReset))
}

func runManageServiceMenu(reader *bufio.Reader) {
	for {
		fmt.Printf("\n%s═══ MANAGE TUNNEL ═══%s\n", ColorBold+ColorYellow, ColorReset)
		fmt.Printf("  %s1)%s %sShow sharing links%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s2)%s %sAdd tunnel hostname (SNI)%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s3)%s %sStop Service%s\n", ColorCyan, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("  %s4)%s %sStart Service%s\n", ColorCyan, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s5)%s %sCompletely Uninstall Service%s\n", ColorCyan, ColorReset, ColorRed, ColorReset)
		fmt.Printf("  %sb)%s %sBack to Diagnostic Tools%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)

		choice := prompt(reader, fmt.Sprintf("\n%sSelect Option: %s", ColorBold+ColorYellow, ColorReset))

		switch strings.ToLower(choice) {
		case "1":
			sName := selectInstalledService(reader, findInstalledServices(), "Choose service for sharing links")
			if sName == "" {
				continue
			}
			displayTunnelSharingLinks(sName)
			prompt(reader, "Press Enter to continue...")
		case "2":
			sName := selectInstalledService(reader, findInstalledServices(), "Choose service to add hostname")
			if sName == "" {
				continue
			}
			addNewSNIForService(reader, sName)
			prompt(reader, "Press Enter to continue...")
		case "3":
			fmt.Printf("    %sStopping Service...%s\n", ColorYellow, ColorReset)
			sName := selectInstalledService(reader, findInstalledServices(), "Choose service to stop")
			if sName == "" {
				continue
			}
			if strings.Contains(sName, "Hysteria") {
				_ = hysteria.UninstallHysteriaService(sName)
			} else if strings.Contains(sName, "WireGuard") {
				_ = wireguard.UninstallWireGuardService(sName)
			} else {
				_ = vless.UninstallXrayService(sName)
			}
			fmt.Printf("    %sService stopped and removed.%s\n", ColorGreen, ColorReset)
		case "4":
			fmt.Printf("    %sStarting service (reinstalling with existing config)...%s\n", ColorYellow, ColorReset)
			sName := selectInstalledService(reader, findInstalledServices(), "Choose service to start")
			if sName == "" {
				continue
			}
			if strings.Contains(sName, "Hysteria") {
				_ = hysteria.InstallHysteriaService(sName, filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml"), 443)
			} else if strings.Contains(sName, "WireGuard") {
				_ = wireguard.InstallWireGuardService(sName, filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf"), 51820)
			} else {
				_ = vless.InstallXrayService(sName, filepath.Join(installer.GetConfigDir("vless"), "server.json"), 443)
			}
		case "5":
			sName := selectInstalledService(reader, findInstalledServices(), "Choose service to uninstall")
			if sName == "" {
				continue
			}
			fmt.Printf("\n%s[!] Removing %s Service...%s\n", ColorRed, sName, ColorReset)
			var err error
			if strings.Contains(sName, "Hysteria") {
				err = hysteria.UninstallHysteriaService(sName)
			} else if strings.Contains(sName, "WireGuard") {
				err = wireguard.UninstallWireGuardService(sName)
			} else {
				err = vless.UninstallXrayService(sName)
			}
			if err == nil {
				fmt.Printf("    %s✓ Service removed.%s\n", ColorGreen, ColorReset)
			} else {
				fmt.Printf("    %s✗ Failed: %v%s\n", ColorRed, err, ColorReset)
			}
		case "b", "back":
			return
		default:
			fmt.Printf("\n%sInvalid choice. Try 1–5 or b.%s\n", ColorRed, ColorReset)
		}
	}
}

func offerUninstallOrphanUDPGW(reader *bufio.Reader) {
	if !serviceExists(installer.UDPGWServiceName) {
		return
	}
	fmt.Printf("\n%s[!] %s is installed (UDP companion for SSH / WSS / TLS).%s\n", ColorYellow+ColorBold, installer.UDPGWServiceName, ColorReset)
	ans := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("    %sRemove this service? [y/N]: %s", ColorBold, ColorReset))))
	if ans != "y" && ans != "yes" {
		return
	}
	installer.UninstallService(installer.UDPGWServiceName)
	removePortAllocState(installer.UDPGWServiceName)
	cleanupArtifactsForTransport(transportUDPGW, installer.UDPGWServiceName)
	fmt.Printf("    %s✓ UDPGW service removed.%s\n", ColorGreen, ColorReset)
}

func maybeRemoveCompanionUDPGW(reader *bufio.Reader) {
	if !serviceExists(installer.UDPGWServiceName) {
		return
	}
	fmt.Printf("\n    %sCompanion %s is still installed.%s\n", ColorYellow, installer.UDPGWServiceName, ColorReset)
	ans := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("    %sRemove UDPGW as well? [Y/n]: %s", ColorBold, ColorReset))))
	if ans == "n" || ans == "no" {
		fmt.Printf("    %sKept %s.%s\n", ColorGray, installer.UDPGWServiceName, ColorReset)
		return
	}
	installer.UninstallService(installer.UDPGWServiceName)
	removePortAllocState(installer.UDPGWServiceName)
	cleanupArtifactsForTransport(transportUDPGW, installer.UDPGWServiceName)
	fmt.Printf("    %s✓ %s removed.%s\n", ColorGreen, installer.UDPGWServiceName, ColorReset)
}

func showUDPGWInstalledMenu(reader *bufio.Reader, serviceName string) bool {
	for {
		fmt.Printf("\n%s════════════════════════════════════════════════════════════%s\n", ColorBold+ColorCyan, ColorReset)
		fmt.Printf("  %s[!] RUNNING SERVICE —%s %s%s%s\n", ColorBold+ColorYellow, ColorReset, ColorBold+ColorGreen, serviceName, ColorReset)
		fmt.Printf("%s════════════════════════════════════════════════════════════%s\n", ColorBold+ColorCyan, ColorReset)
		fmt.Printf("    %sType:%s %sUDP gateway (SSH / WSS / TLS companion)%s\n", ColorGray, ColorReset, ColorBold+ColorCyan, ColorReset)
		fmt.Printf("\n  %s▸ %sCHOOSE AN ACTION%s\n", ColorGray, ColorBold+ColorYellow, ColorReset)
		fmt.Printf("  %s1)%s  %sShow Status & Logs%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorGreen, ColorReset)
		fmt.Printf("  %s3)%s  %sStop Service%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorYellow, ColorReset)
		fmt.Printf("  %s6)%s  %sUninstall UDPGW Service%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorRed, ColorReset)
		fmt.Printf("  %sq)%s  %sExit to Main Menu%s\n", ColorBold+ColorCyan, ColorReset, ColorGray, ColorReset)
		fmt.Printf("  %sx)%s  %sExit Application%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorRed, ColorReset)
		choice := strings.ToLower(prompt(reader, fmt.Sprintf("\n    %sChoice:%s ", ColorBold+ColorYellow, ColorReset)))
		switch choice {
		case "1":
			showServiceStatus(serviceName)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
		case "3":
			fmt.Printf("\n    %s[*] Stopping %s...%s\n", ColorYellow, serviceName, ColorReset)
			installer.UninstallService(serviceName)
			fmt.Printf("    %s✓ Service stopped.%s\n", ColorGreen, ColorReset)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
		case "6":
			fmt.Printf("\n%s[!] Uninstalling %s...%s\n", ColorRed, serviceName, ColorReset)
			installer.UninstallService(serviceName)
			removePortAllocState(serviceName)
			cleanupArtifactsForTransport(transportUDPGW, serviceName)
			fmt.Printf("    %s✓ Service removed.%s\n", ColorGreen, ColorReset)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
			return false
		case "q", "exit":
			return false
		case "x":
			return true
		default:
			fmt.Printf("    %sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func showInstalledMenu(reader *bufio.Reader, serviceName string) bool {
	for {
		tr := detectInstalledTransport(serviceName)
		if tr == transportUDPGW {
			return showUDPGWInstalledMenu(reader, serviceName)
		}
		fmt.Printf("\n%s════════════════════════════════════════════════════════════%s\n", ColorBold+ColorCyan, ColorReset)
		fmt.Printf("  %s[!] RUNNING SERVICE —%s %s%s%s\n", ColorBold+ColorYellow, ColorReset, ColorBold+ColorGreen, serviceName, ColorReset)
		fmt.Printf("%s════════════════════════════════════════════════════════════%s\n", ColorBold+ColorCyan, ColorReset)
		fmt.Printf("    %sType:%s %s%s%s\n", ColorGray, ColorReset, ColorBold+ColorCyan, tr, ColorReset)
		fmt.Printf("\n  %s▸ %sCHOOSE AN ACTION%s\n", ColorGray, ColorBold+ColorYellow, ColorReset)
		fmt.Printf("  %s1)%s  %sShow Status & Logs%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorGreen, ColorReset)
		fmt.Printf("  %s2)%s  %sRestart Service%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorYellow, ColorReset)
		fmt.Printf("  %s3)%s  %sStop Service%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorYellow, ColorReset)

		switch tr {
		case transportXray, transportHysteria:
			fmt.Printf("  %s4)%s  %sAdd tunnel hostname (SNI)%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorGreen, ColorReset)
			fmt.Printf("  %s5)%s  %sShow sharing links%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorMagenta, ColorReset)
		case transportWireGuard:
			fmt.Printf("  %s4)%s  %sShow client config / QR%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorGreen, ColorReset)
		case transportSSH, transportSSL, transportWSS:
			fmt.Printf("  %s4)%s  %sShow instructions%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorGreen, ColorReset)
		default:
		}

		fmt.Printf("  %s6)%s  %sUninstall Service & Remove Files%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorRed, ColorReset)
		fmt.Printf("  %s7)%s  %sReinstall (Fresh Setup)%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorYellow, ColorReset)
		fmt.Printf("  %sq)%s  %sExit to Main Menu%s\n", ColorBold+ColorCyan, ColorReset, ColorGray, ColorReset)
		fmt.Printf("  %sx)%s  %sExit Application%s\n", ColorBold+ColorCyan, ColorReset, ColorBold+ColorRed, ColorReset)

		choice := strings.ToLower(prompt(reader, fmt.Sprintf("\n    %sChoice:%s ", ColorBold+ColorYellow, ColorReset)))

		switch choice {
		case "1":
			showServiceStatus(serviceName)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
		case "2":
			fmt.Printf("\n    %s[*] Restarting %s...%s\n", ColorYellow, serviceName, ColorReset)

			if tr == transportHysteria {
				_ = hysteria.UninstallHysteriaService(serviceName)
				time.Sleep(1 * time.Second)
				_ = hysteria.InstallHysteriaService(serviceName, filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml"), 443)
			} else if tr == transportWireGuard {
				_ = wireguard.UninstallWireGuardService(serviceName)
				time.Sleep(1 * time.Second)
				_ = wireguard.InstallWireGuardService(serviceName, filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf"), 51820)
			} else {
				_ = vless.UninstallXrayService(serviceName)
				time.Sleep(1 * time.Second)
				_ = vless.InstallXrayService(serviceName, filepath.Join(installer.GetConfigDir("vless"), "server.json"), 443)
			}
			fmt.Printf("    %s✓ Service Restarted.%s\n", ColorGreen, ColorReset)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
		case "3":
			fmt.Printf("\n    %s[*] Stopping %s...%s\n", ColorYellow, serviceName, ColorReset)
			if tr == transportHysteria {
				_ = hysteria.UninstallHysteriaService(serviceName)
			} else if tr == transportWireGuard {
				_ = wireguard.UninstallWireGuardService(serviceName)
			} else {
				_ = vless.UninstallXrayService(serviceName)
			}
			fmt.Printf("    %s✓ Service Stopped.%s\n", ColorGreen, ColorReset)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
		case "4":
			switch tr {
			case transportXray, transportHysteria:
				addNewSNI(reader)
			case transportWireGuard:
				displayWireGuardClientInfo()
				prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
			case transportSSH, transportSSL, transportWSS:
				displayInstructionFile(tr)
				prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
			default:
				fmt.Printf("    %sNot available for this tunnel type.%s\n", ColorYellow, ColorReset)
				prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
			}
		case "5":
			if tr == transportXray || tr == transportHysteria {
				displayTunnelSharingLinks(serviceName)
			} else {
				fmt.Printf("    %sNot available for this tunnel type.%s\n", ColorYellow, ColorReset)
			}
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
		case "6":
			fmt.Printf("\n%s[!] Uninstalling %s...%s\n", ColorRed, serviceName, ColorReset)
			if tr == transportHysteria {
				_ = hysteria.UninstallHysteriaService(serviceName)
			} else if tr == transportWireGuard {
				_ = wireguard.UninstallWireGuardService(serviceName)
			} else {
				_ = vless.UninstallXrayService(serviceName)
			}
			cleanupArtifactsForTransport(tr, serviceName)
			if tr == transportSSH || tr == transportSSL || tr == transportWSS {
				maybeRemoveCompanionUDPGW(reader)
			}
			fmt.Printf("    %s✓ All files and services removed.%s\n", ColorGreen, ColorReset)
			prompt(reader, fmt.Sprintf("\n%sPress Enter to continue...%s", ColorGray, ColorReset))
			return false
		case "7":
			fmt.Printf("\n    %s[*] Fresh setup: stopping service, cleaning files...%s\n", ColorYellow, ColorReset)
			if err := freshSetupCleanup(serviceName); err != nil {
				fmt.Printf("    %s✗ Cleanup warning: %v%s\n", ColorRed, err, ColorReset)
			} else {
				fmt.Printf("    %s✓ Removed old service and files.%s\n", ColorGreen, ColorReset)
			}
			time.Sleep(750 * time.Millisecond)
			_ = runSetupWizard(reader)
			return false
		case "q", "exit":
			return false
		case "x":
			return true
		default:
			fmt.Printf("    %sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func showServiceStatus(name string) {
	fmt.Printf("\n%s[ Service Status: %s ]%s\n", ColorBold, name, ColorReset)
	if runtime.GOOS == "windows" {
		_ = exec.Command("sc", "query", name).Run()
	}
	if serviceRunning(name) {
		fmt.Printf("    Status: %sRUNNING%s\n", ColorGreen, ColorReset)
	} else {
		fmt.Printf("    Status: %sSTOPPED%s\n", ColorRed, ColorReset)
	}

	fmt.Printf("\n%s[ Recent Logs ]%s\n", ColorBold, ColorReset)
	base := installer.GetBaseDir()
	logCandidates := []string{
		filepath.Join(base, "logs", "TunnelBypass-Service.log"),
		filepath.Join(base, "logs", name+".out.log"),
		filepath.Join(base, "logs", name+".err.log"),
		filepath.Join(base, "logs", name+".wrapper.log"),
		filepath.Join(base, "logs", "xray_error.log"),
		filepath.Join(base, "logs", "xray_access.log"),
	}
	for _, logPath := range logCandidates {
		if _, err := os.Stat(logPath); err != nil {
			continue
		}
		fmt.Printf("    %s%s%s\n", ColorGray, logPath, ColorReset)
	}
	fmt.Printf("\n    %sNote:%s Open these files to view logs (not printed in CMD).\n", ColorGray, ColorReset)
}
