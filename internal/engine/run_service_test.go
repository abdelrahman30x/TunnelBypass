package engine

import "testing"

func TestTransportInstallsOSServiceIncludesServiceBackedProtocols(t *testing.T) {
	for _, transport := range []string{
		"reality",
		"vless",
		"vless-ws",
		"vless-grpc",
		"ssh-tls",
		"hysteria",
		"wireguard",
		"wss",
		"tls",
		"shadowsocks",
		"shadowsocks-ws",
		"xdns",
		"mdns",
	} {
		t.Run(transport, func(t *testing.T) {
			if !transportInstallsOSService(transport) {
				t.Fatalf("%s should install an OS service", transport)
			}
		})
	}
}
