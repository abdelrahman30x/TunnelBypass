package cli

import (
	"log/slog"

	"tunnelbypass/core/svcinstall"
	"tunnelbypass/core/types"
)

// exitInstallServiceFailed is used when --install-service cannot be honored.
const exitInstallServiceFailed = 3

func runTryInstallService(log *slog.Logger, transport string, opt types.ConfigOptions, isAdmin bool) error {
	_ = log
	return svcinstall.InstallRunTransportService(transport, opt, isAdmin)
}
