package vless

import (
	"fmt"
	"strings"
)

// MergeXrayDNSIntoConfig injects application-level DNS (queryStrategy UseIPv4) and routing so DNS
// traffic uses the direct outbound (avoids resolution loops through the tunnel). Call on server
// JSON maps before marshal.
//
// Gaming-oriented defaults: UseIPv4 skips failed IPv6 resolution paths common on some ISPs;
// direct outbound for DNS and UDP/53 avoids tunnel loops and extra RTT on first connect.
func MergeXrayDNSIntoConfig(root map[string]interface{}) {
	ensureDirectFreedomOutbound(root)

	root["dns"] = map[string]interface{}{
		"servers": []interface{}{
			"localhost",
			map[string]interface{}{"address": "1.1.1.1", "port": 53},
			map[string]interface{}{"address": "8.8.8.8", "port": 53},
		},
		"queryStrategy": "UseIPv4",
	}
	routing, ok := root["routing"].(map[string]interface{})
	if !ok {
		routing = map[string]interface{}{
			"domainStrategy": "UseIPv4",
			"rules":          []interface{}{},
		}
		root["routing"] = routing
	}
	// Global: avoid IPv6 resolution attempts in routing (first-hit latency on broken IPv6 paths).
	routing["domainStrategy"] = "UseIPv4"
	rules, _ := routing["rules"].([]interface{})
	dnsRules := []interface{}{
		map[string]interface{}{
			"type":        "field",
			"protocol":    []string{"dns"},
			"outboundTag": "direct",
		},
		map[string]interface{}{
			"type":        "field",
			"network":     "udp,tcp",
			"port":        53,
			"outboundTag": "direct",
		},
		map[string]interface{}{
			"type":        "field",
			"ip":          []string{"1.1.1.1", "8.8.8.8"},
			"outboundTag": "direct",
		},
	}
	routing["rules"] = append(dnsRules, rules...)
}

func ensureDirectFreedomOutbound(root map[string]interface{}) {
	outbounds, ok := root["outbounds"].([]interface{})
	if !ok {
		outbounds = nil
	}
	for _, raw := range outbounds {
		ob, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if outboundIsDirectFreedom(ob) {
			return
		}
	}
	direct := map[string]interface{}{
		"protocol": "freedom",
		"tag":      "direct",
		"settings": map[string]interface{}{
			"domainStrategy": "UseIPv4",
		},
	}
	root["outbounds"] = append([]interface{}{direct}, outbounds...)
}

func outboundIsDirectFreedom(ob map[string]interface{}) bool {
	p := strings.TrimSpace(fmt.Sprint(ob["protocol"]))
	t := strings.TrimSpace(fmt.Sprint(ob["tag"]))
	return strings.EqualFold(p, "freedom") && t == "direct"
}
