package types

// Default listen and sidecar ports when configuration leaves a port at zero.
// Single source of truth: import "tunnelbypass/core/types" and use types.Default* constants.
const (
	// DefaultTLSTunnelListenPort is the standard HTTPS port for Reality, VLESS (WS/gRPC), WSS, stunnel TLS,
	// and plain Shadowsocks (no v2ray-plugin) when steered for “looks like HTTPS”.
	DefaultTLSTunnelListenPort = 443

	// DefaultHysteriaListenPort is the default QUIC listen port for Hysteria v2.
	DefaultHysteriaListenPort = 8443

	// DefaultSSHTLSDirectListenPort is the TLS front port for SSH-over-TLS direct / VLESS+TLS+SSH fallback setups.
	DefaultSSHTLSDirectListenPort = 2053

	// DefaultWireGuardListenPort is the default UDP listen for WireGuard.
	DefaultWireGuardListenPort = 51820

	// DefaultSSHSpecListenPort is the spec/wizard default for the legacy embedded SSH transport listen port.
	DefaultSSHSpecListenPort = 22

	// DefaultSSHPayloadListenPort is the cleartext HTTP payload listener port for HTTP Custom / Netmod.
	DefaultSSHPayloadListenPort = 80

	// DefaultUDPGWPort is the UDP gateway sidecar listen port (udpgw).
	DefaultUDPGWPort = 7300

	// DefaultShadowsocksListenPort is the provision default for Shadowsocks without v2ray-plugin (same as HTTPS front).
	DefaultShadowsocksListenPort = DefaultTLSTunnelListenPort

	// DefaultShadowsocksV2rayListenPort is the provision default for Shadowsocks + v2ray-plugin (SS + SNI).
	DefaultShadowsocksV2rayListenPort = DefaultTLSTunnelListenPort

	// DefaultShadowsocksConfigTemplatePort is used in shadowsocks JSON generators when Port is unset (local template only).
	DefaultShadowsocksConfigTemplatePort = 8388

	// DefaultXDNSListenPort is the standard DNS port for XDNS (VLESS+mKCP) tunnel.
	DefaultXDNSListenPort = 53

	// DefaultMDNSListenPort is the standard DNS port for MasterDnsVPN tunnel.
	DefaultMDNSListenPort = 53
)
