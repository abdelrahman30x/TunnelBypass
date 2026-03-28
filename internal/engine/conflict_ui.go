package engine

import (
	"fmt"
	"strings"

	"tunnelbypass/core/portable"
	"tunnelbypass/internal/uicolors"
	"tunnelbypass/internal/utils"
)

// FormatPortConflictPretty renders a wizard-style box for port conflicts (non-portable runs).
func FormatPortConflictPretty(c portable.PortConflict) string {
	var b strings.Builder
	hdr := uicolors.ColorBold + uicolors.ColorYellow
	b.WriteString(fmt.Sprintf("\n%s╔══════════════════════════════════════════════════════════════╗%s\n", hdr, uicolors.ColorReset))
	b.WriteString(fmt.Sprintf("%s║     %s%sLISTEN PORT IN USE — CANNOT START OR REINSTALL%s%s          ║%s\n",
		hdr, uicolors.ColorBold, uicolors.ColorRed, uicolors.ColorReset, hdr, uicolors.ColorReset))
	b.WriteString(fmt.Sprintf("%s╚══════════════════════════════════════════════════════════════╝%s\n", hdr, uicolors.ColorReset))

	b.WriteString(fmt.Sprintf("\n  %sPort:%s %s%d%s is already bound by another process or service.\n",
		uicolors.ColorBold, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, c.Port, uicolors.ColorReset))

	if c.OSServiceName != "" {
		b.WriteString(fmt.Sprintf("\n  %sRecommended:%s stop OS service %s%s%s (step 1), or uninstall (step 2), before retrying.\n",
			uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, c.OSServiceName, uicolors.ColorReset))
	} else if c.PID > 0 {
		b.WriteString(fmt.Sprintf("\n  %sRecommended:%s identify PID %s%d%s; run %s%s status%s before any force-kill.\n",
			uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset,
			uicolors.ColorGreen, c.PID, uicolors.ColorReset,
			uicolors.ColorBold+uicolors.ColorCyan, utils.AppName(), uicolors.ColorReset))
	}

	b.WriteString(fmt.Sprintf("\n  %sDetected%s\n", uicolors.ColorBold+uicolors.ColorCyan, uicolors.ColorReset))
	if c.ProcessName != "" || c.PID > 0 {
		b.WriteString(fmt.Sprintf("    %s·%s Process: %s%s%s (PID %s%d%s)\n",
			uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, c.ProcessName, uicolors.ColorReset, uicolors.ColorGreen, c.PID, uicolors.ColorReset))
	}
	if c.RunningTransport != "" {
		b.WriteString(fmt.Sprintf("    %s·%s Transport in registry: %s%s%s\n",
			uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold, c.RunningTransport, uicolors.ColorReset))
	}
	if c.OSServiceName != "" {
		b.WriteString(fmt.Sprintf("    %s·%s %sStop this Windows/Linux service first:%s %s%s%s\n",
			uicolors.ColorCyan, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset, uicolors.ColorBold+uicolors.ColorGreen, c.OSServiceName, uicolors.ColorReset))
	}

	b.WriteString(fmt.Sprintf("\n  %sWhat to do%s\n", uicolors.ColorBold+uicolors.ColorYellow, uicolors.ColorReset))
	for i, block := range c.Suggestions {
		lines := strings.Split(block, "\n")
		for j, line := range lines {
			prefix := "     "
			if j == 0 {
				prefix = fmt.Sprintf("  %s%d)%s ", uicolors.ColorBold+uicolors.ColorCyan, i+1, uicolors.ColorReset)
			}
			b.WriteString(prefix + line + "\n")
		}
		if i < len(c.Suggestions)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}
