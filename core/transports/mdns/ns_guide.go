package mdns

import (
	"fmt"
	"strings"
)

// PrintNSDelegationGuide prints DNS setup instructions for MasterDnsVPN.
func PrintNSDelegationGuide(serverIP, domain string) {
	if domain == "" {
		domain = "v.example.com"
	}
	if serverIP == "" {
		serverIP = "<your-server-ip>"
	}

	nsRecord := "ns." + domain
	if strings.Count(domain, ".") > 1 {
		parts := strings.SplitN(domain, ".", 2)
		nsRecord = "ns." + parts[1]
	}

	fmt.Println()
	fmt.Println("[!] MasterDnsVPN requires DNS NS delegation")
	fmt.Println()
	fmt.Println("    Follow these steps in your DNS provider (Cloudflare, Route53, etc.):")
	fmt.Println()
	fmt.Printf("    1. Create A record: %s -> %s\n", nsRecord, serverIP)
	fmt.Println("       Set TTL to the LOWEST possible value (e.g., 60 seconds).")
	fmt.Println("       Short TTL is critical: if the IP is blocked by your ISP, you can")
	fmt.Println("       change the A record and clients will pick up the new IP quickly")
	fmt.Println("       without waiting for stale ISP DNS cache.")
	fmt.Println()
	fmt.Printf("    2. Create NS record: %s -> %s\n", domain, nsRecord)
	fmt.Println("       Set TTL to the LOWEST possible value (e.g., 60 seconds).")
	fmt.Println()
	fmt.Println("    3. Wait for DNS propagation (minutes to hours depending on TTL)")
	fmt.Println()
	fmt.Printf("    4. Verify direct query:   dig @%s %s A\n", nsRecord, domain)
	fmt.Printf("    5. Verify recursive query (from any machine): dig @1.1.1.1 %s A\n", domain)
	fmt.Println()
	if strings.Count(domain, ".") == 1 {
		fmt.Println("    ⚠ WARNING: You are using a root domain for NS delegation.")
		fmt.Println("       Many DNS providers (Cloudflare, GoDaddy, etc.) do NOT support")
		fmt.Println("       NS records on the root/apex domain. If verification fails,")
		fmt.Printf("       use a subdomain instead, e.g.: v.%s or tunnel.%s\n", domain, domain)
		fmt.Println()
	}
	fmt.Println("    NOTE: ISP-level IP blocks are common. With TTL=60s, resolver cache")
	fmt.Println("    expires in 1 minute. With TTL=86400s (24h), a blocked IP can leave")
	fmt.Println("    users disconnected for a full day.")
	fmt.Println()
}
