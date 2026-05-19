package vless

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/network"
	"tunnelbypass/internal/utils"
	"tunnelbypass/tools/host_catalog"
)

// Minimal JSON shape for Xray log, inbounds, and outbounds.
type XrayConfig struct {
	Log       interface{}   `json:"log"`
	Inbounds  []interface{} `json:"inbounds"`
	Outbounds []interface{} `json:"outbounds"`
}

func stringSliceToIface(in []string) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Xray Reality server config JSON under configs/vless.
func GenerateServerConfig(opt types.ConfigOptions) (string, error) {
	if opt.RealityDest == "" {
		opt.RealityDest = host_catalog.DefaultRealityDestAddressForConfig(opt.RealityDestHost, opt.RealityDestExtraHosts)
	}
	if opt.RealityDestHost == "" {
		opt.RealityDestHost = host_catalog.HostFromRealityDestAddress(opt.RealityDest)
	}

	sharingSNIs := host_catalog.RealitySharingSNIs(opt.Sni, opt.ExtraSNIs)
	serverNames := host_catalog.AppendRealityDestHostsForConfig(sharingSNIs, opt.RealityDestHost, opt.RealityDestExtraHosts)

	sids := opt.ShortIds
	if len(sids) == 0 {
		sids = []string{"8d2c", "74d3", "3bd4", ""}
	}

	var email string
	hostLabel := opt.Sni
	if hostLabel == "" && len(serverNames) > 0 {
		hostLabel = serverNames[0]
	}
	hostLabel = utils.SanitizeForTag(hostLabel)
	if hostLabel == "" {
		uuidPart := opt.UUID
		if len(uuidPart) > 8 {
			uuidPart = uuidPart[:8]
		}
		email = fmt.Sprintf("TunnelBypass-%s", uuidPart)
	} else {
		email = fmt.Sprintf("TunnelBypass-%s", hostLabel)
	}

	sharingIface := make([]interface{}, len(sharingSNIs))
	for i, s := range sharingSNIs {
		sharingIface[i] = s
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			"access":   getAbsLogPath("xray_access.log"),
			"error":    getAbsLogPath("xray_error.log"),
		},
		host_catalog.MetaKey: map[string]interface{}{
			"version":               1,
			"sharingSNIs":           sharingIface,
			"realityDest":           opt.RealityDest,
			"realityDestHost":       opt.RealityDestHost,
			"realityDestExtraHosts": stringSliceToIface(opt.RealityDestExtraHosts),
		},
		"api": map[string]interface{}{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService", "LoggerService"},
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
							"flow":  "xtls-rprx-vision",
							"email": email,
							"level": 0,
						},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]interface{}{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]interface{}{
						"show":        false,
						"dest":        opt.RealityDest,
						"xver":        0,
						"privateKey":  opt.PrivateKey,
						"shortIds":    sids,
						"serverNames": serverNames,
						"publicKey":   opt.PublicKey,
					},
					"tcpSettings": map[string]interface{}{
						"header": map[string]string{"type": "none"},
					},
					"sockopt": map[string]interface{}{
						"tcpNoDelay":  true,
						"tcpFastOpen": true,
						"mark":        0,
						"v6only":      false,
					},
				},
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
				},
			},
			map[string]interface{}{
				"tag":      "api-in",
				"listen":   "127.0.0.1",
				"port":     10085,
				"protocol": "dokodemo-door",
				"settings": map[string]interface{}{
					"address": "127.0.0.1",
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
				"streamSettings": map[string]interface{}{
					"sockopt": map[string]interface{}{
						"tcpNoDelay":  true,
						"tcpFastOpen": true,
					},
				},
			},
			map[string]interface{}{
				"protocol": "blackhole",
				"tag":      "block",
			},
		},
		"routing": map[string]interface{}{
			"domainStrategy": "IPIfNonMatch",
			"rules": append(
				[]interface{}{
					map[string]interface{}{
						"type":        "field",
						"inboundTag":  []string{"api-in"},
						"outboundTag": "api",
					},
				},
				network.BuildXrayPrivateRule(opt.NetworkProfile.PrivateRouting())...,
			),
		},
		"policy": map[string]interface{}{
			"levels": map[string]interface{}{
				"0": map[string]interface{}{
					"handshakeMSeconds": 5000,
					"connIdle":          300,
					"uplinkOnly":        0,
					"downlinkOnly":      0,
					"bufferSize":        2000,
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
			"system": map[string]interface{}{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
	}

	MergeXrayDNSIntoConfig(config)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	configsDir := installer.GetConfigDir("vless")
	_ = os.MkdirAll(configsDir, 0755)
	_ = host_catalog.SaveRealityDestPrefsFile(configsDir, host_catalog.RealityDestPrefs{
		PreferredHost: opt.RealityDestHost,
		ExtraHosts:    opt.RealityDestExtraHosts,
	})

	fileName := "server.json"
	targetPath := filepath.Join(configsDir, fileName)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", err
	}
	if err := EnsureInboundListenIPv4(targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// Xray client config JSON (Reality / grpc / tcp) under configs/vless.
func GenerateClientConfig(opt types.ConfigOptions) (string, error) {
	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	}

	outbound := map[string]interface{}{
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
		"streamSettings": map[string]interface{}{},
	}

	stream := outbound["streamSettings"].(map[string]interface{})

	if opt.Transport == "reality" {
		outbound["settings"].(map[string]interface{})["vnext"].([]interface{})[0].(map[string]interface{})["users"].([]interface{})[0].(map[string]string)["flow"] = "xtls-rprx-vision"
		stream["network"] = "tcp"
		stream["security"] = "reality"

		sid := "12345678"
		if len(opt.ShortIds) > 0 {
			sid = opt.ShortIds[0]
		}
		names := host_catalog.ServerNamesForVLESSConfig(opt.Sni, opt.ExtraSNIs, opt.RealityDestHost, opt.RealityDestExtraHosts)
		clientSNI := opt.Sni
		if clientSNI == "" && len(names) > 0 {
			clientSNI = names[0]
		}

		stream["realitySettings"] = map[string]interface{}{
			"publicKey":   opt.PublicKey,
			"shortId":     sid,
			"sni":         clientSNI,
			"fingerprint": "chrome",
			"serverName":  clientSNI,
		}
	} else if opt.Transport == "grpc" {
		stream["network"] = "grpc"
		stream["grpcSettings"] = map[string]interface{}{
			"serviceName": opt.ServiceName,
		}
	} else {
		stream["network"] = "tcp"
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "info",
		},
		"inbounds":  BuildClientInbounds(),
		"outbounds": []interface{}{outbound},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	configsDir := installer.GetConfigDir("vless")
	_ = os.MkdirAll(configsDir, 0755)

	fileName := "client.json"
	targetPath := filepath.Join(configsDir, fileName)
	err = os.WriteFile(targetPath, data, 0644)
	return targetPath, err
}

// VLESS Reality sharing URL (primary SNI).
func GenerateVlessURL(opt types.ConfigOptions) string {
	return GenerateVlessURLForSNI(opt, opt.Sni)
}

// VLESS Reality URL for a given SNI.
func GenerateVlessURLForSNI(opt types.ConfigOptions, sni string) string {
	var endpoint string
	switch {
	case opt.Host != "":
		endpoint = opt.Host
	case opt.ServerAddr != "":
		endpoint = opt.ServerAddr
	case opt.Sni != "":
		endpoint = opt.Sni
	default:
		endpoint = host_catalog.RandomHost()
	}

	sid := "12345678"
	if len(opt.ShortIds) > 0 {
		sid = opt.ShortIds[0]
	}
	tag := url.QueryEscape(fmt.Sprintf("TunnelBypass-VLESS-Reality-%s", utils.SanitizeForTag(sni)))

	return fmt.Sprintf(
		"vless://%s@%s:%d?security=reality&encryption=none&allowInsecure=true&pbk=%s&fp=chrome&sni=%s&sid=%s&spx=%s&type=tcp&headerType=none&alpn=h2%%2Chttp%%2F1.1&flow=xtls-rprx-vision#%s",
		opt.UUID, endpoint, opt.Port,
		url.QueryEscape(opt.PublicKey),
		sni, sid, url.QueryEscape("/"), tag,
	)
}

// v2rayN-compatible import JSON line.
func GenerateV2rayNJSON(opt types.ConfigOptions, publicIP string) string {
	var endpoint string
	switch {
	case opt.Host != "":
		endpoint = opt.Host
	case publicIP != "":
		endpoint = publicIP
	case opt.ServerAddr != "":
		endpoint = opt.ServerAddr
	case opt.Sni != "":
		endpoint = opt.Sni
	default:
		endpoint = host_catalog.RandomHost()
	}

	sid := "12345678"
	if len(opt.ShortIds) > 0 {
		sid = opt.ShortIds[0]
	}
	ps := fmt.Sprintf("TunnelBypass-VLESS-Reality-%s", utils.SanitizeForTag(opt.Sni))
	return fmt.Sprintf(
		`{"v":"2","ps":%q,"add":%q,"port":"%d","id":%q,"aid":"0","scy":"none","net":"tcp","type":"none","host":"","path":"","tls":"reality","sni":%q,"alpn":"h2,http/1.1","fp":"chrome","pbk":%q,"sid":%q,"spx":"/","flow":"xtls-rprx-vision","tlsVersion":"1.3"}`,
		ps, endpoint, opt.Port, opt.UUID, opt.Sni, opt.PublicKey, sid,
	)
}

// One VLESS URL per sharing SNI (primary + extras — not the full merged catalog).
func GenerateAllSNIUrls(opt types.ConfigOptions) []string {
	var urls []string
	for _, sni := range host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs) {
		if sni == "" {
			continue
		}
		urls = append(urls, fmt.Sprintf("# %s\n%s", sni, GenerateVlessURLForSNI(opt, sni)))
	}
	return urls
}

// ShareLink slice for Reality (one entry per sharing SNI).
func GenerateShareLinks(opt types.ConfigOptions) []utils.ShareLink {
	var links []utils.ShareLink
	for _, sni := range host_catalog.SharingLinkSNIs(opt.Sni, opt.ExtraSNIs) {
		if sni == "" {
			continue
		}
		links = append(links, utils.ShareLink{
			Label: fmt.Sprintf("VLESS Reality (%s)", sni),
			URL:   GenerateVlessURLForSNI(opt, sni),
		})
	}
	return links
}

// BuildClientInbounds returns the standard SOCKS (10808) + HTTP (10809) inbounds
// used by all Xray client configs so the local machine can connect to the proxy.
func BuildClientInbounds() []interface{} {
	return []interface{}{
		map[string]interface{}{
			"tag":      "socks-in",
			"port":     10808,
			"listen":   "127.0.0.1",
			"protocol": "socks",
			"settings": map[string]interface{}{
				"auth": "noauth",
				"udp":  true,
				"ip":   "127.0.0.1",
			},
			"sniffing": map[string]interface{}{
				"enabled":      true,
				"destOverride": []string{"http", "tls", "quic"},
			},
		},
		map[string]interface{}{
			"tag":      "http-in",
			"port":     10809,
			"listen":   "127.0.0.1",
			"protocol": "http",
			"sniffing": map[string]interface{}{
				"enabled":      true,
				"destOverride": []string{"http", "tls", "quic"},
			},
		},
	}
}

func getAbsLogPath(name string) string {
	logsDir := installer.GetLogsDir()
	_ = os.MkdirAll(logsDir, 0755)
	return filepath.Join(logsDir, name)
}
