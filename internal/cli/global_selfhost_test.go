package cli

import (
	"reflect"
	"testing"
)

func TestInjectGlobalSelfHostPrepends(t *testing.T) {
	earlySelfHostScan = true
	earlyLanRelaxScan = true
	t.Cleanup(func() {
		earlySelfHostScan = false
		earlyLanRelaxScan = false
	})
	ra := []string{"reality", "--port", "8443"}
	got := injectGlobalSelfHostPrepends(ra)
	want := []string{"--self-host", "--lan-relax", "reality", "--port", "8443"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestInjectGlobalSelfHostPrepends_noDuplicate(t *testing.T) {
	earlySelfHostScan = true
	t.Cleanup(func() { earlySelfHostScan = false })
	ra := []string{"--self-host", "reality"}
	got := injectGlobalSelfHostPrepends(ra)
	if !reflect.DeepEqual(got, ra) {
		t.Fatalf("got %#v want %#v", got, ra)
	}
}
