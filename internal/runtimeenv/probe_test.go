package runtimeenv

import "testing"

func TestProbeNoPanic(t *testing.T) {
	t.Parallel()
	p := Probe()
	if p.OS == "" {
		t.Fatal("empty OS")
	}
	s := FormatProbeForDebug(p)
	if s == "" {
		t.Fatal("empty debug line")
	}
	WriteProbeSummary(nil, p) // no-op
}
