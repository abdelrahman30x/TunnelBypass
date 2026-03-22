package installer

import "tunnelbypass/core/udpgw"

// RunUDPGWServer starts the badvpn-compatible UDPGW TCP server (internal or external per TB_UDPGW_*).
func RunUDPGWServer(port int) error {
	return udpgw.RunLegacy(port)
}
