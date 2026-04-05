package vless

import (
	"encoding/json"
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

// NormalizeWSPath ensures a non-empty path starting with /.
func NormalizeWSPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// GenerateVlessWSServerConfig writes Xray VLESS + WebSocket + TLS server JSON under configs/vless-ws.
func GenerateVlessWSServerConfig(opt types.ConfigOptions) (string, error) {
	if opt.Port == 0 {
		opt.Port = 443
	}
	opt.UUID = strings.TrimSpace(opt.UUID)
	if opt.UUID == "" {
		opt.UUID = utils.GenerateUUID()
	}
	wsPath := NormalizeWSPath(opt.WSPath)

	host := strings.TrimSpace(opt.Sni)
	if host == "" {
		host = "localhost"
	}

	cfgDir := installer.GetConfigDir("vless-ws")
	_ = os.MkdirAll(cfgDir, 0755)
	certPath := filepath.Join(cfgDir, "tls-cert.pem")
	keyPath := filepath.Join(cfgDir, "tls-key.pem")
	if err := installer.EnsureSelfSignedCert(certPath, keyPath, host); err != nil {
		return "", fmt.Errorf("vless-ws tls cert: %w", err)
	}

	certAbs, _ := filepath.Abs(certPath)
	keyAbs, _ := filepath.Abs(keyPath)

	var email string
	hostLabel := utils.SanitizeForTag(host)
	if hostLabel == "" {
		email = "TunnelBypass-ws"
	} else {
		email = fmt.Sprintf("TunnelBypass-%s", hostLabel)
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			"access":   getAbsLogPath("xray_access.log"),
			"error":    getAbsLogPath("xray_error.log"),
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "vless-ws-in",
				"port":     opt.Port,
				"listen":   "0.0.0.0",
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []interface{}{
						map[string]interface{}{
							"id":    opt.UUID,
							"email": email,
							"level": 0,
						},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]interface{}{
					"network":  "ws",
					"security": "tls",
					"tlsSettings": map[string]interface{}{
						"certificates": []interface{}{
							map[string]interface{}{
								"certificateFile": certAbs,
								"keyFile":         keyAbs,
							},
						},
					},
					"wsSettings": map[string]interface{}{
						"path":            wsPath,
						"readBufferSize":  65536,
						"writeBufferSize": 65536,
					},
					"sockopt": map[string]interface{}{
						"tcpNoDelay":  true,
						"tcpFastOpen": true,
						"v6only":      false,
					},
				},
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{
				"protocol": "freedom",
				"tag":      "direct",
				"settings": map[string]interface{}{
					"domainStrategy": "UseIPv4",
				},
			},
			map[string]interface{}{
				"protocol": "blackhole",
				"tag":      "block",
			},
		},
		"routing": map[string]interface{}{
			"domainStrategy": "IPIfNonMatch",
			"rules": []interface{}{
				map[string]interface{}{
					"type":        "field",
					"ip":          []string{"geoip:private"},
					"outboundTag": "block",
				},
			},
		},
		"policy": map[string]interface{}{
			"levels": map[string]interface{}{
				"0": map[string]interface{}{
					"handshake": 4,
					"connIdle":  300,
				},
			},
		},
	}

	MergeXrayDNSIntoConfig(config)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(cfgDir, "server.json")
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", err
	}
	if err := EnsureInboundListenIPv4(targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// GenerateVlessWSClientConfig writes Xray client JSON for VLESS + WS + TLS.
func GenerateVlessWSClientConfig(opt types.ConfigOptions) (string, error) {
	endpoint := strings.TrimSpace(opt.ServerAddr)
	if opt.Host != "" {
		endpoint = opt.Host
	}
	if endpoint == "" {
		endpoint = "127.0.0.1"
	}

	sni := strings.TrimSpace(opt.Sni)
	if sni == "" {
		sni = host_catalog.RandomHost()
	}

	wsPath := NormalizeWSPath(opt.WSPath)

	outbound := map[string]interface{}{
		"protocol": "vless",
		"settings": map[string]interface{}{
			"vnext": []interface{}{
				map[string]interface{}{
					"address": endpoint,
					"port":    opt.Port,
					"users": []interface{}{
						map[string]interface{}{
							"id":         opt.UUID,
							"encryption": "none",
							"flow":       "",
						},
					},
				},
			},
		},
		"streamSettings": map[string]interface{}{
			"network":  "ws",
			"security": "tls",
			"tlsSettings": map[string]interface{}{
				"serverName":    sni,
				"allowInsecure": true,
				"fingerprint":   "chrome",
			},
			"wsSettings": map[string]interface{}{
				"path":            wsPath,
				"readBufferSize":  65536,
				"writeBufferSize": 65536,
				"headers": map[string]interface{}{
					"Host": sni,
				},
			},
		},
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "info",
			"access":   getAbsLogPath("xray_access.log"),
			"error":    getAbsLogPath("xray_error.log"),
		},
		"outbounds": []interface{}{outbound},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	cfgDir := installer.GetConfigDir("vless-ws")
	_ = os.MkdirAll(cfgDir, 0755)
	targetPath := filepath.Join(cfgDir, "client.json")
	return targetPath, os.WriteFile(targetPath, data, 0644)
}

// GenerateVlessWSURL returns a vless:// sharing link for WS+TLS.
func GenerateVlessWSURL(opt types.ConfigOptions) string {
	var endpoint string
	switch {
	case opt.ServerAddr != "":
		endpoint = opt.ServerAddr
	case opt.Host != "":
		endpoint = opt.Host
	case opt.Sni != "":
		endpoint = opt.Sni
	default:
		endpoint = host_catalog.RandomHost()
	}
	sni := strings.TrimSpace(opt.Sni)
	if sni == "" {
		sni = endpoint
	}
	wsPath := NormalizeWSPath(opt.WSPath)

	// Encode the path for use as a URL query parameter value.
	// Spaces and most special chars are encoded, but we encode '=' inside
	// any embedded query string (e.g. ?MOD=AJPERES → ?MOD%3DAJPERES) while
	// keeping '/' and '?' readable — matching what Xray/v2ray clients expect.
	pathEnc := strings.ReplaceAll(url.PathEscape(wsPath), "=", "%3D")

	tag := url.QueryEscape(fmt.Sprintf("TunnelBypass-VLESS-WS-TLS-%s", utils.SanitizeForTag(sni)))
	return fmt.Sprintf(
		"vless://%s@%s:%d?encryption=none&security=tls&sni=%s&allowInsecure=true&fp=chrome&type=ws&host=%s&path=%s&packetEncoding=xudp#%s",
		opt.UUID, endpoint, opt.Port,
		url.QueryEscape(sni), url.QueryEscape(sni), pathEnc, tag,
	)
}
