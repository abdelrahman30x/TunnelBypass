package portable

import "testing"

func TestOSServiceNameForTransportCoversUninstallTypes(t *testing.T) {
	tests := map[string]string{
		"reality":        "TunnelBypass-VLESS",
		"vless":          "TunnelBypass-VLESS",
		"vless-ws":       "TunnelBypass-VLESS-WS",
		"vless-grpc":     "TunnelBypass-VLESS-GRPC",
		"ssh-tls":        "TunnelBypass-SSH-TLS",
		"hysteria":       "TunnelBypass-Hysteria",
		"wireguard":      "TunnelBypass-WireGuard",
		"ssh":            "TunnelBypass-SSH",
		"wss":            "TunnelBypass-WSS",
		"tls":            "TunnelBypass-SSL",
		"shadowsocks":    "TunnelBypass-Shadowsocks",
		"shadowsocks-ws": "TunnelBypass-Shadowsocks-WS",
		"xdns":           "TunnelBypass-XDNS",
		"mdns":           "TunnelBypass-MasterDnsVPN",
		"udpgw":          "TunnelBypass-UDPGW",
	}
	for transport, want := range tests {
		t.Run(transport, func(t *testing.T) {
			if got := OSServiceNameForTransport(transport); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
