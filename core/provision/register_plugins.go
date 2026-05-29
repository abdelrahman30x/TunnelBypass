package provision

import (
	"log/slog"

	"tunnelbypass/core/transport"
	"tunnelbypass/core/types"
)

func init() {
	transport.RegisterProvision("reality", []string{"vless"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionReality(log, opt, so, co)
	})
	transport.RegisterProvision("vless-ws", []string{"vlessws", "xray-wss", "xraywss"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionVlessWS(log, opt, so, co)
	})
	transport.RegisterProvision("vless-grpc", []string{"grpc", "grpc-tls"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionVlessGRPC(log, opt, so, co)
	})
	transport.RegisterProvision("ssh-tls", []string{"ssh-tls-direct", "vless-tls-ssh"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionSSH_TLS(log, opt, so, co)
	})
	transport.RegisterProvision("hysteria", nil, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionHysteria(log, opt, so, co)
	})
	transport.RegisterProvision("wireguard", nil, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionWireguard(log, opt, so, co)
	})
	transport.RegisterProvision("ssh", nil, func(log *slog.Logger, opt types.ConfigOptions, _, _ string) (transport.Result, error) {
		return provisionSSH(log, opt)
	})
	transport.RegisterProvision("tls", nil, func(log *slog.Logger, opt types.ConfigOptions, _, _ string) (transport.Result, error) {
		return provisionTLS(log, opt)
	})
	transport.RegisterProvision("wss", nil, func(log *slog.Logger, opt types.ConfigOptions, _, _ string) (transport.Result, error) {
		return provisionWSS(log, opt)
	})
	transport.RegisterProvision("ssh-payload", []string{"payload", "ssh-http-payload", "http-payload"}, func(log *slog.Logger, opt types.ConfigOptions, _, _ string) (transport.Result, error) {
		return provisionSSHPayload(log, opt)
	})
	transport.RegisterProvision("udpgw", nil, func(log *slog.Logger, opt types.ConfigOptions, _, _ string) (transport.Result, error) {
		_ = log
		_ = opt
		// Standalone UDPGW has no config files; engine still runs portable.RunNamed("udpgw", ...).
		return transport.Result{}, nil
	})
	transport.RegisterProvision("shadowsocks", []string{"ss"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionShadowsocks(log, opt, so, co)
	})
	transport.RegisterProvision("shadowsocks-ws", []string{"ss-ws"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		opt.SSPlugin = "v2ray-plugin"
		return provisionShadowsocks(log, opt, so, co)
	})
	transport.RegisterProvision("xdns", []string{"vless-mkcp", "vless-dns", "dns-tunnel"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionXDNS(log, opt, so, co)
	})
	transport.RegisterProvision("mdns", []string{"masterdns", "mdnsvpn"}, func(log *slog.Logger, opt types.ConfigOptions, so, co string) (transport.Result, error) {
		return provisionMDNS(log, opt, so, co)
	})
}
