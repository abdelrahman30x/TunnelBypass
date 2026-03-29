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

	"gopkg.in/yaml.v3"
)

func displayTunnelSharingLinks(serviceName string) {
	if strings.Contains(serviceName, "WireGuard") {
		fmt.Printf("\n    %sWireGuard: use the generated client config file (no URL sharing).%s\n", ColorYellow, ColorReset)
		return
	}
	if strings.Contains(serviceName, "SSH") || strings.Contains(serviceName, "SSL") {
		fmt.Printf("\n    %sThis tunnel type has no URL sharing links — use the generated instructions.%s\n", ColorYellow, ColorReset)
		return
	}

	configPath := filepath.Join(installer.GetConfigDir("vless"), "server.json")
	if strings.Contains(serviceName, "Hysteria") {
		configPath = filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
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
		names, _ := hysteria.ReadServerNamesFromServerConfig(configPath)
		detectedIP := utils.GetPublicIP()
		opt := types.ConfigOptions{
			UUID:         auth,
			ServerAddr:   detectedIP,
			Port:         port,
			ObfsPassword: obfs,
		}
		for _, sni := range names {
			u := hysteria.GenerateHysteriaURLForSNI(opt, sni)
			fmt.Printf("\n  %sHost: %s%s\n  %s%s%s\n", ColorGreen, sni, ColorReset, ColorBold, u, ColorReset)
		}
		return
	}

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

	stream := ib["streamSettings"].(map[string]interface{})
	reality := stream["realitySettings"].(map[string]interface{})

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
	pk, _ := reality["privateKey"].(string)
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
		PrivateKey:  pk,
		ShortIds:    shortIds,
		RealityDest: dest,
	}

	sns, _ := reality["serverNames"].([]interface{})
	for _, s := range sns {
		sni := s.(string)
		opt.Sni = sni
		u := vless.GenerateVlessURL(opt)
		fmt.Printf("\n  %sHost: %s%s\n  %s%s%s\n", ColorGreen, sni, ColorReset, ColorBold, u, ColorReset)
	}
}

func addNewSNI(reader *bufio.Reader) {
	addNewSNIForService(reader, findInstalledService())
}

func addNewSNIForService(reader *bufio.Reader, sName string) {
	newSni := prompt(reader, "New tunnel hostname (SNI): ")
	if newSni == "" {
		return
	}

	if strings.Contains(sName, "WireGuard") || strings.Contains(sName, "SSH") || strings.Contains(sName, "SSL") {
		fmt.Printf("    %sNot applicable for this tunnel type.%s\n", ColorYellow, ColorReset)
		return
	}

	if strings.Contains(sName, "Hysteria") {
		configPath := filepath.Join(installer.GetConfigDir("hysteria"), "server.yaml")
		added, err := hysteria.AppendServerName(configPath, newSni)
		if err != nil {
			fmt.Printf("    %s✗ %v%s\n", ColorRed, err, ColorReset)
			return
		}
		if !added {
			fmt.Printf("    %sThat hostname is already listed.%s\n", ColorYellow, ColorReset)
			return
		}
		fmt.Printf("    %s✓ Added '%s' to tunnel host list.%s\n", ColorGreen, newSni, ColorReset)
		fmt.Println("    [*] Restarting tunnel service...")
		_ = hysteria.InstallHysteriaService(sName, configPath, readListenPort(configPath, 443))
		return
	}

	configPath := filepath.Join(installer.GetConfigDir("vless"), "server.json")
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
	stream := ib["streamSettings"].(map[string]interface{})
	reality := stream["realitySettings"].(map[string]interface{})
	sns, _ := reality["serverNames"].([]interface{})
	for _, s := range sns {
		if s.(string) == newSni {
			fmt.Println("    [!] Hostname already exists.")
			return
		}
	}
	reality["serverNames"] = append(sns, newSni)
	newData, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(configPath, newData, 0644)
	fmt.Printf("    %s✓ Added '%s'.%s\n", ColorGreen, newSni, ColorReset)
	fmt.Println("    [*] Restarting tunnel service...")
	_ = vless.InstallXrayService(sName, configPath, readInboundPort(cfg, 443))
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
	portStr := strings.TrimPrefix(listen, ":")
	p, _ := strconv.Atoi(portStr)
	if p > 0 {
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
