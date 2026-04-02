package hysteria

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"

	"gopkg.in/yaml.v3"
)

// GenerateHysteriaConfig writes server.yaml and client.yaml paths under configs/hysteria.
func GenerateHysteriaConfig(opt types.ConfigOptions) (string, string, error) {
	if opt.Port == 0 {
		opt.Port = 443
	}

	sharingSNIs := host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs)
	if len(sharingSNIs) == 0 {
		masq := strings.TrimSpace(opt.Sni)
		if masq == "" {
			masq = host_catalog.FirstRealityDestHost()
		}
		sharingSNIs = []string{host_catalog.NormalizeHost(masq)}
	}
	serverNames := host_catalog.AppendRealityDestHosts(sharingSNIs)
	sharingIface := make([]interface{}, len(sharingSNIs))
	for i, s := range sharingSNIs {
		sharingIface[i] = s
	}
	masquerade := opt.Sni
	if masquerade == "" {
		masquerade = serverNames[0]
	}

	clientSNI := opt.Sni
	if clientSNI == "" {
		clientSNI = masquerade
	}
	obfsPassword := effectiveObfsPassword(opt)

	configsDir := installer.GetConfigDir("hysteria")
	_ = os.MkdirAll(configsDir, 0755)
	certPath := filepath.ToSlash(filepath.Join(configsDir, "cert.pem"))
	keyPath := filepath.ToSlash(filepath.Join(configsDir, "key.pem"))
	_ = installer.EnsureSelfSignedCert(certPath, keyPath, clientSNI)

	serverConfig := map[string]interface{}{
		"listen": ListenAddr(opt.Port),
		"tls": map[string]interface{}{
			"cert": certPath,
			"key":  keyPath,
			"alpn": []string{"h3"},
			"sni":  serverNames,
		},
		"auth": map[string]interface{}{
			"type":     "password",
			"password": opt.UUID,
		},
		"masquerade": map[string]interface{}{
			"type": "string",
			"string": map[string]interface{}{
				"content": "<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title></title></head><body></body></html>",
				"headers": map[string]interface{}{
					"content-type": "text/html; charset=utf-8",
				},
			},
		},
		"obfs": map[string]interface{}{
			"type": "salamander",
			"salamander": map[string]interface{}{
				"password": obfsPassword,
			},
		},
		"speedTest":      false,
		"disableUDP":     false,
		"udpIdleTimeout": "60s",
		"sniff": map[string]interface{}{
			"enable":        true,
			"timeout":       "2s",
			"rewriteDomain": false,
			"tcpPorts":      "80,443,8000-9000",
			"udpPorts":      "all",
		},
		"resolver": map[string]interface{}{
			"type": "https",
			"https": map[string]interface{}{
				"addr":     "1.1.1.1:443",
				"timeout":  "10s",
				"sni":      "cloudflare-dns.com",
				"insecure": false,
			},
		},
		"logging": map[string]interface{}{
			"level":           "error",
			"redactSensitive": true,
			"accessLog":       false,
		},
		host_catalog.MetaKey: map[string]interface{}{
			"serverNames": serverNames,
			"sharingSNIs": sharingIface,
			"version":     1,
			// Extra metadata for UI/branding purposes.
			"name": fmt.Sprintf("TunnelBypass-%s", masquerade),
		},
	}

	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}

	clientConfig := map[string]interface{}{
		"server": ClientServerAddr(endpoint, opt.Port),
		"auth":   opt.UUID,
		"tls": map[string]interface{}{
			"sni":      clientSNI,
			"insecure": true,
		},
		"obfs": map[string]interface{}{
			"type": "salamander",
			"salamander": map[string]interface{}{
				"password": obfsPassword,
			},
		},
		"socks5": map[string]interface{}{
			"listen": "127.0.0.1:1080",
		},
		"http": map[string]interface{}{
			"listen": "127.0.0.1:8080",
		},
	}

	srvData, err := yaml.Marshal(serverConfig)
	if err != nil {
		return "", "", err
	}
	cliData, err := yaml.Marshal(clientConfig)
	if err != nil {
		return "", "", err
	}

	srvPath := filepath.Join(configsDir, "server.yaml")
	cliPath := filepath.Join(configsDir, "client.yaml")

	if err := os.WriteFile(srvPath, srvData, 0644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(cliPath, cliData, 0644); err != nil {
		return "", "", err
	}
	if err := EnsureServerYAML(srvPath); err != nil {
		return "", "", err
	}

	return srvPath, cliPath, nil
}

// GenerateHysteriaURL builds a hysteria2:// URL for opt.Sni.
func GenerateHysteriaURL(opt types.ConfigOptions) string {
	return GenerateHysteriaURLForSNI(opt, effectivePrimarySNI(opt))
}

func effectivePrimarySNI(opt types.ConfigOptions) string {
	if opt.Sni != "" {
		return opt.Sni
	}
	user := host_catalog.SharingLinkSNIs("", opt.ExtraSNIs)
	if len(user) > 0 {
		return user[0]
	}
	return host_catalog.FirstRealityDestHost()
}

func GenerateHysteriaURLForSNI(opt types.ConfigOptions, sni string) string {
	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}
	pw := effectiveObfsPassword(opt)
	obfsStr := fmt.Sprintf("&obfs=salamander&obfs-password=%s", url.QueryEscape(pw))
	addr := ClientServerAddr(endpoint, opt.Port)
	frag := fmt.Sprintf("TunnelBypass-Hysteria2-%s", utils.SanitizeForTag(sni))
	// Extra query keys for sing-box / NekoBox-style clients: HTTP/3 ALPN, uTLS Chrome, TLS fragment (not record_fragment).
	// Official hysteria ignores unknown parameters per URI scheme notes; subscribers and manual JSON import may use these.
	tlsHints := "&alpn=h3&fp=chrome&fragment=1"
	return fmt.Sprintf("hysteria2://%s@%s/?sni=%s&insecure=1%s%s#%s",
		opt.UUID, addr, url.QueryEscape(sni), obfsStr, tlsHints, url.QueryEscape(frag))
}

// GenerateAllSNIUrls is one URL per primary + extra SNIs (not the full merged catalog).
func GenerateAllSNIUrls(opt types.ConfigOptions) []string {
	var urls []string
	for _, sni := range host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs) {
		if sni == "" {
			continue
		}
		urls = append(urls, fmt.Sprintf("# %s\n%s", sni, GenerateHysteriaURLForSNI(opt, sni)))
	}
	return urls
}

// GenerateShareLinks is labeled hysteria2:// links per sharing SNI.
func GenerateShareLinks(opt types.ConfigOptions) []utils.ShareLink {
	var links []utils.ShareLink
	for _, sni := range host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs) {
		if sni == "" {
			continue
		}
		links = append(links, utils.ShareLink{
			Label: fmt.Sprintf("Hysteria2 (%s)", sni),
			URL:   GenerateHysteriaURLForSNI(opt, sni),
		})
	}
	return links
}

// ReadServerNamesFromServerConfig reads SNIs from server.yaml (metadata or masquerade).
func ReadServerNamesFromServerConfig(configPath string) ([]string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	data = utils.StripUTF8BOM(data)
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	raw, ok := root[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		// Hysteria v2 shape:
		// masquerade:
		//   proxy:
		//     url: https://host/
		if mm, ok := root["masquerade"].(map[string]interface{}); ok {
			if proxy, ok := mm["proxy"].(map[string]interface{}); ok {
				if rawURL, ok := proxy["url"].(string); ok && rawURL != "" {
					host := rawURL
					host = strings.TrimPrefix(host, "https://")
					host = strings.TrimPrefix(host, "http://")
					if i := strings.Index(host, "/"); i >= 0 {
						host = host[:i]
					}
					if host != "" {
						return []string{host}, nil
					}
				}
			}
		}
		if m, ok := root["masquerade"].(string); ok && m != "" {
			return []string{m}, nil
		}
		return nil, nil
	}
	arr, _ := raw["serverNames"].([]interface{})
	var out []string
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// AppendServerName adds newSni to sharingSNIs and sets serverNames = AppendRealityDestHosts(sharingSNIs)
// (same rule as GenerateHysteriaConfig). If newSni is already in sharingSNIs, returns false.
// sharingOnly is true when newSni was already in the TLS list (e.g. a dest host) and only sharing links were extended.
func AppendServerName(configPath, newSni string) (added bool, sharingOnly bool, err error) {
	newSni = host_catalog.NormalizeHost(newSni)
	if newSni == "" {
		return false, false, nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, false, err
	}
	data = utils.StripUTF8BOM(data)
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false, false, err
	}
	meta, ok := root[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{"version": 1}
		root[host_catalog.MetaKey] = meta
	}

	sharingStrs := yamlStringSlice(meta["sharingSNIs"])
	if len(sharingStrs) == 0 {
		if h := masqueradeHostFromRoot(root); h != "" {
			sharingStrs = []string{host_catalog.NormalizeHost(h)}
		}
	}
	for _, s := range sharingStrs {
		if strings.EqualFold(strings.TrimSpace(s), newSni) {
			return false, false, nil
		}
	}

	namesBefore := yamlStringSlice(meta["serverNames"])
	if len(namesBefore) == 0 {
		if tlsMap, ok := root["tls"].(map[string]interface{}); ok {
			namesBefore = yamlStringSlice(tlsMap["sni"])
		}
	}
	inNamesBefore := hostListContainsFold(namesBefore, newSni)

	sharingStrs = append(sharingStrs, newSni)
	serverNames := host_catalog.AppendRealityDestHosts(sharingStrs)

	meta["sharingSNIs"] = stringsToYAMLIface(sharingStrs)
	meta["serverNames"] = stringsToYAMLIface(serverNames)
	meta["version"] = 1
	if tlsMap, ok := root["tls"].(map[string]interface{}); ok {
		tlsMap["sni"] = stringsToYAMLIface(serverNames)
	}
	out, err := yaml.Marshal(root)
	if err != nil {
		return false, false, err
	}
	return true, inNamesBefore, os.WriteFile(configPath, out, 0644)
}

func yamlStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, x := range arr {
		if s, ok := x.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func stringsToYAMLIface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func masqueradeHostFromRoot(root map[string]interface{}) string {
	if mm, ok := root["masquerade"].(map[string]interface{}); ok {
		if proxy, ok := mm["proxy"].(map[string]interface{}); ok {
			if rawURL, ok := proxy["url"].(string); ok && rawURL != "" {
				host := rawURL
				host = strings.TrimPrefix(host, "https://")
				host = strings.TrimPrefix(host, "http://")
				if i := strings.Index(host, "/"); i >= 0 {
					host = host[:i]
				}
				return host
			}
		}
	}
	if m, ok := root["masquerade"].(string); ok {
		return m
	}
	return ""
}

func hostListContainsFold(hosts []string, h string) bool {
	h = strings.TrimSpace(h)
	for _, x := range hosts {
		if strings.EqualFold(strings.TrimSpace(x), h) {
			return true
		}
	}
	return false
}
