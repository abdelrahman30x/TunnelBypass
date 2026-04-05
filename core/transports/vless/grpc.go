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

// GRPCServiceNameFromSNI returns the gRPC service name bound to the tunnel SNI so client and server
// match. Format: TunnelBypass-<hostname>, e.g. SNI pubgmobile.com → TunnelBypass-pubgmobile.com.
func GRPCServiceNameFromSNI(sni string) string {
	sni = strings.TrimSpace(sni)
	if sni == "" {
		return "TunnelBypass-localhost"
	}
	return "TunnelBypass-" + sni
}

// GenerateVlessGRPCServerConfig writes Xray VLESS + REALITY + gRPC server JSON under configs/vless-grpc.
//
// Stack: VLESS → gRPC (multi-stream) → REALITY (TLS camouflage to dest).
// Matches upstream VLESS-gRPC-REALITY examples; avoids deprecated plain-TLS+gRPC.
func GenerateVlessGRPCServerConfig(opt types.ConfigOptions) (string, error) {
	if opt.Port == 0 {
		opt.Port = 443
	}
	opt.UUID = strings.TrimSpace(opt.UUID)
	if opt.UUID == "" {
		opt.UUID = utils.GenerateUUID()
	}
	if strings.TrimSpace(opt.PrivateKey) == "" || strings.TrimSpace(opt.PublicKey) == "" {
		return "", fmt.Errorf("vless-grpc: reality privateKey and publicKey are required (generate in provision)")
	}
	sids := opt.ShortIds
	if len(sids) == 0 {
		sids = []string{"8d2c", "74d3", "3bd4", ""}
	}
	if strings.TrimSpace(opt.RealityDest) == "" {
		opt.RealityDest = host_catalog.DefaultRealityDestAddress()
	}

	host := strings.TrimSpace(opt.Sni)
	if host == "" {
		host = "localhost"
	}
	sharingSNIs := host_catalog.RealitySharingSNIs(opt.Sni, opt.ExtraSNIs)
	serverNames := host_catalog.AppendRealityDestHosts(sharingSNIs)
	sharingIface := make([]interface{}, len(sharingSNIs))
	for i, s := range sharingSNIs {
		sharingIface[i] = s
	}

	var email string
	hostLabel := utils.SanitizeForTag(host)
	if hostLabel == "" {
		email = "TunnelBypass-grpc-reality"
	} else {
		email = fmt.Sprintf("TunnelBypass-grpc-%s", hostLabel)
	}

	serviceName := GRPCServiceNameFromSNI(host)

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			"access":   getAbsLogPath("xray_grpc_access.log"),
			"error":    getAbsLogPath("xray_grpc_error.log"),
		},
		host_catalog.MetaKey: map[string]interface{}{
			"version":     1,
			"sharingSNIs": sharingIface,
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "vless-grpc-in",
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
					"network":  "grpc",
					"security": "reality",
					"grpcSettings": map[string]interface{}{
						"serviceName":          serviceName,
						"multiMode":            true,
						"idleTimeout":          30,
						"healthCheckTimeout":   15,
						"initial_windows_size": 524288,
					},
					"realitySettings": map[string]interface{}{
						"show":        false,
						"dest":        opt.RealityDest,
						"xver":        0,
						"privateKey":  opt.PrivateKey,
						"shortIds":    sids,
						"serverNames": serverNames,
						"publicKey":   opt.PublicKey,
					},
					"sockopt": map[string]interface{}{
						"tcpNoDelay":  true,
						"tcpFastOpen": true,
						"v6only":      false,
					},
				},
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls"},
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
			"domainStrategy": "UseIPv4",
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
					"handshake":    4,
					"connIdle":     300,
					"uplinkOnly":   5,
					"downlinkOnly": 30,
				},
			},
			"system": map[string]interface{}{
				"statsInboundUplink":   false,
				"statsInboundDownlink": false,
			},
		},
	}

	MergeXrayDNSIntoConfig(config)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	cfgDir := installer.GetConfigDir("vless-grpc")
	_ = os.MkdirAll(cfgDir, 0755)
	targetPath := filepath.Join(cfgDir, "server.json")
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", err
	}
	if err := EnsureInboundListenIPv4(targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// GenerateVlessGRPCClientConfig writes an optional Xray client JSON (VLESS REALITY + gRPC).
func GenerateVlessGRPCClientConfig(opt types.ConfigOptions) (string, error) {
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
	serviceName := GRPCServiceNameFromSNI(sni)

	sid := "12345678"
	if len(opt.ShortIds) > 0 {
		sid = opt.ShortIds[0]
	}
	names := host_catalog.ServerNamesForVLESS(opt.Sni, opt.ExtraSNIs)
	clientSNI := opt.Sni
	if clientSNI == "" && len(names) > 0 {
		clientSNI = names[0]
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
						},
					},
				},
			},
		},
		"streamSettings": map[string]interface{}{
			"network":  "grpc",
			"security": "reality",
			"grpcSettings": map[string]interface{}{
				"serviceName": serviceName,
				"multiMode":   true,
			},
			"realitySettings": map[string]interface{}{
				"publicKey":   opt.PublicKey,
				"shortId":     sid,
				"fingerprint": "chrome",
				"serverName":  clientSNI,
				"serverNames": names,
			},
		},
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
			"access":   getAbsLogPath("xray_grpc_access.log"),
			"error":    getAbsLogPath("xray_grpc_error.log"),
		},
		"outbounds": []interface{}{outbound},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	cfgDir := installer.GetConfigDir("vless-grpc")
	_ = os.MkdirAll(cfgDir, 0755)
	targetPath := filepath.Join(cfgDir, "client.json")
	return targetPath, os.WriteFile(targetPath, data, 0644)
}

// GenerateVlessGRPCURL returns a vless:// sharing link for VLESS REALITY + gRPC.
func GenerateVlessGRPCURL(opt types.ConfigOptions) string {
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
	serviceName := GRPCServiceNameFromSNI(sni)

	sid := "12345678"
	if len(opt.ShortIds) > 0 {
		sid = opt.ShortIds[0]
	}
	pbk := strings.TrimSpace(opt.PublicKey)
	tag := url.QueryEscape(fmt.Sprintf("TunnelBypass-gRPC-Reality-%s", utils.SanitizeForTag(sni)))

	return fmt.Sprintf(
		"vless://%s@%s:%d?encryption=none&security=reality&pbk=%s&fp=chrome&sni=%s&sid=%s&spx=%s&type=grpc&serviceName=%s&mode=multi&packetEncoding=xudp#%s",
		opt.UUID,
		endpoint,
		opt.Port,
		url.QueryEscape(pbk),
		url.QueryEscape(sni),
		sid,
		url.QueryEscape("/"),
		url.QueryEscape(serviceName),
		tag,
	)
}
