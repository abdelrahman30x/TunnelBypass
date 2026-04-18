package cli

import (
	"strings"

	"tunnelbypass/internal/cfg"
)

// canonicalRunTransports are normalized names accepted as run positional transport.
var canonicalRunTransports = map[string]struct{}{
	"reality":     {},
	"vless-ws":    {},
	"vless-grpc":  {},
	"ssh-tls":     {},
	"wss":         {},
	"tls":         {},
	"hysteria":    {},
	"ssh":         {},
	"wireguard":   {},
}

func isKnownRunTransportToken(tok string) bool {
	if strings.HasPrefix(tok, "-") {
		return false
	}
	c := cfg.NormalizeTransport(tok)
	_, ok := canonicalRunTransports[c]
	return ok
}

// reorderRunArgsForFlagParse moves a single early positional transport to the end when
// flags appear after it, so Go's flag package parses all flags (parsing stops at first non-flag).
func reorderRunArgsForFlagParse(args []string) []string {
	var transportIdxs []int
	for i, a := range args {
		if isKnownRunTransportToken(a) {
			transportIdxs = append(transportIdxs, i)
		}
	}
	if len(transportIdxs) != 1 {
		return args
	}
	idx := transportIdxs[0]
	hasFlagAfter := false
	for _, a := range args[idx+1:] {
		if strings.HasPrefix(a, "-") {
			hasFlagAfter = true
			break
		}
	}
	if !hasFlagAfter {
		return args
	}
	tok := args[idx]
	rest := make([]string, 0, len(args)-1)
	rest = append(rest, args[:idx]...)
	rest = append(rest, args[idx+1:]...)
	return append(rest, tok)
}
