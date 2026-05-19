package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/destprobe"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

type realityDestState struct {
	ConfigPath string
	Host       string
	Dest       string
	ExtraHosts []string
}

var restartXrayAfterRealityDestChange = func(serviceName, configPath string, port int) error {
	_ = vless.UninstallXrayService(serviceName)
	time.Sleep(1 * time.Second)
	return vless.InstallXrayService(serviceName, configPath, port, types.ConfigOptions{})
}

func serviceSupportsRealityDest(serviceName string) bool {
	switch detectInstalledTransport(serviceName) {
	case transportXray, transportGRPC:
		return true
	default:
		return false
	}
}

func serviceHasConfigSettings(serviceName string) bool {
	if serviceSupportsRealityDest(serviceName) {
		return true
	}
	_, ok := serverConfigPathForService(serviceName)
	return ok
}

func transportSupportsRealityDestName(transport string) bool {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "reality", "vless", "vless-grpc", "grpc", "grpc-tls":
		return true
	default:
		return false
	}
}

func runConfigSettingsMenu(reader *bufio.Reader, serviceName string) {
	for {
		serverPath, hasServerFile := serverConfigPathForService(serviceName)
		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s          %s✦  CONFIG SETTINGS  ✦%s          %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n\n", ColorTeal+ColorBold, ColorReset)
		if hasServerFile {
			fmt.Printf("  %s[1]%s  %sOpen server config file%s  %s(%s)%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset, ColorGray, serverPath, ColorReset)
		}
		if serviceSupportsRealityDest(serviceName) {
			fmt.Printf("  %s[2]%s  %sReality dest (TCP camouflage)%s  %s(per-config hosts.json + prefs)%s\n", ColorBold+ColorWhite, ColorReset, ColorCyan, ColorReset, ColorGray, ColorReset)
		}
		if !hasServerFile && !serviceSupportsRealityDest(serviceName) {
			fmt.Printf("  %sNo editable config settings for this tunnel type yet.%s\n", ColorGray, ColorReset)
		}
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[B]%s  %sBack%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)

		choice := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("\n%sChoice: %s", ColorBold+ColorYellow, ColorReset))))
		switch choice {
		case "1":
			if hasServerFile {
				openServerConfigForService(serviceName)
			} else {
				fmt.Printf("    %sNo server config file found for this tunnel type.%s\n", ColorYellow, ColorReset)
			}
		case "2":
			if serviceSupportsRealityDest(serviceName) {
				runServiceRealityDestMenu(reader, serviceName)
			} else {
				fmt.Printf("    %sNot available for this tunnel type.%s\n", ColorYellow, ColorReset)
			}
		case "b", "back":
			return
		default:
			fmt.Printf("    %sInvalid choice.%s\n", ColorRed, ColorReset)
		}
	}
}

func serverConfigPathForService(serviceName string) (string, bool) {
	switch detectInstalledTransport(serviceName) {
	case transportXray, transportGRPC, transportSSHTLS, transportXDNS:
		return xrayConfigPathForSNI(serviceName), true
	case transportHysteria:
		return filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml"), true
	case transportWireGuard:
		return filepath.Join(installer.GetConfigDir("wireguard"), "wg_server.conf"), true
	case transportShadowsocks, transportShadowsocksWS:
		return filepath.Join(installer.GetConfigDir("shadowsocks"), "server.json"), true
	case transportMDNS:
		return filepath.Join(installer.GetConfigDir("mdns"), "server_config.toml"), true
	case transportSSL:
		p := filepath.Join(installer.GetConfigDir("stunnel"), "stunnel-server.conf")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		legacy := filepath.Join(installer.GetConfigDir("stunnel"), "stunnel_server.conf")
		if _, err := os.Stat(legacy); err == nil {
			return legacy, true
		}
		return p, true
	case transportSSH:
		return filepath.Join(installer.GetConfigDir("ssh"), "ssh_tunnel_instructions.txt"), true
	case transportWSS:
		return filepath.Join(installer.GetConfigDir("wstunnel"), "wss_tunnel_instructions.txt"), true
	default:
		return "", false
	}
}

func openServerConfigForService(serviceName string) {
	path, ok := serverConfigPathForService(serviceName)
	if !ok || strings.TrimSpace(path) == "" {
		fmt.Printf("    %sNo server config file found for this tunnel type.%s\n", ColorYellow, ColorReset)
		return
	}
	if _, err := os.Stat(path); err != nil {
		fmt.Printf("    %s✗ Server file not found: %s (%v)%s\n", ColorRed, path, err, ColorReset)
		return
	}
	if err := openFileInEditor(path); err != nil {
		fmt.Printf("    %s✗ Could not open editor: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	fmt.Printf("    %s✓ Editor closed: %s%s\n", ColorGreen, path, ColorReset)
}

func openFileInEditor(path string) error {
	name, args, err := editorCommand(path)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func editorCommand(path string) (string, []string, error) {
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("notepad.exe"); err == nil {
			return p, []string{path}, nil
		}
		if p, err := exec.LookPath("notepad"); err == nil {
			return p, []string{path}, nil
		}
		return "", nil, fmt.Errorf("notepad.exe not found in PATH")
	}
	if runtime.GOOS == "linux" {
		if p, err := exec.LookPath("nano"); err == nil {
			return p, []string{path}, nil
		}
	}
	if ed := strings.TrimSpace(os.Getenv("EDITOR")); ed != "" {
		parts := strings.Fields(ed)
		if len(parts) > 0 {
			if p, err := exec.LookPath(parts[0]); err == nil {
				return p, append(parts[1:], path), nil
			}
		}
	}
	for _, candidate := range []string{"nano", "vi", "vim"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return p, []string{path}, nil
		}
	}
	if runtime.GOOS == "darwin" {
		if p, err := exec.LookPath("open"); err == nil {
			return p, []string{"-t", path}, nil
		}
	}
	return "", nil, fmt.Errorf("no supported editor found (tried nano, $EDITOR, vi/vim)")
}

func runServiceRealityDestMenu(reader *bufio.Reader, serviceName string) {
	for {
		st, err := readRealityDestStateForService(serviceName)
		if err != nil {
			fmt.Printf("    %s✗ Cannot read Reality dest settings: %v%s\n", ColorRed, err, ColorReset)
			return
		}
		prefs := host_catalog.RealityDestPrefs{PreferredHost: st.Host, ExtraHosts: st.ExtraHosts}
		list := host_catalog.EffectiveRealityDestHostsForPrefs(prefs)

		fmt.Printf("\n%s  ╔═════════════════════════════════════════╗%s\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ║%s      %s✦  REALITY DEST (TCP)  ✦%s       %s║%s\n", ColorTeal+ColorBold, ColorReset, ColorBold+ColorWhite, ColorReset, ColorTeal+ColorBold, ColorReset)
		fmt.Printf("%s  ╚═════════════════════════════════════════╝%s\n\n", ColorTeal+ColorBold, ColorReset)
		fmt.Printf("  %sConfig:%s %s\n", ColorGray, ColorReset, st.ConfigPath)
		fmt.Printf("  %sCurrent dest:%s %s%s%s", ColorGray, ColorReset, ColorBold, st.Dest, ColorReset)
		if st.Host != "" {
			fmt.Printf("  %s(SNI: %s)%s", ColorGray, st.Host, ColorReset)
		}
		fmt.Println()
		fmt.Printf("  %sChanging this rewrites server.json, verifies it, then restarts this service.%s\n\n", ColorGray, ColorReset)

		for i, h := range list {
			mark := ""
			if strings.EqualFold(h, st.Host) {
				mark = ColorGray + "  ← current" + ColorReset
			}
			custom := ""
			if hostListContainsFold(st.ExtraHosts, h) {
				custom = ColorGray + " (custom)" + ColorReset
			}
			fmt.Printf("  %s[%d]%s  %s%s%s\n", ColorBold+ColorWhite, i+1, ColorReset, h, custom, mark)
		}
		fmt.Printf("\n  %s[A]%s  %sAdd and use custom dest hostname%s\n", ColorBold+ColorWhite, ColorReset, ColorGreen, ColorReset)
		fmt.Printf("  %s[C]%s  %sClear this config's custom dest hosts%s\n", ColorBold+ColorWhite, ColorReset, ColorYellow, ColorReset)
		fmt.Printf("\n%s  ─────────────────────────────────────────%s\n", ColorGray, ColorReset)
		fmt.Printf("  %s[B]%s  %sBack%s\n", ColorBold+ColorWhite, ColorReset, ColorGray, ColorReset)

		choice := strings.ToLower(strings.TrimSpace(prompt(reader, fmt.Sprintf("\n%sChoice: %s", ColorBold+ColorYellow, ColorReset))))
		switch choice {
		case "b", "back":
			return
		case "a":
			raw := strings.TrimSpace(prompt(reader, "Custom dest hostname (URL ok): "))
			host := host_catalog.NormalizeHost(raw)
			if host == "" {
				fmt.Printf("    %s[!] Invalid hostname.%s\n", ColorYellow, ColorReset)
				continue
			}
			extras := append([]string(nil), st.ExtraHosts...)
			if !hostListContainsFold(extras, host) {
				extras = append(extras, host)
			}
			applyRealityDestChoice(serviceName, host, extras)
		case "c":
			if err := saveRealityDestPrefsForConfigPath(st.ConfigPath, host_catalog.RealityDestPrefs{PreferredHost: st.Host}); err != nil {
				fmt.Printf("    %s✗ %v%s\n", ColorRed, err, ColorReset)
				continue
			}
			if err := rewriteXrayRealityDestConfig(st.ConfigPath, st.Host, st.Dest, nil); err != nil {
				fmt.Printf("    %s✗ %v%s\n", ColorRed, err, ColorReset)
				continue
			}
			fmt.Printf("    %s✓ Custom dest hosts cleared for this config.%s\n", ColorGreen, ColorReset)
		default:
			idx, err := strconv.Atoi(choice)
			if err != nil || idx < 1 || idx > len(list) {
				fmt.Printf("    %sInvalid choice.%s\n", ColorRed, ColorReset)
				continue
			}
			applyRealityDestChoice(serviceName, list[idx-1], st.ExtraHosts)
		}
	}
}

func applyRealityDestChoice(serviceName, host string, extraHosts []string) {
	host = host_catalog.NormalizeHost(host)
	if host == "" {
		fmt.Printf("    %s[!] Invalid hostname.%s\n", ColorYellow, ColorReset)
		return
	}
	fmt.Printf("    %s[*] Checking TCP reachability, TLS certificate, and HTTPS response for %s...%s\n", ColorYellow, host, ColorReset)
	ctx, cancel := context.WithTimeout(context.Background(), destprobe.DefaultTimeout)
	defer cancel()
	res, err := destprobe.ProbeHost(ctx, host)
	if err != nil {
		fmt.Printf("    %s✗ Dest rejected: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	if err := setRealityDestForService(serviceName, res.Host, res.Address, extraHosts); err != nil {
		fmt.Printf("    %s✗ Could not apply dest: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	fmt.Printf("    %s✓ Dest set to %s (SNI %s, HTTPS %d) and service restarted.%s\n", ColorGreen, res.Address, res.Host, res.StatusCode, ColorReset)
}

func readRealityDestStateForService(serviceName string) (realityDestState, error) {
	configPath := xrayConfigPathForSNI(serviceName)
	cfg, err := readJSONConfig(configPath)
	if err != nil {
		return realityDestState{}, err
	}
	rs, err := firstRealitySettings(cfg)
	if err != nil {
		return realityDestState{}, err
	}
	meta := ensureMeta(cfg)
	dest, _ := rs["dest"].(string)
	host, _ := meta["realityDestHost"].(string)
	if strings.TrimSpace(host) == "" {
		host = host_catalog.HostFromRealityDestAddress(dest)
	}
	extras := interfaceStringSlice(meta["realityDestExtraHosts"])
	if p, err := host_catalog.LoadRealityDestPrefsFile(filepath.Dir(configPath)); err == nil {
		if p.PreferredHost != "" {
			host = p.PreferredHost
		}
		if len(p.ExtraHosts) > 0 {
			extras = p.ExtraHosts
		}
	}
	return realityDestState{
		ConfigPath: configPath,
		Host:       host_catalog.NormalizeHost(host),
		Dest:       strings.TrimSpace(dest),
		ExtraHosts: dedupeHostnamesOrdered(extras),
	}, nil
}

func setRealityDestForService(serviceName, host, dest string, extraHosts []string) error {
	if !serviceSupportsRealityDest(serviceName) {
		return fmt.Errorf("Reality dest is not supported by %s", serviceName)
	}
	configPath := xrayConfigPathForSNI(serviceName)
	if err := rewriteXrayRealityDestConfig(configPath, host, dest, extraHosts); err != nil {
		return err
	}
	if err := verifyXrayRealityDest(configPath, dest); err != nil {
		return err
	}
	if err := saveRealityDestPrefsForConfigPath(configPath, host_catalog.RealityDestPrefs{
		PreferredHost: host,
		ExtraHosts:    extraHosts,
	}); err != nil {
		return err
	}
	return restartXrayAfterRealityDestChange(serviceName, configPath, readInboundPortFromServerJSON(configPath, 443))
}

func rewriteXrayRealityDestConfig(configPath, host, dest string, extraHosts []string) error {
	host = host_catalog.NormalizeHost(host)
	if host == "" {
		return fmt.Errorf("invalid dest host")
	}
	dest = strings.TrimSpace(dest)
	if dest == "" {
		dest = host_catalog.RealityDestAddressForHost(host)
	}
	cfg, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}
	rs, err := firstRealitySettings(cfg)
	if err != nil {
		return err
	}
	meta := ensureMeta(cfg)
	sharingList := interfaceStringSlice(meta["sharingSNIs"])
	if len(sharingList) == 0 {
		sharingList = firstServerNameOnly(rs)
	}
	extraHosts = dedupeHostnamesOrdered(extraHosts)
	rs["dest"] = dest
	rs["serverNames"] = stringSliceToJSONInterfaces(host_catalog.AppendRealityDestHostsForConfig(sharingList, host, extraHosts))
	meta["sharingSNIs"] = stringSliceToJSONInterfaces(sharingList)
	meta["realityDest"] = dest
	meta["realityDestHost"] = host
	meta["realityDestExtraHosts"] = stringSliceToJSONInterfaces(extraHosts)
	meta["version"] = 1

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0644)
}

func verifyXrayRealityDest(configPath, wantDest string) error {
	cfg, err := readJSONConfig(configPath)
	if err != nil {
		return err
	}
	rs, err := firstRealitySettings(cfg)
	if err != nil {
		return err
	}
	got, _ := rs["dest"].(string)
	if strings.TrimSpace(got) != strings.TrimSpace(wantDest) {
		return fmt.Errorf("server.json verification failed: dest is %q, want %q", got, wantDest)
	}
	return nil
}

func readJSONConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func firstRealitySettings(cfg map[string]interface{}) (map[string]interface{}, error) {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return nil, fmt.Errorf("no inbounds in server.json")
	}
	ib, ok := inbounds[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid first inbound")
	}
	stream, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no streamSettings in first inbound")
	}
	rs, ok := stream["realitySettings"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("server config has no REALITY settings")
	}
	return rs, nil
}

func ensureMeta(cfg map[string]interface{}) map[string]interface{} {
	meta, ok := cfg[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{"version": 1}
		cfg[host_catalog.MetaKey] = meta
	}
	return meta
}

func firstServerNameOnly(rs map[string]interface{}) []string {
	names := interfaceStringSlice(rs["serverNames"])
	if len(names) == 0 {
		return nil
	}
	return []string{names[0]}
}

func saveRealityDestPrefsForConfigPath(configPath string, prefs host_catalog.RealityDestPrefs) error {
	return host_catalog.SaveRealityDestPrefsFile(filepath.Dir(configPath), prefs)
}
