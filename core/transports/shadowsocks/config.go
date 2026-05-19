package shadowsocks

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

const (
	// DefaultMethod is the recommended Shadowsocks 2022 cipher.
	DefaultMethod = "2022-blake3-aes-256-gcm"
	// MetaV2rayClientCertRawKey is stored in server.json under host_catalog.MetaKey — base64 DER for v2ray-plugin certRaw= (sharing links without client-side cert files).
	MetaV2rayClientCertRawKey = "v2rayClientCertRaw"
	// MetaV2rayWSPathKey stores the v2ray-plugin WebSocket path (must match server plugin_opts).
	MetaV2rayWSPathKey = "v2rayWsPath"
)

// normalizeV2rayPluginCertKeyPaths rewrites cert=/key= values to forward slashes.
// v2ray-plugin parses plugin_opts with escape rules where '\' mangles Windows paths
// (e.g. C:\TunnelBypass\...\cert.crt becomes C:TunnelBypass...\cert.crt).
func normalizeV2rayPluginCertKeyPaths(opts string) string {
	parts := strings.Split(opts, ";")
	for i, p := range parts {
		switch {
		case strings.HasPrefix(p, "cert="):
			parts[i] = "cert=" + filepath.ToSlash(strings.TrimPrefix(p, "cert="))
		case strings.HasPrefix(p, "key="):
			parts[i] = "key=" + filepath.ToSlash(strings.TrimPrefix(p, "key="))
		}
	}
	return strings.Join(parts, ";")
}

// RandomV2rayPluginWSPath returns a non-default WebSocket path for v2ray-plugin (mode=websocket), e.g. /tb- + base64url.
func RandomV2rayPluginWSPath() string {
	b := make([]byte, 9)
	_, _ = rand.Read(b)
	return "/tb-" + base64.RawURLEncoding.EncodeToString(b)
}

// normalizeV2rayWSPath ensures a leading slash; empty stays empty.
func normalizeV2rayWSPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// NormalizeV2rayWSPath is the exported form for provision and other packages.
func NormalizeV2rayWSPath(p string) string {
	return normalizeV2rayWSPath(p)
}

// V2rayPluginPathFromServerOpts extracts path= value from server-style plugin_opts, or "" if missing.
func V2rayPluginPathFromServerOpts(pluginOpts string) string {
	pluginOpts = strings.TrimSpace(pluginOpts)
	if pluginOpts == "" {
		return ""
	}
	for _, part := range strings.Split(pluginOpts, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "path=") {
			return normalizeV2rayWSPath(strings.TrimPrefix(part, "path="))
		}
	}
	return ""
}

// v2rayPluginClientOptsFallback is used when no cert is pinned. Upstream teddysun/v2ray-plugin does not
// implement allowInsecure in tls.Config — the flag is ignored; prefer SSV2rayClientCertRaw (certRaw= in plugin_opts).
func v2rayPluginClientOptsFallback(sni, wsPath string) string {
	sni = strings.TrimSpace(sni)
	var b strings.Builder
	if sni == "" {
		b.WriteString("tls")
	} else {
		b.WriteString("tls;host=")
		b.WriteString(sni)
	}
	if p := normalizeV2rayWSPath(wsPath); p != "" && p != "/" {
		b.WriteString(";path=")
		b.WriteString(p)
	}
	b.WriteString(";allowInsecure")
	return b.String()
}

// V2rayPluginCertRawFromPEMFile reads the first PEM CERTIFICATE and returns standard base64(DER) for v2ray-plugin certRaw=.
func V2rayPluginCertRawFromPEMFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return V2rayPluginCertRawFromPEM(b)
}

// V2rayPluginCertRawFromPEM decodes the first PEM CERTIFICATE to base64(DER) for v2ray-plugin certRaw=.
func V2rayPluginCertRawFromPEM(pemData []byte) (string, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("no PEM certificate in data")
	}
	return base64.StdEncoding.EncodeToString(block.Bytes), nil
}

// buildV2rayPluginClientOpts returns SIP003 plugin_opts for the v2ray-plugin TLS client.
// Uses SSV2rayClientCertRaw (base64 DER) for certRaw= so remote clients need no local cert file.
func buildV2rayPluginClientOpts(opt types.ConfigOptions, sni string) (string, error) {
	sni = strings.TrimSpace(sni)
	var hostPart string
	if sni == "" {
		hostPart = "tls"
	} else {
		hostPart = fmt.Sprintf("tls;host=%s", sni)
	}
	pathSeg := ""
	if p := normalizeV2rayWSPath(opt.SSV2rayPluginWSPath); p != "" && p != "/" {
		pathSeg = ";path=" + p
	}
	raw := strings.TrimSpace(opt.SSV2rayClientCertRaw)
	if raw == "" {
		return v2rayPluginClientOptsFallback(sni, opt.SSV2rayPluginWSPath), nil
	}
	return hostPart + pathSeg + ";certRaw=" + raw, nil
}

// clientPluginOpts returns plugin_opts for client.json. For v2ray-plugin it matches the SIP002 link
// for the primary tunnel hostname (see GenerateShadowsocksURL for the same endpoint + password).
func clientPluginOpts(opt types.ConfigOptions) (string, error) {
	p := strings.TrimSpace(opt.SSPlugin)
	if p == "" {
		return "", nil
	}
	if strings.EqualFold(p, "v2ray-plugin") {
		return buildV2rayPluginClientOpts(opt, effectivePrimarySNI(opt))
	}
	return strings.TrimSpace(opt.SSPluginOpts), nil
}

// GenerateShadowsocksConfig writes server.json and client.json under configs/shadowsocks.
func GenerateShadowsocksConfig(opt types.ConfigOptions) (string, string, error) {
	if opt.Port == 0 {
		opt.Port = types.DefaultShadowsocksConfigTemplatePort
	}
	method := effectiveMethod(opt)
	password := effectivePassword(opt, method)
	if err := validatePasswordWithSingStack(method, password); err != nil {
		return "", "", fmt.Errorf("shadowsocks: SS2022 key rejected by sing-shadowsocks: %w", err)
	}

	configsDir := installer.GetConfigDir("shadowsocks")
	_ = os.MkdirAll(configsDir, 0755)

	sharingSNIs := host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs)
	sharingIface := make([]interface{}, len(sharingSNIs))
	for i, s := range sharingSNIs {
		sharingIface[i] = s
	}

	meta := map[string]interface{}{
		"version":     1,
		"sharingSNIs": sharingIface,
	}
	if strings.EqualFold(strings.TrimSpace(opt.SSPlugin), "v2ray-plugin") && strings.TrimSpace(opt.SSV2rayClientCertRaw) != "" {
		meta[MetaV2rayClientCertRawKey] = strings.TrimSpace(opt.SSV2rayClientCertRaw)
	}
	if strings.EqualFold(strings.TrimSpace(opt.SSPlugin), "v2ray-plugin") {
		if p := normalizeV2rayWSPath(opt.SSV2rayPluginWSPath); p != "" && p != "/" {
			meta[MetaV2rayWSPathKey] = p
		}
	}

	serverConfig := map[string]interface{}{
		"server":             "0.0.0.0",
		"server_port":        opt.Port,
		"password":           password,
		"method":             method,
		"mode":               "tcp_and_udp",
		"fast_open":          true,
		"no_delay":           true,
		"dns":                "google",
		"ipv6_first":         false,
		host_catalog.MetaKey: meta,
	}

	if opt.SSPlugin != "" {
		pluginPath := opt.SSPlugin
		// Try to resolve absolute path for the server config so it works across all OSes.
		if exe, err := installer.EnsureBinary(opt.SSPlugin); err == nil && exe != "" {
			pluginPath = exe
		}
		serverConfig["plugin"] = pluginPath
		if opt.SSPluginOpts != "" {
			opts := opt.SSPluginOpts
			if strings.EqualFold(strings.TrimSpace(opt.SSPlugin), "v2ray-plugin") {
				opts = normalizeV2rayPluginCertKeyPaths(opts)
			}
			serverConfig["plugin_opts"] = opts
		}
	}

	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}

	clientConfig := map[string]interface{}{
		"server":        endpoint,
		"server_port":   opt.Port,
		"password":      password,
		"method":        method,
		"local_address": "127.0.0.1",
		"local_port":    1080,
		"mode":          "tcp_and_udp",
		"fast_open":     true,
		"no_delay":      true,
	}

	if opt.SSPlugin != "" {
		clientConfig["plugin"] = opt.SSPlugin
		co, err := clientPluginOpts(opt)
		if err != nil {
			return "", "", fmt.Errorf("shadowsocks client plugin_opts: %w", err)
		}
		if co != "" {
			clientConfig["plugin_opts"] = co
		}
	}

	srvData, err := json.MarshalIndent(serverConfig, "", "  ")
	if err != nil {
		return "", "", err
	}
	cliData, err := json.MarshalIndent(clientConfig, "", "  ")
	if err != nil {
		return "", "", err
	}

	srvPath := filepath.Join(configsDir, "server.json")
	cliPath := filepath.Join(configsDir, "client.json")

	if err := os.WriteFile(srvPath, srvData, 0644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(cliPath, cliData, 0644); err != nil {
		return "", "", err
	}

	return srvPath, cliPath, nil
}

// GenerateShadowsocksURL builds a standard ss:// URI per SIP002 spec.
func GenerateShadowsocksURL(opt types.ConfigOptions) (string, error) {
	return GenerateShadowsocksURLForSNI(opt, effectivePrimarySNI(opt))
}

// GenerateShadowsocksURLForSNI builds a ss:// URL for a given SNI label.
func GenerateShadowsocksURLForSNI(opt types.ConfigOptions, sni string) (string, error) {
	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}
	method := effectiveMethod(opt)
	password := effectivePassword(opt, method)
	tag := fmt.Sprintf("TunnelBypass-Shadowsocks-%s", utils.SanitizeForTag(sni))

	// SIP002 format: ss://BASE64(method:password)@host:port/?plugin=plugin_name%3Bplugin_opts#tag
	userInfo := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(
		[]byte(method + ":" + password),
	)

	uri := fmt.Sprintf("ss://%s@%s:%d", userInfo, endpoint, opt.Port)

	if opt.SSPlugin != "" {
		clientOpts := opt.SSPluginOpts
		if strings.EqualFold(strings.TrimSpace(opt.SSPlugin), "v2ray-plugin") {
			var err error
			clientOpts, err = buildV2rayPluginClientOpts(opt, sni)
			if err != nil {
				return "", fmt.Errorf("shadowsocks url plugin_opts: %w", err)
			}
		}

		pluginString := opt.SSPlugin
		if clientOpts != "" {
			pluginString += ";" + clientOpts
		}
		// SIP002: entire plugin argument MUST be percent-encoded. Raw ";" in the query string is
		// parsed as a parameter separator by many clients → only "v2ray-plugin" is seen and TLS opts are lost.
		uri += "/?plugin=" + url.QueryEscape(pluginString)
	}

	uri += "#" + url.QueryEscape(tag)
	return uri, nil
}

// GenerateAllSNIUrls is one URL per primary + extra SNIs.
func GenerateAllSNIUrls(opt types.ConfigOptions) ([]string, error) {
	var urls []string
	for _, sni := range host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs) {
		if sni == "" {
			continue
		}
		u, err := GenerateShadowsocksURLForSNI(opt, sni)
		if err != nil {
			return nil, err
		}
		urls = append(urls, fmt.Sprintf("# %s\n%s", sni, u))
	}
	return urls, nil
}

// GenerateShareLinks returns labeled ss:// links per sharing SNI.
func GenerateShareLinks(opt types.ConfigOptions) ([]utils.ShareLink, error) {
	var links []utils.ShareLink
	for _, sni := range host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs) {
		if sni == "" {
			continue
		}
		u, err := GenerateShadowsocksURLForSNI(opt, sni)
		if err != nil {
			return nil, err
		}
		links = append(links, utils.ShareLink{
			Label: fmt.Sprintf("Shadowsocks (%s)", sni),
			URL:   u,
		})
	}
	return links, nil
}

// GeneratePassword returns a key compatible with shadowsocks-rust and Xray SS2022 inbounds.
// For 2022-blake3-aes-256-gcm / chacha20-poly1305: 32 random bytes, standard Base64 (same idea as openssl rand -base64 32).
// For 2022-blake3-aes-128-gcm: 16 random bytes, standard Base64 (openssl rand -base64 16).
// Legacy ciphers: returns a UUID string (passphrase-style; not used for SS2022 defaults).
//
// Xray multi-user: when inbound uses SS2022 with per-client keys, each client password must be
// "ServerPSK:UserPSK"; this helper does not build that form — set SSPassword manually for such layouts.
func GeneratePassword(method string) string {
	var keySize int
	switch strings.ToLower(method) {
	case "2022-blake3-aes-256-gcm":
		keySize = 32
	case "2022-blake3-aes-128-gcm":
		keySize = 16
	case "2022-blake3-chacha20-poly1305":
		keySize = 32
	default:
		// Legacy ciphers use passphrase-based key derivation; return a UUID.
		return utils.GenerateUUID()
	}
	key := make([]byte, keySize)
	_, _ = rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}

// ReadServerConfig reads existing server.json for status/restart operations.
func ReadServerConfig(configPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	data = utils.StripUTF8BOM(data)
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return root, nil
}

// ReadPasswordFromServerConfig extracts password from server.json.
func ReadPasswordFromServerConfig(configPath string) (string, error) {
	root, err := ReadServerConfig(configPath)
	if err != nil {
		return "", err
	}
	pw, _ := root["password"].(string)
	return pw, nil
}

// ReadMethodFromServerConfig extracts the cipher method from server.json.
func ReadMethodFromServerConfig(configPath string) (string, error) {
	root, err := ReadServerConfig(configPath)
	if err != nil {
		return "", err
	}
	m, _ := root["method"].(string)
	return m, nil
}

func effectiveMethod(opt types.ConfigOptions) string {
	if strings.TrimSpace(opt.SSMethod) != "" {
		return strings.TrimSpace(opt.SSMethod)
	}
	return DefaultMethod
}

func effectivePassword(opt types.ConfigOptions, method string) string {
	if strings.TrimSpace(opt.SSPassword) != "" {
		return strings.TrimSpace(opt.SSPassword)
	}
	// For 2022 ciphers, UUID is not valid; generate a proper key.
	return GeneratePassword(method)
}

func effectivePrimarySNI(opt types.ConfigOptions) string {
	if opt.Sni != "" {
		return opt.Sni
	}
	user := host_catalog.SharingLinkSNIs("", opt.ExtraSNIs)
	if len(user) > 0 {
		return user[0]
	}
	return host_catalog.PreferredRealityDestHostForConfig(opt.RealityDestHost, opt.RealityDestExtraHosts)
}
