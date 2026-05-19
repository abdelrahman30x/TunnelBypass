package types

import "tunnelbypass/internal/network"

type ConfigOptions struct {
	Transport   string   `json:"transport"`
	ServerAddr  string   `json:"server"`
	Port        int      `json:"port"`
	UUID        string   `json:"uuid"`
	Sni         string   `json:"sni"`
	ExtraSNIs   []string `json:"extra_snis"`
	ServiceName string   `json:"service_name"`
	PrivateKey  string   `json:"private_key"`
	PublicKey   string   `json:"public_key"`
	ShortIds    []string `json:"short_ids"`
	RealityDest string   `json:"reality_dest"`
	// RealityDestHost and RealityDestExtraHosts are per-config TCP camouflage preferences.
	// RealityDest is the concrete Xray address (host:443), while RealityDestHost is the
	// hostname shown in menus and added to serverNames.
	RealityDestHost       string   `json:"reality_dest_host"`
	RealityDestExtraHosts []string `json:"reality_dest_extra_hosts"`
	Host                  string   `json:"host"`
	ObfsPassword          string   `json:"obfs_password"`
	WSPath                string   `json:"ws_path"`

	// Shadowsocks-specific options.
	SSMethod string `json:"ss_method"` // Shadowsocks cipher (default: 2022-blake3-aes-256-gcm per SS2022 / Xray docs).
	// For SS2022 methods: PSK length must match the cipher (e.g. 32-byte key as standard base64 for 2022-blake3-aes-256-gcm).
	// TunnelBypass auto-generates that when empty. For Xray multi-user inbound, client password is "ServerPSK:UserPSK" (not used in single-password ss-server JSON).
	SSPassword   string `json:"ss_password"`
	SSPlugin     string `json:"ss_plugin"`      // Plugin name (e.g. "v2ray-plugin")
	SSPluginOpts string `json:"ss_plugin_opts"` // Plugin options

	// SSV2rayClientCertRaw is base64(DER) for v2ray-plugin client certRaw= (embedded in ss:// / client.json; no client-side cert file).
	// Set at provision from the server TLS cert; also stored in server.json _tunnelbypass.v2rayClientCertRaw for sharing regeneration.
	SSV2rayClientCertRaw string `json:"-"`
	// SSV2rayPluginWSPath is v2ray-plugin WebSocket path=… (random per provision); stored in server.json meta for sharing links.
	SSV2rayPluginWSPath string `json:"-"`

	SSHUser           string `json:"ssh_user"`
	SSHPassword       string `json:"ssh_password"`
	SSHWelcomeMessage string `json:"ssh_welcome_message"`
	SSHIsAdmin        bool   `json:"ssh_is_admin"`
	SSHBackendPort    int    `json:"ssh_backend_port"`

	// Linux server install: adaptive networking (see ApplyLinuxTransitNetworking).
	LinuxOptimizeNet    bool `json:"linux_optimize_net,omitempty"`
	LinuxDNSFix         bool `json:"linux_dns_fix,omitempty"`
	LinuxRouter         bool `json:"linux_router,omitempty"`
	LinuxNoAutoOptimize bool `json:"linux_no_auto_optimize,omitempty"`

	// XDNS / mKCP tuning options.
	KCPSeed       string `json:"kcp_seed"`
	KCPMTU        int    `json:"kcp_mtu"`         // default: 1350
	KCPTTI        int    `json:"kcp_tti"`         // default: 20
	KCPHeaderType string `json:"kcp_header_type"` // default: "none"

	// MasterDnsVPN options.
	MDNSDomain           string   `json:"mdns_domain"`
	MDNSEncryptionKey    string   `json:"mdns_encryption_key"`
	MDNSEncryptionMethod int      `json:"mdns_encryption_method"` // 0-5, default 3 (AES-128-GCM)
	MDNSLocalDNS         bool     `json:"mdns_local_dns"`
	MDNSResolvers        []string `json:"mdns_resolvers"`

	// NetworkProfile is resolved once in engine.Run (read-only for generators).
	NetworkProfile network.NetworkProfile `json:"-"`
}

// XrayServerConfig is used for parsing server-side configuration
type XrayServerConfig struct {
	Inbounds []struct {
		Port     int `json:"port"`
		Settings struct {
			Clients []struct {
				ID   string `json:"id"`
				Flow string `json:"flow"`
			} `json:"clients"`
		} `json:"settings"`
		StreamSettings struct {
			Network         string `json:"network"`
			Security        string `json:"security"`
			RealitySettings struct {
				PrivateKey  string   `json:"privateKey"`
				PublicKey   string   `json:"publicKey"`
				ShortIds    []string `json:"shortIds"`
				ServerNames []string `json:"serverNames"`
			} `json:"realitySettings"`
		} `json:"streamSettings"`
	} `json:"inbounds"`
}

const (
	TransportReality       = "reality"
	TransportGRPC          = "grpc"
	TransportVLESS         = "vless-tcp"
	TransportUDP           = "udp"
	TransportHysteria      = "hysteria"
	TransportShadowsocks   = "shadowsocks"
	TransportShadowsocksWS = "shadowsocks-ws"
	TransportXDNS          = "xdns"
	TransportMDNS          = "mdns"
)
