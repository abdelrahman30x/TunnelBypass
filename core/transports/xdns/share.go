package xdns

import (
	"encoding/json"
	"fmt"
	"net/url"

	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
)

// finalMaskJSON returns the URL-encoded finalmask JSON for the vless fm parameter.
func finalMaskJSON(seed string) string {
	fm := map[string]interface{}{
		"udp": []interface{}{
			map[string]interface{}{
				"type": "mkcp-aes128gcm",
				"settings": map[string]interface{}{
					"password": seed,
				},
			},
		},
	}
	b, _ := json.Marshal(fm)
	return url.QueryEscape(string(b))
}

// GenerateVlessURL returns a vless:// sharing link for XDNS (VLESS + mKCP + DNS).
func GenerateVlessURL(opt types.ConfigOptions, seed string) string {
	var endpoint string
	switch {
	case opt.ServerAddr != "":
		endpoint = opt.ServerAddr
	case opt.Host != "":
		endpoint = opt.Host
	default:
		endpoint = "127.0.0.1"
	}

	tag := url.QueryEscape(fmt.Sprintf("TunnelBypass-XDNS-%s", utils.SanitizeForTag(endpoint)))
	// Include both legacy seed/headerType (for older clients) and fm (for Xray v26+).
	return fmt.Sprintf(
		"vless://%s@%s:%d?encryption=none&allowInsecure=1&type=kcp&seed=%s&headerType=%s&fm=%s#%s",
		opt.UUID, endpoint, opt.Port,
		url.QueryEscape(seed),
		url.QueryEscape(opt.KCPHeaderType),
		finalMaskJSON(seed),
		tag,
	)
}
