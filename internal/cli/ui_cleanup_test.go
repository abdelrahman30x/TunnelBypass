package cli

import (
	"os"
	"path/filepath"
	"testing"

	"tunnelbypass/core/installer"
)

func TestDetectInstalledTransportKnownProtocols(t *testing.T) {
	tests := []struct {
		service string
		want    installedTransport
	}{
		{"TunnelBypass-VLESS", transportXray},
		{"TunnelBypass-VLESS-WS", transportVLESSWS},
		{"TunnelBypass-VLESS-GRPC", transportGRPC},
		{"TunnelBypass-SSH-TLS", transportSSHTLS},
		{"TunnelBypass-Hysteria", transportHysteria},
		{"TunnelBypass-WireGuard", transportWireGuard},
		{"WireGuardTunnel$wg_server", transportWireGuard},
		{"wg-quick@wg_server", transportWireGuard},
		{"TunnelBypass-SSH", transportSSH},
		{"TunnelBypass-SSH-Forwarder", transportSSH},
		{"TunnelBypass-SSL", transportSSL},
		{"TunnelBypass-WSS", transportWSS},
		{"TunnelBypass-Shadowsocks", transportShadowsocks},
		{"TunnelBypass-Shadowsocks-WS", transportShadowsocksWS},
		{"TunnelBypass-XDNS", transportXDNS},
		{"TunnelBypass-MasterDnsVPN", transportMDNS},
		{"TunnelBypass-UDPGW", transportUDPGW},
	}
	for _, tc := range tests {
		t.Run(tc.service, func(t *testing.T) {
			if got := detectInstalledTransport(tc.service); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanupArtifactsForTransportRemovesProtocolFilesAndLogs(t *testing.T) {
	dir := t.TempDir()
	installer.SetDataRootOverride(dir)
	t.Cleanup(func() { installer.SetDataRootOverride("") })

	tests := []struct {
		name      string
		tr        installedTransport
		configDir string
	}{
		{"TunnelBypass-VLESS", transportXray, "vless"},
		{"TunnelBypass-VLESS-WS", transportVLESSWS, "vless-ws"},
		{"TunnelBypass-VLESS-GRPC", transportGRPC, "vless-grpc"},
		{"TunnelBypass-SSH-TLS", transportSSHTLS, "ssh-tls"},
		{"TunnelBypass-Hysteria", transportHysteria, "hysteria"},
		{"TunnelBypass-WireGuard", transportWireGuard, "wireguard"},
		{"TunnelBypass-SSH", transportSSH, "ssh"},
		{"TunnelBypass-SSL", transportSSL, "stunnel"},
		{"TunnelBypass-WSS", transportWSS, "wstunnel"},
		{"TunnelBypass-Shadowsocks", transportShadowsocks, "shadowsocks"},
		{"TunnelBypass-XDNS", transportXDNS, "xdns"},
		{"TunnelBypass-MasterDnsVPN", transportMDNS, "mdns"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgDir := installer.GetConfigDir(tc.configDir)
			if err := os.MkdirAll(cfgDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfgDir, "marker"), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
			logDir := filepath.Join(installer.GetBaseDir(), "logs")
			if err := os.MkdirAll(logDir, 0755); err != nil {
				t.Fatal(err)
			}
			for _, suffix := range []string{".out.log", ".err.log", ".wrapper.log", ".log"} {
				if err := os.WriteFile(filepath.Join(logDir, tc.name+suffix), []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			cleanupArtifactsForTransport(tc.tr, tc.name)

			if _, err := os.Stat(cfgDir); !os.IsNotExist(err) {
				t.Fatalf("config dir still exists: %s", cfgDir)
			}
			for _, suffix := range []string{".out.log", ".err.log", ".wrapper.log", ".log"} {
				p := filepath.Join(logDir, tc.name+suffix)
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Fatalf("log still exists: %s", p)
				}
			}
		})
	}
}

func TestDedupeServiceNamesKeepsOrder(t *testing.T) {
	got := dedupeServiceNames([]string{"TunnelBypass-SSH", "tunnelbypass-ssh", "", "TunnelBypass-UDPGW"})
	want := []string{"TunnelBypass-SSH", "TunnelBypass-UDPGW"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}
