package installer

import (
	"os"
	"strings"
)

// TB_XRAY_VERSION or built-in default.
func EffectiveXrayVersion() string {
	if v := strings.TrimSpace(os.Getenv("TB_XRAY_VERSION")); v != "" {
		return v
	}
	return XrayVersion
}

// EffectiveHysteriaVersion returns TB_HYSTERIA_VERSION or the embedded default.
func EffectiveHysteriaVersion() string {
	if v := strings.TrimSpace(os.Getenv("TB_HYSTERIA_VERSION")); v != "" {
		return v
	}
	return HysteriaVersion
}

// TB_WSTUNNEL_VERSION or built-in default.
func EffectiveWstunnelVersion() string {
	if v := strings.TrimSpace(os.Getenv("TB_WSTUNNEL_VERSION")); v != "" {
		return v
	}
	return WstunnelVersion
}

// TB_STUNNEL_VERSION or built-in default.
func EffectiveStunnelVersion() string {
	if v := strings.TrimSpace(os.Getenv("TB_STUNNEL_VERSION")); v != "" {
		return v
	}
	return StunnelVersion
}
