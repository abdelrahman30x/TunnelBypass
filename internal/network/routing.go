package network

// BuildXrayPrivateRule returns routing rule elements for private IP handling.
// Returns nil when AllowAllPrivate (omit the rule entirely).
func BuildXrayPrivateRule(mode PrivateRoutingMode) []interface{} {
	switch mode {
	case AllowAllPrivate:
		// Empty rules array (not JSON null) for Xray configs.
		return []interface{}{}
	default:
		return []interface{}{
			map[string]interface{}{
				"type":        "field",
				"ip":          []string{"geoip:private"},
				"outboundTag": "block",
			},
		}
	}
}
