package wireguard

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"

	"golang.org/x/crypto/curve25519"
)

// Server and client .conf paths under configs/wireguard.
func GenerateWireGuardConfig(opt types.ConfigOptions) (string, string, error) {
	sPriv, sPub := generateWGKeys()
	cPriv, cPub := generateWGKeys()

	hostLabel := opt.Sni
	if hostLabel == "" {
		hostLabel = opt.Host
	}
	if hostLabel == "" {
		hostLabel = opt.ServerAddr
	}
	if hostLabel == "" {
		hostLabel = "TunnelBypass"
	}

	serverConfig := fmt.Sprintf(`# TunnelBypass (%s)
[Interface]
PrivateKey = %s
Address = 10.0.0.1/24
ListenPort = %d

[Peer]
PublicKey = %s
AllowedIPs = 10.0.0.2/32
`, hostLabel, sPriv, opt.Port, cPub)

	endpoint := opt.ServerAddr
	if opt.Host != "" {
		endpoint = opt.Host
	} else if opt.Sni != "" {
		endpoint = opt.Sni
	}

	clientConfig := fmt.Sprintf(`# TunnelBypass (%s)
[Interface]
PrivateKey = %s
Address = 10.0.0.2/24
DNS = 1.1.1.1

[Peer]
PublicKey = %s
Endpoint = %s:%d
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 21
`, hostLabel, cPriv, sPub, endpoint, opt.Port)

	configsDir := installer.GetConfigDir("wireguard")
	_ = os.MkdirAll(configsDir, 0755)

	srvPath := filepath.Join(configsDir, "wg_server.conf")
	cliPath := filepath.Join(configsDir, "wg_client.conf")

	_ = os.WriteFile(srvPath, []byte(serverConfig), 0600)
	err := os.WriteFile(cliPath, []byte(clientConfig), 0600)

	if err != nil {
		return srvPath, cliPath, err
	}
	return srvPath, cliPath, nil
}

// Raw client .conf as ShareLink (good for QR import in most apps).
func GenerateClientShareLink(cliPath string) (utils.ShareLink, error) {
	data, err := os.ReadFile(cliPath)
	if err != nil {
		return utils.ShareLink{}, err
	}
	return utils.ShareLink{
		Label: "WireGuard client",
		URL:   string(data), // best for QR import
	}, nil
}

// data: URL with base64 client config (optional; file import is usual).
func GenerateClientDataURL(cliPath string) (string, error) {
	data, err := os.ReadFile(cliPath)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	// Strip any accidental whitespace; keep URL compact.
	b64 = strings.TrimSpace(b64)
	return "data:text/plain;base64," + b64, nil
}

// wireguard://<base64> import link (non-standard but common in apps).
func GenerateClientWireGuardBase64URL(cliPath string) (string, error) {
	data, err := os.ReadFile(cliPath)
	if err != nil {
		return "", err
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	b64 = strings.TrimSpace(b64)
	return "wireguard://" + b64, nil
}

type wgClientParsed struct {
	privateKey    string
	address       string
	peerPublicKey string
	endpoint      string
	keepalive     string
}

func parseWGClientConfig(cliPath string) (wgClientParsed, error) {
	f, err := os.Open(cliPath)
	if err != nil {
		return wgClientParsed{}, err
	}
	defer f.Close()

	var out wgClientParsed
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		switch k {
		case "privatekey":
			out.privateKey = v
		case "address":
			out.address = v
		case "publickey":
			// In client config, this is the peer (server) public key under [Peer]
			out.peerPublicKey = v
		case "endpoint":
			out.endpoint = v
		case "persistentkeepalive":
			out.keepalive = v
		}
	}
	if err := s.Err(); err != nil {
		return wgClientParsed{}, err
	}
	return out, nil
}

// wg:// link for Throne / NekoBox / Hiddify-style clients.
func GenerateClientWGURL(cliPath string, name string) (string, error) {
	p, err := parseWGClientConfig(cliPath)
	if err != nil {
		return "", err
	}
	if p.privateKey == "" || p.peerPublicKey == "" || p.endpoint == "" || p.address == "" {
		return "", fmt.Errorf("incomplete wireguard client config for wg:// link")
	}

	u := url.URL{
		Scheme: "wg",
		Host:   p.endpoint,
	}
	q := url.Values{}
	q.Set("private_key", p.privateKey)
	q.Set("local_address", p.address)
	q.Set("public_key", p.peerPublicKey)
	if strings.TrimSpace(p.keepalive) != "" {
		q.Set("persistent_keepalive_interval", strings.TrimSpace(p.keepalive))
	}
	// Hint clients to use plain DNS (avoid DoT 853 timeouts).
	q.Set("dns", "1.1.1.1")
	q.Set("dns_port", "53")
	u.RawQuery = q.Encode()
	name = strings.TrimSpace(name)
	if name != "" {
		// Throne/NekoBox uses fragment (#name) as profile display name.
		// We ensure it includes TunnelBypass + host to avoid generic "name".
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "tunnelbypass") {
			u.Fragment = "TunnelBypass-" + name
		} else {
			u.Fragment = name
		}
	}
	return u.String(), nil
}

// sing-box JSON for Throne; plain UDP DNS on 53 (avoids DoT 853 issues). Path to written JSON.
func GenerateThroneProfile(cliPath string, profileName string) (string, error) {
	p, err := parseWGClientConfig(cliPath)
	if err != nil {
		return "", err
	}
	if p.privateKey == "" || p.peerPublicKey == "" || p.endpoint == "" || p.address == "" {
		return "", fmt.Errorf("incomplete wireguard client config for Throne profile")
	}

	host := p.endpoint
	port := 0
	if parts := strings.Split(p.endpoint, ":"); len(parts) == 2 {
		host = parts[0]
		fmt.Sscanf(parts[1], "%d", &port)
	}
	if port == 0 {
		return "", fmt.Errorf("invalid endpoint: %s", p.endpoint)
	}

	name := strings.TrimSpace(profileName)
	if name == "" {
		name = "TunnelBypass WireGuard"
	}

	// sing-box config (minimal) for Throne:
	// - mixed inbound on 127.0.0.1:2080
	// - wireguard outbound tag "proxy"
	// - DNS uses UDP 1.1.1.1:53 via detour "proxy"
	cfg := map[string]any{
		"log": map[string]any{
			"level": "info",
		},
		"dns": map[string]any{
			"servers": []any{
				map[string]any{
					"type":     "udp",
					"tag":      "cf",
					"address":  "1.1.1.1:53",
					"detour":   "proxy",
					"strategy": "prefer_ipv4",
				},
			},
			"final": "cf",
		},
		"inbounds": []any{
			map[string]any{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": 2080,
			},
		},
		"outbounds": []any{
			map[string]any{
				"type":                          "wireguard",
				"tag":                           "proxy",
				"server":                        host,
				"server_port":                   port,
				"local_address":                 []string{p.address},
				"private_key":                   p.privateKey,
				"peer_public_key":               p.peerPublicKey,
				"persistent_keepalive_interval": 21,
			},
			map[string]any{"type": "direct", "tag": "direct"},
			map[string]any{"type": "block", "tag": "block"},
		},
		"route": map[string]any{
			"final":                 "proxy",
			"auto_detect_interface": true,
		},
		"_profile_name": name,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}

	configsDir := installer.GetConfigDir("wireguard")
	_ = os.MkdirAll(configsDir, 0755)
	outPath := filepath.Join(configsDir, "throne-wireguard.json")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return "", err
	}
	return outPath, nil
}

func generateWGKeys() (string, string) {
	// Proper WireGuard keys are X25519:
	// - private key is 32 random bytes with clamping
	// - public key is X25519(private, basepoint)
	priv := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, priv)
	priv[0] &= 248
	priv[31] = (priv[31] & 127) | 64

	pub, _ := curve25519.X25519(priv, curve25519.Basepoint)
	return base64.StdEncoding.EncodeToString(priv), base64.StdEncoding.EncodeToString(pub)
}
