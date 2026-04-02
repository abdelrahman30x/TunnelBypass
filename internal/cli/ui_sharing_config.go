package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/hysteria"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"

	"gopkg.in/yaml.v3"
)

func displayTunnelSharingLinks(serviceName string) {
	if strings.Contains(serviceName, "WireGuard") {
		fmt.Printf("\n    %sWireGuard: use the generated client config file (no URL sharing).%s\n", ColorYellow, ColorReset)
		return
	}
	if strings.Contains(serviceName, "GRPC") {
		printConfiguredTunnelHostnames(serviceName)
		displayGRPCSharingLinks()
		return
	}
	if strings.Contains(serviceName, "SSH-TLS") {
		printConfiguredTunnelHostnames(serviceName)
		displaySSHTLSSharingLinks()
		return
	}
	if strings.Contains(serviceName, "SSH") || strings.Contains(serviceName, "SSL") {
		fmt.Printf("\n    %sThis tunnel type has no URL sharing links — use the generated instructions.%s\n", ColorYellow, ColorReset)
		return
	}

	if strings.Contains(serviceName, "Hysteria") {
		configPath := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			fmt.Printf("    ✗ Error reading config: %v\n", err)
			return
		}
		data = utils.StripUTF8BOM(data)
		var cfg map[string]interface{}
		if yaml.Unmarshal(data, &cfg) != nil {
			fmt.Println("    ✗ Invalid config")
			return
		}
		listen, _ := cfg["listen"].(string)
		portStr := strings.TrimPrefix(listen, ":")
		port, _ := strconv.Atoi(portStr)
		if port <= 0 {
			port = 8443
		}
		authMap, _ := cfg["auth"].(map[string]interface{})
		auth, _ := authMap["password"].(string)

		obfs := ""
		if obfsMap, ok := cfg["obfs"].(map[string]interface{}); ok {
			if salam, ok := obfsMap["salamander"].(map[string]interface{}); ok {
				obfs, _ = salam["password"].(string)
			} else {
				obfs, _ = obfsMap["password"].(string)
			}
		}
		names, _ := sharingTunnelHostnamesFromConfig(serviceName)
		if len(names) == 0 {
			names, _ = hysteria.ReadServerNamesFromServerConfig(configPath)
			names = dedupeHostnamesOrdered(names)
			if len(names) > 0 {
				names = []string{names[0]}
			}
		}
		detectedIP := utils.GetPublicIP()
		opt := types.ConfigOptions{
			UUID:         auth,
			ServerAddr:   detectedIP,
			Port:         port,
			ObfsPassword: obfs,
		}
		for _, sni := range names {
			u := hysteria.GenerateHysteriaURLForSNI(opt, sni)
			fmt.Printf("\n  %sSNI:%s %s%s\n  %s%s%s\n", ColorGreen, ColorReset, sni, ColorReset, ColorBold, u, ColorReset)
		}
		return
	}

	printConfiguredTunnelHostnames(serviceName)

	configPath := xrayConfigPathForSNI(serviceName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("    ✗ Error reading config: %v\n", err)
		return
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		fmt.Println("    ✗ Invalid config JSON")
		return
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return
	}
	ib := inbounds[0].(map[string]interface{})
	portf, _ := ib["port"].(float64)
	port := int(portf)
	if port <= 0 {
		port = 443
	}

	stream, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		fmt.Printf("    %s✗ No streamSettings in config.%s\n", ColorRed, ColorReset)
		return
	}
	reality, ok := stream["realitySettings"].(map[string]interface{})
	if !ok {
		fmt.Printf("    %s[!] This config has no REALITY inbound (e.g. WebSocket-only). Check configs folder for client/sharing files.%s\n", ColorYellow, ColorReset)
		return
	}

	sidArr, _ := reality["shortIds"].([]interface{})
	var shortIds []string
	for _, s := range sidArr {
		shortIds = append(shortIds, s.(string))
	}
	for i := 0; i < len(shortIds); i++ {
		if shortIds[i] == "" {
			shortIds = append(shortIds[:i], shortIds[i+1:]...)
			break
		}
	}
	privK, _ := reality["privateKey"].(string)
	pubK, _ := reality["publicKey"].(string)
	if strings.TrimSpace(pubK) == "" && strings.TrimSpace(privK) != "" {
		if derived, err := utils.X25519PublicKeyFromPrivate(privK); err == nil {
			pubK = derived
		}
	}
	dest, _ := reality["dest"].(string)

	settings := ib["settings"].(map[string]interface{})
	clients := settings["clients"].([]interface{})
	cli := clients[0].(map[string]interface{})
	uuid, _ := cli["id"].(string)

	detectedIP := utils.GetPublicIP()
	opt := types.ConfigOptions{
		UUID:        uuid,
		ServerAddr:  detectedIP,
		Port:        port,
		PrivateKey:  privK,
		PublicKey:   pubK,
		ShortIds:    shortIds,
		RealityDest: dest,
	}

	sniList, _ := sharingTunnelHostnamesFromConfig(serviceName)
	if len(sniList) == 0 {
		sns, _ := reality["serverNames"].([]interface{})
		for _, s := range sns {
			if str, ok := s.(string); ok && strings.TrimSpace(str) != "" {
				sniList = append(sniList, strings.TrimSpace(str))
				break
			}
		}
	}
	for _, sni := range sniList {
		opt.Sni = sni
		u := vless.GenerateVlessURL(opt)
		fmt.Printf("\n  %sServer:%s %s:%d\n", ColorGreen+ColorBold, ColorReset, detectedIP, port)
		fmt.Printf("  %sSNI:%s %s\n", ColorGray, ColorReset, sni)
		fmt.Printf("  %s%s%s\n", ColorBold, u, ColorReset)
	}
}

// displaySSHTLSSharingLinks prints optional VLESS TCP+TLS URL from ssh-tls server.json (primary use is SSH-over-TLS apps).
func displaySSHTLSSharingLinks() {
	configPath := filepath.Join(installer.GetConfigDir("ssh-tls"), "server.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("    %s✗ Cannot read %s: %v%s\n", ColorRed, configPath, err, ColorReset)
		return
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		fmt.Println("    ✗ Invalid config JSON")
		return
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return
	}
	ib, ok := inbounds[0].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ Invalid inbound in config")
		return
	}
	portf, _ := ib["port"].(float64)
	port := int(portf)
	if port <= 0 {
		port = 443
	}
	settings, ok := ib["settings"].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ Invalid inbound settings")
		return
	}
	clients, ok := settings["clients"].([]interface{})
	if !ok || len(clients) == 0 {
		fmt.Println("    ✗ No VLESS clients in config")
		return
	}
	cli, ok := clients[0].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ Invalid client entry")
		return
	}
	uuid, _ := cli["id"].(string)
	stream, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ No streamSettings")
		return
	}
	tls, ok := stream["tlsSettings"].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ No tlsSettings (not an SSH-TLS server config?)")
		return
	}
	sns, _ := tls["serverNames"].([]interface{})
	sni := ""
	if len(sns) > 0 {
		sni, _ = sns[0].(string)
	}
	detectedIP := utils.GetPublicIP()
	opt := types.ConfigOptions{
		UUID:       uuid,
		ServerAddr: detectedIP,
		Port:       port,
		Sni:        sni,
	}
	u := vless.GenerateVlessSSHDirectTLSURL(opt)
	fmt.Printf("\n  %sSSH + TLS (direct)%s — optional VLESS (TCP+TLS) link:\n", ColorGreen, ColorReset)
	fmt.Printf("  %s%s%s\n", ColorBold, u, ColorReset)
	fmt.Printf("  %sMost users connect with Netmod/HTTP Custom using server, port, SNI, and SSH credentials (see wizard summary).%s\n", ColorGray, ColorReset)
	fmt.Printf("  %sVerify TLS 1.3 negotiated cipher (force AES-256 in openssl client):%s\n", ColorGray, ColorReset)
	fmt.Printf("  %s%s%s\n", ColorGray, vless.OpenSSLVerifyTLS13AES256Command(detectedIP, port, sni), ColorReset)
}

func displayGRPCSharingLinks() {
	configPath := filepath.Join(installer.GetConfigDir("vless-grpc"), "server.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("    %s✗ Cannot read %s: %v%s\n", ColorRed, configPath, err, ColorReset)
		return
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		fmt.Println("    ✗ Invalid config JSON")
		return
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return
	}
	ib, ok := inbounds[0].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ Invalid inbound in config")
		return
	}
	portf, _ := ib["port"].(float64)
	port := int(portf)
	if port <= 0 {
		port = 443
	}
	settings, ok := ib["settings"].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ Invalid inbound settings")
		return
	}
	clients, ok := settings["clients"].([]interface{})
	if !ok || len(clients) == 0 {
		fmt.Println("    ✗ No VLESS clients in config")
		return
	}
	cli, ok := clients[0].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ Invalid client entry")
		return
	}
	uuid, _ := cli["id"].(string)
	stream, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		fmt.Println("    ✗ No streamSettings")
		return
	}
	sec, _ := stream["security"].(string)
	grpcSettings, _ := stream["grpcSettings"].(map[string]interface{})
	svcFromCfg, _ := grpcSettings["serviceName"].(string)

	var sni string
	var publicKey string
	var shortIds []string

	if sec == "reality" {
		rs, ok := stream["realitySettings"].(map[string]interface{})
		if !ok {
			fmt.Println("    ✗ No realitySettings (expect REALITY + gRPC server config)")
			return
		}
		pkPub, _ := rs["publicKey"].(string)
		publicKey = pkPub
		privK, _ := rs["privateKey"].(string)
		if strings.TrimSpace(publicKey) == "" && strings.TrimSpace(privK) != "" {
			if derived, err := utils.X25519PublicKeyFromPrivate(privK); err == nil {
				publicKey = derived
			}
		}
		if sidArr, ok := rs["shortIds"].([]interface{}); ok {
			for _, s := range sidArr {
				if sstr, ok := s.(string); ok {
					shortIds = append(shortIds, sstr)
				}
			}
		}
		sns, _ := rs["serverNames"].([]interface{})
		if len(sns) > 0 {
			sni, _ = sns[0].(string)
		}
	} else {
		// Legacy TLS+gRPC configs (before REALITY migration)
		tls, ok := stream["tlsSettings"].(map[string]interface{})
		if !ok {
			fmt.Println("    ✗ No tlsSettings / realitySettings (not a gRPC server config?)")
			return
		}
		sns, _ := tls["serverNames"].([]interface{})
		if len(sns) > 0 {
			sni, _ = sns[0].(string)
		}
	}

	detectedIP := utils.GetPublicIP()
	opt := types.ConfigOptions{
		UUID:       uuid,
		ServerAddr: detectedIP,
		Port:       port,
		Sni:        sni,
		PublicKey:  publicKey,
		ShortIds:   shortIds,
	}

	var u string
	if sec == "reality" {
		u = vless.GenerateVlessGRPCURL(opt)
	} else {
		if b, err := os.ReadFile(filepath.Join(installer.GetConfigDir("vless-grpc"), "sharing-link.txt")); err == nil {
			lines := strings.Split(string(utils.StripUTF8BOM(b)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "vless://") {
					u = line
					break
				}
			}
		}
	}

	qrPath := filepath.Join(installer.GetConfigDir("vless-grpc"), "qr-vless-grpc.png")
	if u != "" {
		_ = utils.SaveQRCodePNG(qrPath, u, 320)
	}

	if svcFromCfg == "" {
		svcFromCfg = vless.GRPCServiceNameFromSNI(sni)
	}
	fmt.Printf("\n  %sREALITY + gRPC%s — Elite / HTTP-2 gRPC path\n", ColorBold+ColorCyan, ColorReset)
	fmt.Printf("  %sServer:%s %s:%d\n", ColorGreen+ColorBold, ColorReset, detectedIP, port)
	fmt.Printf("  %sService Name:%s %s\n", ColorGray, ColorReset, svcFromCfg)
	fmt.Printf("  %sSNI:%s %s\n", ColorGray, ColorReset, sni)
	if u != "" {
		fmt.Printf("\n  %sSharing link:%s\n  %s%s%s\n", ColorGreen, ColorReset, ColorBold, u, ColorReset)
		fmt.Printf("\n  %sQR code saved:%s %s%s%s\n", ColorGray, ColorReset, ColorBold, qrPath, ColorReset)
	}
	if sec == "reality" {
		fmt.Printf("\n  %sTransport:%s REALITY + gRPC  |  %sMode:%s multi-stream\n", ColorGray, ColorReset, ColorGray, ColorReset)
	} else {
		fmt.Printf("\n  %sLegacy TLS+gRPC server.json — run Setup again to regenerate REALITY + gRPC.%s\n", ColorYellow, ColorReset)
		if u == "" {
			fmt.Printf("  %s(No vless:// link; regenerate configs.)%s\n", ColorGray, ColorReset)
		}
		fmt.Printf("\n  %sALPN: h2  |  Mode: multi-stream%s\n", ColorGray, ColorReset)
		fmt.Printf("  %sVerify TLS cipher:%s\n  %s%s%s\n",
			ColorGray, ColorReset, ColorGray,
			vless.OpenSSLVerifyTLS13AES256Command(detectedIP, port, sni), ColorReset)
	}
}

func addNewSNI(reader *bufio.Reader) {
	addNewSNIForService(reader, findInstalledService())
}

func addNewSNIForService(reader *bufio.Reader, sName string) {
	printConfiguredTunnelHostnames(sName)

	raw := strings.TrimSpace(prompt(reader, "\n    New tunnel hostname (SNI, URL ok): "))
	newSni := host_catalog.NormalizeHost(raw)
	if newSni == "" {
		return
	}
	if raw != newSni && (strings.Contains(raw, "://") || strings.Contains(raw, "/")) {
		fmt.Printf("    %s→ %s%s\n", ColorGray, newSni, ColorReset)
	}

	if strings.Contains(sName, "SSH-TLS") {
		fmt.Printf("    %sSNI is tied to the TLS certificate. Re-run Setup (tunnel mode 6) to change hostname and regenerate certs.%s\n", ColorYellow, ColorReset)
		return
	}
	if strings.Contains(sName, "WireGuard") || strings.Contains(sName, "SSH") || strings.Contains(sName, "SSL") {
		fmt.Printf("    %sNot applicable for this tunnel type.%s\n", ColorYellow, ColorReset)
		return
	}

	sharingNow, _ := sharingTunnelHostnamesFromConfig(sName)
	if hostListContainsFold(sharingNow, newSni) {
		fmt.Printf("    %s[!] That hostname is already in this tunnel's sharing links list (duplicates are ignored).%s\n", ColorYellow, ColorReset)
		printConfiguredTunnelHostnames(sName)
		return
	}

	if strings.Contains(sName, "Hysteria") {
		configPath := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		added, shareOnly, err := hysteria.AppendServerName(configPath, newSni)
		if err != nil {
			fmt.Printf("    %s✗ %v%s\n", ColorRed, err, ColorReset)
			return
		}
		if !added {
			fmt.Printf("    %sThat hostname is already in sharing links.%s\n", ColorYellow, ColorReset)
			printConfiguredTunnelHostnames(sName)
			return
		}
		if shareOnly {
			fmt.Printf("    %s✓ Added '%s' to sharing links (already in server host list).%s\n", ColorGreen, newSni, ColorReset)
		} else {
			fmt.Printf("    %s✓ Added '%s' to tunnel host list.%s\n", ColorGreen, newSni, ColorReset)
		}
		fmt.Println("    [*] Restarting tunnel service...")
		_ = hysteria.InstallHysteriaService(sName, configPath, readListenPort(configPath, 443))
		printConfiguredTunnelHostnames(sName)
		return
	}

	configPath := xrayConfigPathForSNI(sName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("    %s✗ Error reading config: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		fmt.Println("    ✗ Invalid config JSON")
		return
	}
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		fmt.Printf("    %s✗ No inbounds in config.%s\n", ColorRed, ColorReset)
		return
	}
	ib := inbounds[0].(map[string]interface{})
	stream, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		fmt.Printf("    %s✗ No streamSettings.%s\n", ColorRed, ColorReset)
		return
	}

	meta, ok := cfg[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{"version": 1}
		cfg[host_catalog.MetaKey] = meta
	}
	sharingList := interfaceStringSlice(meta["sharingSNIs"])

	sharingOnly := false
	if rs, ok := stream["realitySettings"].(map[string]interface{}); ok {
		if hostListContainsFold(sharingList, newSni) {
			fmt.Printf("    %s[!] That hostname is already in this tunnel's sharing links list (duplicates are ignored).%s\n", ColorYellow, ColorReset)
			printConfiguredTunnelHostnames(sName)
			return
		}
		namesBefore := interfaceStringSlice(rs["serverNames"])
		sharingOnly = hostListContainsFold(namesBefore, newSni)
		sharingList = append(sharingList, newSni)
		sharingList = dedupeHostnamesOrdered(sharingList)
		rs["serverNames"] = stringSliceToJSONInterfaces(host_catalog.AppendRealityDestHosts(sharingList))
		meta["sharingSNIs"] = stringSliceToJSONInterfaces(sharingList)
	} else if tls, ok := stream["tlsSettings"].(map[string]interface{}); ok {
		// VLESS-WS (and TLS-only): extend serverNames; cert must cover the name for clients to succeed.
		rawNames := interfaceStringSlice(tls["serverNames"])
		sharingOnly = hostListContainsFold(rawNames, newSni)
		if !sharingOnly {
			rawNames = append(rawNames, newSni)
			rawNames = dedupeHostnamesOrdered(rawNames)
			tls["serverNames"] = stringSliceToJSONInterfaces(rawNames)
		}
		sharingList = append(sharingList, newSni)
		sharingList = dedupeHostnamesOrdered(sharingList)
		meta["sharingSNIs"] = stringSliceToJSONInterfaces(sharingList)
	} else {
		fmt.Printf("    %s✗ This config has no Reality/TLS host list to extend.%s\n", ColorRed, ColorReset)
		return
	}

	newData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Printf("    %s✗ %v%s\n", ColorRed, err, ColorReset)
		return
	}
	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		fmt.Printf("    %s✗ Cannot save config: %v%s\n", ColorRed, err, ColorReset)
		return
	}
	if sharingOnly {
		fmt.Printf("    %s✓ Added '%s' to sharing links (already in Reality/TLS serverNames).%s\n", ColorGreen, newSni, ColorReset)
	} else {
		fmt.Printf("    %s✓ Added '%s' to server host list and sharing links.%s\n", ColorGreen, newSni, ColorReset)
	}
	fmt.Println("    [*] Restarting tunnel service...")
	_ = vless.InstallXrayService(sName, configPath, readInboundPort(cfg, 443))
	printConfiguredTunnelHostnames(sName)
}

func readListenPort(configPath string, defaultPort int) int {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultPort
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if yaml.Unmarshal(data, &cfg) != nil {
		return defaultPort
	}
	listen, _ := cfg["listen"].(string)
	if p := hysteria.ParseListenFirstPort(listen); p > 0 {
		return p
	}
	return defaultPort
}

func readInboundPort(cfg map[string]interface{}, defaultPort int) int {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		return defaultPort
	}
	ib := inbounds[0].(map[string]interface{})
	pf, ok := ib["port"].(float64)
	if ok && int(pf) > 0 {
		return int(pf)
	}
	return defaultPort
}

func readInboundPortFromServerJSON(path string, defaultPort int) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultPort
	}
	data = utils.StripUTF8BOM(data)
	var cfg map[string]interface{}
	if json.Unmarshal(data, &cfg) != nil {
		return defaultPort
	}
	return readInboundPort(cfg, defaultPort)
}

func promptUserSelection(reader *bufio.Reader) (string, bool) {
	users, err := installer.ListWindowsUsers()
	if err != nil || len(users) == 0 {
		fmt.Printf("    %s[!] Warning: Could not list system users: %v%s\n", ColorYellow, err, ColorReset)
		u := prompt(reader, fmt.Sprintf("    %sSSH Username [user]: %s", ColorBold, ColorReset))
		if u == "" {
			return "user", true
		}
		return u, true
	}

	fmt.Printf("\n    %s[ SELECT SSH USER ]%s\n", ColorBold, ColorReset)
	for i, u := range users {
		fmt.Printf("    %d) %s\n", i+1, u)
	}
	fmt.Printf("    n) Create New User\n")

	choice := prompt(reader, "    Select User [1-"+strconv.Itoa(len(users))+" or n]: ")
	if strings.ToLower(choice) == "n" {
		u := prompt(reader, "    Enter New Username: ")
		if u == "" {
			return "user", true
		}
		return u, true
	}

	idx, _ := strconv.Atoi(choice)
	if idx > 0 && idx <= len(users) {
		return users[idx-1], false
	}

	fmt.Printf("    %s[!] Invalid choice, defaulting to: %s%s\n", ColorYellow, users[0], ColorReset)
	return users[0], false
}
