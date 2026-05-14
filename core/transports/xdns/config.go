package xdns

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/transports/vless"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
)

func buildKCPSettings(opt types.ConfigOptions) map[string]interface{} {
	return map[string]interface{}{
		"mtu":              opt.KCPMTU,
		"tti":              opt.KCPTTI,
		"uplinkCapacity":   5,
		"downlinkCapacity": 100,
		"congestion":       false,
		"readBufferSize":   1,
		"writeBufferSize":  1,
	}
}

func buildFinalMask(opt types.ConfigOptions) map[string]interface{} {
	return map[string]interface{}{
		"udp": []interface{}{
			map[string]interface{}{
				"type": "mkcp-aes128gcm",
				"settings": map[string]interface{}{
					"password": opt.KCPSeed,
				},
			},
		},
	}
}

// GenerateServerConfig creates the Xray server JSON for VLESS + mKCP on UDP port 53.
func GenerateServerConfig(opt types.ConfigOptions) (string, error) {
	email := fmt.Sprintf("TunnelBypass-xdns-%s", utils.SanitizeForTag(opt.Host))
	if opt.Host == "" {
		email = fmt.Sprintf("TunnelBypass-xdns-%s", utils.SanitizeForTag(opt.ServerAddr))
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			"access":   getAbsLogPath("xray_access.log"),
			"error":    getAbsLogPath("xray_error.log"),
		},
		"_tunnelbypass": map[string]interface{}{
			"version": 1,
			"seed":    opt.KCPSeed,
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "vless-in",
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
					"network":   "kcp",
					"kcpSettings": buildKCPSettings(opt),
					"finalmask": buildFinalMask(opt),
					"sockopt": map[string]interface{}{
						"mark":   0,
						"v6only": false,
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
			"rules":          []interface{}{},
		},
	}

	vless.MergeXrayDNSIntoConfig(config)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	configsDir := installer.GetConfigDir("xdns")
	_ = os.MkdirAll(configsDir, 0755)

	targetPath := filepath.Join(configsDir, "server.json")
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", err
	}
	if err := vless.EnsureInboundListenIPv4(targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// GenerateClientConfig creates the Xray client JSON for VLESS + mKCP on UDP port 53.
func GenerateClientConfig(opt types.ConfigOptions) (string, error) {
	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "info",
		},
		"inbounds":  vless.BuildClientInbounds(),
		"outbounds": []interface{}{
			map[string]interface{}{
				"protocol": "vless",
				"settings": map[string]interface{}{
					"vnext": []interface{}{
						map[string]interface{}{
							"address": endpoint,
							"port":    opt.Port,
							"users": []interface{}{
								map[string]string{
									"id":         opt.UUID,
									"encryption": "none",
									"flow":       "",
								},
							},
						},
					},
				},
				"streamSettings": map[string]interface{}{
					"network":     "kcp",
					"kcpSettings": buildKCPSettings(opt),
					"finalmask":   buildFinalMask(opt),
				},
				"tag": "proxy",
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	configsDir := installer.GetConfigDir("xdns")
	_ = os.MkdirAll(configsDir, 0755)

	targetPath := filepath.Join(configsDir, "client.json")
	err = os.WriteFile(targetPath, data, 0644)
	return targetPath, err
}

func getAbsLogPath(name string) string {
	logsDir := installer.GetLogsDir()
	_ = os.MkdirAll(logsDir, 0755)
	return filepath.Join(logsDir, name)
}
