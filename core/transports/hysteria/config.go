package hysteria

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

// GenerateHysteriaConfig writes server.yaml and client.yaml paths under configs/hysteria.
func GenerateHysteriaConfig(opt types.ConfigOptions) (string, string, error) {
	if opt.Port == 0 {
		opt.Port = 443
	}

	serverNames := host_catalog.SNIsForSharing(opt.Sni, opt.ExtraSNIs)
	if len(serverNames) == 0 {
		serverNames = host_catalog.DefaultHosts()
	}
	masquerade := opt.Sni
	if masquerade == "" {
		masquerade = serverNames[0]
	}

	clientSNI := opt.Sni
	if clientSNI == "" {
		clientSNI = masquerade
	}
	obfsPassword := strings.TrimSpace(opt.ObfsPassword)
	// Salamander obfs needs PSK >= 4 bytes; empty or short clears obfs here.
	if len(obfsPassword) < 4 {
		obfsPassword = ""
	}

	configsDir := installer.GetConfigDir("hysteria")
	_ = os.MkdirAll(configsDir, 0755)
	certPath := filepath.ToSlash(filepath.Join(configsDir, "cert.pem"))
	keyPath := filepath.ToSlash(filepath.Join(configsDir, "key.pem"))
	_ = installer.EnsureSelfSignedCert(certPath, keyPath, clientSNI)

	serverConfig := map[string]interface{}{
		"listen":   fmt.Sprintf(":%d", opt.Port),
		"protocol": "udp",
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
			"type": "proxy",
			"proxy": map[string]interface{}{
				"url":         "https://" + masquerade + "/",
				"rewriteHost": true,
			},
		},
		"bandwidth": map[string]interface{}{
			"up":   "100 mbps",
			"down": "100 mbps",
		},
		"quic": map[string]interface{}{
			"initStreamReceiveWindow": 16777216,
			"maxStreamReceiveWindow":  33554432,
			"initConnReceiveWindow":   33554432,
			"maxConnReceiveWindow":    67108864,
			"maxIncomingStreams":      1024,
			"maxIdleTimeout":          "30s",
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
			"type": "udp",
			"udp": map[string]interface{}{
				"addr":    "8.8.4.4:53",
				"timeout": "4s",
			},
			"tcp": map[string]interface{}{
				"addr":    "8.8.8.8:53",
				"timeout": "4s",
			},
			"tls": map[string]interface{}{
				"addr":     "1.1.1.1:853",
				"timeout":  "10s",
				"sni":      "cloudflare-dns.com",
				"insecure": false,
			},
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
		"server": fmt.Sprintf("%s:%d", endpoint, opt.Port),
		"auth":   opt.UUID,
		"tls": map[string]interface{}{
			"sni":      clientSNI,
			"insecure": true,
		},
		"bandwidth": map[string]interface{}{
			"up":   "20 mbps",
			"down": "50 mbps",
		},
		"socks5": map[string]interface{}{
			"listen": "127.0.0.1:1080",
		},
		"http": map[string]interface{}{
			"listen": "127.0.0.1:8080",
		},
	}
	if obfsPassword != "" {
		serverConfig["obfs"] = map[string]interface{}{
			"type": "salamander",
			"salamander": map[string]interface{}{
				"password": obfsPassword,
			},
		}
		clientConfig["obfs"] = map[string]interface{}{
			"type": "salamander",
			"salamander": map[string]interface{}{
				"password": obfsPassword,
			},
		}
	}

	srvData, _ := yaml.Marshal(serverConfig)
	cliData, _ := yaml.Marshal(clientConfig)

	srvPath := filepath.Join(configsDir, "server.yaml")
	cliPath := filepath.Join(configsDir, "client.yaml")

	_ = os.WriteFile(srvPath, srvData, 0644)
	_ = os.WriteFile(cliPath, cliData, 0644)

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
	names := host_catalog.SNIsForSharing("", opt.ExtraSNIs)
	if len(names) > 0 {
		return names[0]
	}
	h := host_catalog.DefaultHosts()
	if len(h) > 0 {
		return h[0]
	}
	return ""
}

func GenerateHysteriaURLForSNI(opt types.ConfigOptions, sni string) string {
	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}

	obfsPassword := strings.TrimSpace(opt.ObfsPassword)
	obfsStr := ""
	if len(obfsPassword) >= 4 {
		obfsStr = fmt.Sprintf("&obfs=salamander&obfs-password=%s", obfsPassword)
	}
	tag := "TunnelBypass"
	return fmt.Sprintf("hysteria2://%s@%s:%d/?sni=%s&insecure=1%s#%s-%s",
		opt.UUID, endpoint, opt.Port, sni, obfsStr, tag, sni)
}

// GenerateAllSNIUrls is one URL per configured/catalog SNI (like VLESS).
func GenerateAllSNIUrls(opt types.ConfigOptions) []string {
	var urls []string
	for _, sni := range host_catalog.SNIsForSharing(opt.Sni, opt.ExtraSNIs) {
		urls = append(urls, fmt.Sprintf("# %s\n%s", sni, GenerateHysteriaURLForSNI(opt, sni)))
	}
	return urls
}

// GenerateShareLinks is labeled hysteria2:// links per SNI.
func GenerateShareLinks(opt types.ConfigOptions) []utils.ShareLink {
	var links []utils.ShareLink
	for _, sni := range host_catalog.SNIsForSharing(opt.Sni, opt.ExtraSNIs) {
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

// AppendServerName adds an SNI to server.yaml; false if already present.
func AppendServerName(configPath, newSni string) (added bool, err error) {
	if newSni == "" {
		return false, nil
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, err
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false, err
	}
	meta, ok := root[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{"version": 1, "serverNames": []interface{}{}}
		root[host_catalog.MetaKey] = meta
	}
	var names []interface{}
	if arr, ok := meta["serverNames"].([]interface{}); ok {
		names = append([]interface{}{}, arr...)
	}
	// Seed names from masquerade when metadata array is empty.
	if len(names) == 0 {
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
						names = []interface{}{host}
					}
				}
			}
		} else if masq, ok := root["masquerade"].(string); ok && masq != "" {
			// Backward compatibility with old string shape.
			names = []interface{}{masq}
		}
	}
	for _, v := range names {
		if s, ok := v.(string); ok && s == newSni {
			return false, nil
		}
	}
	names = append(names, newSni)
	meta["serverNames"] = names
	meta["version"] = 1
	out, err := yaml.Marshal(root)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(configPath, out, 0644)
}
