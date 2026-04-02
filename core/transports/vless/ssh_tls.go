package vless

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

// normalizeSSHFallbackDest forces a concrete loopback (or stable) host so dest is never 0.0.0.0:port.
func normalizeSSHFallbackDest(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "127.0.0.1:22"
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		if p, e := strconv.Atoi(raw); e == nil && p > 0 && p <= 65535 {
			return net.JoinHostPort("127.0.0.1", strconv.Itoa(p))
		}
		return "127.0.0.1:22"
	}
	if host == "" {
		host = "127.0.0.1"
	} else {
		host = strings.Trim(host, "[]")
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			host = "127.0.0.1"
		}
	}
	return net.JoinHostPort(host, port)
}

// xrayFallbackDestForJSON returns an integer port when dest is loopback (Xray forwards to 127.0.0.1:port),
// otherwise the full "host:port" string. Netmod/SSH-over-TLS needs a single raw-TCP fallback without path rules.
func xrayFallbackDestForJSON(normalized string) interface{} {
	h, p, err := net.SplitHostPort(normalized)
	if err != nil {
		return normalized
	}
	portNum, err := strconv.Atoi(p)
	if err != nil || portNum < 1 || portNum > 65535 {
		return normalized
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return portNum
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return portNum
	}
	return normalized
}

// OpenSSLVerifyTLS13AES256Command returns an openssl s_client line that offers only TLS_AES_256_GCM_SHA384.
// Plain "openssl s_client -tls1_3" advertises both AES-128 and AES-256; Go's TLS 1.3 server often picks AES-128
// when both intersect, so the session shows TLS_AES_128_GCM_SHA256 even if server.json lists only AES-256.
// Use this command to verify that the peer actually negotiates AES-256.
func OpenSSLVerifyTLS13AES256Command(host string, port int, serverName string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "127.0.0.1"
	}
	sni := strings.TrimSpace(serverName)
	if port <= 0 {
		port = 443
	}
	if sni == "" {
		return fmt.Sprintf("openssl s_client -connect %s:%d -tls1_3 -ciphersuites TLS_AES_256_GCM_SHA384", host, port)
	}
	return fmt.Sprintf("openssl s_client -connect %s:%d -servername %s -tls1_3 -ciphersuites TLS_AES_256_GCM_SHA384", host, port, sni)
}

// GenerateVlessSSHDirectTLSServerConfig writes Xray VLESS + TCP + TLS with fallbacks to SSH (e.g. 127.0.0.1:22).
// sshFallbackDest must be a host:port string such as "127.0.0.1:4022".
func GenerateVlessSSHDirectTLSServerConfig(opt types.ConfigOptions, sshFallbackDest string) (string, error) {
	if opt.Port == 0 {
		opt.Port = 2053
	}
	opt.UUID = strings.TrimSpace(opt.UUID)
	if opt.UUID == "" {
		opt.UUID = utils.GenerateUUID()
	}
	host := strings.TrimSpace(opt.Sni)
	if host == "" {
		host = "localhost"
	}
	var extraSANs []string
	for _, s := range opt.ExtraSNIs {
		s = strings.TrimSpace(s)
		if s != "" {
			extraSANs = append(extraSANs, s)
		}
	}

	cfgDir := installer.GetConfigDir("ssh-tls")
	_ = os.MkdirAll(cfgDir, 0755)
	certPath := filepath.Join(cfgDir, "ssh-tls-cert.pem")
	keyPath := filepath.Join(cfgDir, "ssh-tls-key.pem")
	if err := installer.EnsureSelfSignedCertWithSAN(certPath, keyPath, host, extraSANs); err != nil {
		return "", fmt.Errorf("ssh-tls tls cert: %w", err)
	}

	certAbs, _ := filepath.Abs(certPath)
	keyAbs, _ := filepath.Abs(keyPath)

	var email string
	hostLabel := utils.SanitizeForTag(host)
	if hostLabel == "" {
		email = "TunnelBypass-ssh-tls"
	} else {
		email = fmt.Sprintf("TunnelBypass-%s", hostLabel)
	}

	dest := normalizeSSHFallbackDest(sshFallbackDest)

	serverNames := []string{host}
	for _, s := range extraSANs {
		if s != host {
			serverNames = append(serverNames, s)
		}
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			// info: easier to see fallback / errors when debugging Netmod / SSH-over-TLS.
			"loglevel": "info",
			"access":   getAbsLogPath("xray_ssh_tls_access.log"),
			"error":    getAbsLogPath("xray_ssh_tls_error.log"),
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "vless-ssh-tls-in",
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
					// Single fallback, no path: pass raw TCP to local SSH immediately (path "/" breaks some clients like Netmod).
					"fallbacks": []interface{}{
						map[string]interface{}{
							"dest": xrayFallbackDestForJSON(dest),
							"xver": 0,
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"network":  "tcp",
					"security": "tls",
					"tlsSettings": map[string]interface{}{
						// TLS 1.3: Xray uses Go crypto/tls; cipherSuites narrows server support but the negotiated
						// suite also depends on the client's ClientHello. Peers that offer AES-128+256 may still
						// end up on AES-128 unless you constrain the client (see OpenSSLVerifyTLS13AES256Command).
						"minVersion":   "1.3",
						"maxVersion":   "1.3",
						"cipherSuites": "TLS_AES_256_GCM_SHA384",
						"certificates": []interface{}{
							map[string]interface{}{
								"certificateFile": certAbs,
								"keyFile":         keyAbs,
							},
						},
						"serverNames": serverNames,
						// HTTP/1.1 only: avoids h2 framing on raw-SSH fallback after TLS.
						"alpn": []string{"http/1.1"},
					},
					"tcpSettings": map[string]interface{}{
						"header": map[string]string{"type": "none"},
					},
					"sockopt": map[string]interface{}{
						"tcpNoDelay": true,
						// TFO off: avoid edge cases proxying to loopback SSH.
						"tcpFastOpen": false,
						"v6only":      false,
					},
				},
				// Sniffing off: SSH fallback traffic must not be treated as HTTP/TLS for routing/sniff heuristics.
				"sniffing": map[string]interface{}{
					"enabled": false,
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

// GenerateVlessSSHDirectTLSClientConfig writes an optional Xray client JSON (VLESS TCP+TLS) for power users.
func GenerateVlessSSHDirectTLSClientConfig(opt types.ConfigOptions) (string, error) {
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
							// No flow / Vision — plain VLESS for optional client; SSH fallback is raw TCP after TLS.
						},
					},
				},
			},
		},
		"streamSettings": map[string]interface{}{
			"network":  "tcp",
			"security": "tls",
			"tlsSettings": map[string]interface{}{
				"serverName":    sni,
				"allowInsecure": true,
				// No uTLS fingerprint (e.g. chrome): it bundles browser cipher prefs and biases toward AES-128.
				"minVersion":   "1.3",
				"maxVersion":   "1.3",
				"cipherSuites": "TLS_AES_256_GCM_SHA384",
				"alpn":         []string{"http/1.1"},
			},
			"tcpSettings": map[string]interface{}{
				"header": map[string]string{"type": "none"},
			},
		},
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "info",
			"access":   getAbsLogPath("xray_ssh_tls_access.log"),
			"error":    getAbsLogPath("xray_ssh_tls_error.log"),
		},
		"outbounds": []interface{}{outbound},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	cfgDir := installer.GetConfigDir("ssh-tls")
	_ = os.MkdirAll(cfgDir, 0755)
	targetPath := filepath.Join(cfgDir, "client.json")
	return targetPath, os.WriteFile(targetPath, data, 0644)
}

// GenerateVlessSSHDirectTLSURL returns a vless:// sharing link for TCP+TLS (optional; primary use is SSH-over-TLS clients).
func GenerateVlessSSHDirectTLSURL(opt types.ConfigOptions) string {
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
	tag := url.QueryEscape(fmt.Sprintf("TunnelBypass-SSH-TLS-%s", utils.SanitizeForTag(sni)))
	return fmt.Sprintf(
		"vless://%s@%s:%d?encryption=none&security=tls&sni=%s&allowInsecure=true&type=tcp&headerType=none#%s",
		opt.UUID, endpoint, opt.Port,
		url.QueryEscape(sni), tag,
	)
}
