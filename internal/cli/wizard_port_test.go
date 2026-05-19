package cli

import (
	"testing"

	"tunnelbypass/core/types"
)

func TestParseWizardPortChoice(t *testing.T) {
	choices := wizardPortChoices("reality")
	for _, tc := range []struct {
		name       string
		raw        string
		wantPort   int
		wantCustom bool
		wantOK     bool
	}{
		{name: "empty defaults to 443", raw: "", wantPort: 443, wantOK: true},
		{name: "first list item is 443", raw: "1", wantPort: 443, wantOK: true},
		{name: "second list item is 8443", raw: "2", wantPort: 8443, wantOK: true},
		{name: "custom marker", raw: "c", wantCustom: true, wantOK: true},
		{name: "direct custom port", raw: "1443", wantPort: 1443, wantOK: true},
		{name: "too high", raw: "70000", wantOK: false},
		{name: "text invalid", raw: "abc", wantOK: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotPort, gotCustom, gotOK := parseWizardPortChoice(tc.raw, choices, types.DefaultTLSTunnelListenPort)
			if gotPort != tc.wantPort || gotCustom != tc.wantCustom || gotOK != tc.wantOK {
				t.Fatalf("got port=%d custom=%v ok=%v, want port=%d custom=%v ok=%v",
					gotPort, gotCustom, gotOK, tc.wantPort, tc.wantCustom, tc.wantOK)
			}
		})
	}
}

func TestWizardPortChoicesIncludeTransportDefaults(t *testing.T) {
	for _, tc := range []struct {
		transport string
		want      int
	}{
		{transport: "wireguard", want: types.DefaultWireGuardListenPort},
		{transport: "ssh", want: types.DefaultSSHSpecListenPort},
		{transport: "xdns", want: types.DefaultXDNSListenPort},
		{transport: "mdns", want: types.DefaultMDNSListenPort},
	} {
		t.Run(tc.transport, func(t *testing.T) {
			found := false
			for _, c := range wizardPortChoices(tc.transport) {
				if c.Port == tc.want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("choices for %s did not include %d", tc.transport, tc.want)
			}
		})
	}
}
