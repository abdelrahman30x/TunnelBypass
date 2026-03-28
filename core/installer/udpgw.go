package installer

import "tunnelbypass/core/udpgw"

// RunUDPGWServer starts the badvpn-compatible UDPGW TCP server (built-in internal gateway by default).
func RunUDPGWServer(port int) error {
	return udpgw.RunLegacy(port)
}
