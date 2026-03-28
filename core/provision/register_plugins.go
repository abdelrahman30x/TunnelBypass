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
	transport.RegisterProvision("udpgw", nil, func(log *slog.Logger, opt types.ConfigOptions, _, _ string) (transport.Result, error) {
		_ = log
		_ = opt
		// Standalone UDPGW has no config files; engine still runs portable.RunNamed("udpgw", ...).
		return transport.Result{}, nil
	})
}
