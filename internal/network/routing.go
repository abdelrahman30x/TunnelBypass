package network

// BuildXrayPrivateRule returns routing rule elements for private IP handling.
func BuildXrayPrivateRule(mode PrivateRoutingMode) []interface{} {
	return []interface{}{
		map[string]interface{}{
			"type":        "field",
			"ip":          []string{"geoip:private"},
			"outboundTag": "block",
		},
	}
}
