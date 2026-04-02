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
		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s         %s✦  DIAGNOSTIC TOOLS  ✦%s          %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("  %s[1]%s  %sManage Tunnel Host Catalog%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[2]%s  %sCompletely Remove / Uninstall Service%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)
		fmt.Printf("  %s[3]%s  %sReality dest (TCP camouflage)%s  %s(hosts.json + prefs)%s\n", ColorBold+ColorWhite, ColorReset, ColorCyan, ColorReset, ColorGray, ColorReset)
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[B]%s  %sBack to Main Menu%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)

		choice := prompt(reader, fmt.Sprintf("\n%sSelect Tool: %s", ColorBold+ColorYellow, ColorReset))

		switch strings.ToLower(choice) {
		case "1":
			runHostCatalogMenu(reader)
		case "2":
			runManageServiceMenu(reader)
		case "3":
			runRealityDestMenu(reader)
		case "b", "back":
			return
		default:
			fmt.Printf("\n%sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func runRealityDestMenu(reader *bufio.Reader) {
	for {
		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s      %s✦  REALITY DEST (TCP)  ✦%s       %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("  %sUsed as Xray REALITY %sdest%s and Hysteria default masquerade when no tunnel SNI is set.%s\n",
			ColorGray, ColorBold, ColorGray, ColorReset)
		fmt.Printf("  %sDefault TCP address:%s %s\n", ColorGray, ColorReset, host_catalog.DefaultRealityDestAddress())
		pref := host_catalog.PreferredRealityDestHost()
		fmt.Printf("  %sPreferred hostname:%s %s%s%s\n\n", ColorGray, ColorReset, ColorBold, pref, ColorReset)

		list := host_catalog.EffectiveRealityDestHosts()
		for i, h := range list {
			tag := ""
			if host_catalog.IsRealityDestExtraHost(h) {
				tag = ColorGray + " (custom)" + ColorReset
			}
			mark := ""
			if strings.EqualFold(h, pref) {
				mark = ColorGray + "  ← preferred" + ColorReset
			}
			fmt.Printf("  %s[%d]%s  %s%s%s\n", ColorBold+ColorWhite, i+1, ColorReset, h, tag, mark)
		}
		if len(list) == 0 {
			fmt.Printf("  %s(no dest hosts — check embedded hosts.json)%s\n", ColorYellow, ColorReset)
		}
		fmt.Printf("\n  %sEnter 1–%d%s to set preferred dest  %s|  new installs & empty %sreality_dest%s use this\n",
			ColorGray, len(list), ColorReset, ColorGray, ColorBold, ColorReset)
		fmt.Printf("  %s[A]%s  %sAdd custom hostname%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[C]%s  %sClear custom hosts only%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("  %s[P]%s  %sClear preferred (first in list)%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[B]%s  %sBack%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)

		choice := strings.TrimSpace(prompt(reader, fmt.Sprintf("\n%sChoice: %s", ColorBold+ColorYellow, ColorReset)))
		lc := strings.ToLower(choice)

		switch lc {
		case "b", "back":
			return
		case "a":
			raw := strings.TrimSpace(prompt(reader, "Custom hostname (SNI, URL ok): "))
			h := host_catalog.NormalizeHost(raw)
			if h == "" {
				fmt.Printf("  %s[!] Invalid hostname.%s\n", ColorYellow, ColorReset)
				continue
			}
			if err := host_catalog.AddRealityDestExtraHost(h); err != nil {
				fmt.Printf("  %s✗ %v%s\n", ColorRed, err, ColorReset)
				continue
			}
			fmt.Printf("  %s✓ Added %q to dest pool.%s\n", ColorGreen, h, ColorReset)
			set := strings.ToLower(strings.TrimSpace(prompt(reader, "Set as preferred dest? [Y/n]: ")))
			if set == "" || set == "y" || set == "yes" {
				if err := host_catalog.SetPreferredRealityDestHost(h); err != nil {
					fmt.Printf("  %s✗ %v%s\n", ColorRed, err, ColorReset)
					continue
				}
				fmt.Printf("  %s✓ Preferred set to %s.%s\n", ColorGreen, h, ColorReset)
			}
		case "c":
			if err := host_catalog.ClearExtraRealityDestHosts(); err != nil {
				fmt.Printf("  %s✗ %v%s\n", ColorRed, err, ColorReset)
				continue
			}
			fmt.Printf("  %s✓ Custom dest hosts cleared.%s\n", ColorGreen, ColorReset)
		case "p":
			if err := host_catalog.SetPreferredRealityDestHost(""); err != nil {
				fmt.Printf("  %s✗ %v%s\n", ColorRed, err, ColorReset)
				continue
			}
			fmt.Printf("  %s✓ Preferred cleared — default is first host in the list.%s\n", ColorGreen, ColorReset)
		default:
			if n, err := strconv.Atoi(lc); err == nil && n >= 1 && n <= len(list) {
				h := list[n-1]
				if err := host_catalog.SetPreferredRealityDestHost(h); err != nil {
					fmt.Printf("  %s✗ %v%s\n", ColorRed, err, ColorReset)
					continue
				}
				fmt.Printf("  %s✓ Preferred dest set to %s (%s:443).%s\n", ColorGreen, h, h, ColorReset)
				fmt.Printf("  %sRe-run Setup or regenerate configs so existing tunnels pick up the new dest.%s\n", ColorGray, ColorReset)
				continue
			}
			fmt.Printf("  %sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func runHostCatalogMenu(reader *bufio.Reader) {
	for {
		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s           %s✦  HOST CATALOG  ✦%s            %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n", ColorTeal+ColorBold, ColorReset)
		runHosts()
		fmt.Printf("  %s[1]%s  %sAdd Host%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[2]%s  %sRemove Host%s  %s(by row number or hostname)%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset, ColorGray, ColorReset)
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[B]%s  %sBack%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)

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
	fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
	fmt.Printf("%s  ║%s            %s✦  USAGE GUIDE  ✦%s            %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
	fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n", ColorTeal+ColorBold, ColorReset)
	fmt.Printf("\n  %sWelcome to TunnelBypass!%s\n", ColorBold+ColorGreen, ColorReset)

	fmt.Printf("\n%s1) Setup tunnel%s\n", ColorBold+ColorYellow, ColorReset)
	fmt.Printf("   %s- Main Menu -> 1%s %s(Setup/Reinstall Tunnel)%s\n", ColorCyan, ColorReset, ColorGray, ColorReset)
	fmt.Printf("   %s- Choose type%s: Reality, gRPC, Hysteria, WSS Xray, SSH+TLS, …\n", ColorCyan, ColorReset)
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
		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s           %s✦  MANAGE TUNNEL  ✦%s           %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("  %s[1]%s  %sShow Sharing Links%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[2]%s  %sAdd Tunnel Hostname (SNI)%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[3]%s  %sStop Service%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("  %s[4]%s  %sStart Service%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[5]%s  %sCompletely Uninstall Service%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[B]%s  %sBack to Diagnostic Tools%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)

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
				cfgPath, port := xrayServiceConfigPathAndPort(sName)
				_ = vless.InstallXrayService(sName, cfgPath, port)
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
		fmt.Printf("  %s[1]%s  %sShow Status & Logs%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[3]%s  %sStop Service%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("  %s[6]%s  %sUninstall UDPGW Service%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)
		fmt.Printf("  %s[Q]%s  %sExit to Main Menu%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)
		fmt.Printf("  %s[X]%s  %sExit Application%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)
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
			if choice == "" {
				fmt.Printf("    %sNo input — choose 1, 3, 6, q, or x.%s\n", ColorYellow, ColorReset)
			} else {
				fmt.Printf("    %sInvalid choice.%s\n", ColorRed, ColorReset)
			}
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
		fmt.Printf("  %s[1]%s  %sShow Status & Logs%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[2]%s  %sRestart Service%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("  %s[3]%s  %sStop Service%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)

		switch tr {
		case transportXray, transportHysteria, transportSSHTLS, transportGRPC:
			fmt.Printf("  %s[4]%s  %sAdd tunnel hostname (SNI)%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
			fmt.Printf("  %s[5]%s  %sShow sharing links%s\n", ColorBold+ColorWhite, ColorReset, ColorMagenta, ColorReset)
		case transportWireGuard:
			fmt.Printf("  %s[4]%s  %sShow client config / QR%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		case transportSSH, transportSSL, transportWSS:
			fmt.Printf("  %s[4]%s  %sShow instructions%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		default:
		}

		fmt.Printf("  %s[6]%s  %sUninstall Service & Remove Files%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)
		fmt.Printf("  %s[7]%s  %sReinstall (Fresh Setup)%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("  %s[Q]%s  %sExit to Main Menu%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)
		fmt.Printf("  %s[X]%s  %sExit Application%s\n", ColorBold+ColorWhite, ColorReset, ColorRed, ColorReset)

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
				cfgPath, port := xrayServiceConfigPathAndPort(serviceName)
				_ = vless.InstallXrayService(serviceName, cfgPath, port)
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
			case transportXray, transportHysteria, transportSSHTLS, transportGRPC:
				addNewSNIForService(reader, serviceName)
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
			if tr == transportXray || tr == transportHysteria || tr == transportSSHTLS || tr == transportGRPC {
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
			if tr == transportSSH || tr == transportSSL || tr == transportWSS || tr == transportSSHTLS {
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
			if choice == "" {
				fmt.Printf("    %sNo input — choose 1–7, q, or x.%s\n", ColorYellow, ColorReset)
			} else {
				fmt.Printf("    %sInvalid choice.%s\n", ColorRed, ColorReset)
			}
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
		filepath.Join(base, "logs", name+".log"),
	}
	
	if strings.Contains(name, "VLESS") || strings.Contains(name, "Reality") || strings.Contains(name, "Tunnel") || strings.Contains(name, "SSH-TLS") {
		logCandidates = append(logCandidates, filepath.Join(base, "logs", "xray_error.log"))
		logCandidates = append(logCandidates, filepath.Join(base, "logs", "xray_access.log"))
	}
	if strings.Contains(name, "SSH-TLS") {
		logCandidates = append(logCandidates, filepath.Join(base, "logs", "xray_ssh_tls_error.log"))
		logCandidates = append(logCandidates, filepath.Join(base, "logs", "xray_ssh_tls_access.log"))
	}
	if strings.Contains(name, "GRPC") {
		logCandidates = append(logCandidates, filepath.Join(base, "logs", "xray_grpc_error.log"))
		logCandidates = append(logCandidates, filepath.Join(base, "logs", "xray_grpc_access.log"))
	}
	
	for _, logPath := range logCandidates {
		if _, err := os.Stat(logPath); err != nil {
			continue
		}
		fmt.Printf("    %s%s%s\n", ColorGray, logPath, ColorReset)
	}
	fmt.Printf("\n    %sNote:%s Open these files to view logs (not printed in CMD).\n", ColorGray, ColorReset)
	if runtime.GOOS == "linux" {
		fmt.Printf("    %sLinux systemd:%s View logs using `journalctl -f -u %s`\n", ColorYellow, ColorReset, name)
	}
	if runtime.GOOS == "darwin" {
		fmt.Printf("    %smacOS:%s Use Console.app or `log show --last 1h --style syslog` for system logs; TunnelBypass lines are in the files above.\n", ColorYellow, ColorReset)
	}
}

// xrayServiceConfigPathAndPort resolves server.json and listen port for Reality / VLESS-WS / SSH-TLS Xray services.
func xrayServiceConfigPathAndPort(serviceName string) (string, int) {
	cfgPath := filepath.Join(installer.GetConfigDir("vless"), "server.json")
	switch {
	case strings.Contains(serviceName, "SSH-TLS"):
		cfgPath = filepath.Join(installer.GetConfigDir("ssh-tls"), "server.json")
	case strings.Contains(serviceName, "VLESS-WS"):
		cfgPath = filepath.Join(installer.GetConfigDir("vless-ws"), "server.json")
	case strings.Contains(serviceName, "GRPC"):
		cfgPath = filepath.Join(installer.GetConfigDir("vless-grpc"), "server.json")
	}
	return cfgPath, readInboundPortFromServerJSON(cfgPath, 443)
}

