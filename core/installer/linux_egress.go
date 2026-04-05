package installer

import (
	"os/exec"
	"strings"
)

// LinuxEgressInterface returns the outbound interface for default IPv4 traffic (e.g. eth0), parsed
// from `ip route get 8.8.8.8`. Empty string if unknown.
func LinuxEgressInterface() string {
	out, err := exec.Command("sh", "-c", "ip -4 route get 8.8.8.8 2>/dev/null").Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	parts := strings.Fields(string(out))
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "dev" && i+1 < len(parts) {
			iface := strings.TrimSpace(parts[i+1])
			if iface != "" && iface != "lo" {
				return iface
			}
		}
	}
	return ""
}
