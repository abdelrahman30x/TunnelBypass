package cfg

import (
	"strings"
	"testing"
)

func TestValidateSpecUnknownTransport(t *testing.T) {
	err := ValidateSpec("bogus", SpecFile{})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown transport error, got %v", err)
	}
}

func TestValidateSpecPort(t *testing.T) {
	err := ValidateSpec("ssh", SpecFile{Port: 70000})
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected port error, got %v", err)
	}
}

func TestValidateSpecTransportMismatch(t *testing.T) {
	err := ValidateSpec("ssh", SpecFile{Transport: "hysteria"})
	if err == nil || !strings.Contains(err.Error(), "disagrees") {
		t.Fatalf("expected disagree error, got %v", err)
	}
}

func TestApplySpecDefaultsReality(t *testing.T) {
	f := SpecFile{}
	ApplySpecDefaults("reality", &f)
	if f.Port != 443 {
		t.Fatalf("port %d", f.Port)
	}
}

func TestApplySpecDefaultsHysteria(t *testing.T) {
	f := SpecFile{}
	ApplySpecDefaults("hysteria", &f)
	if f.Port != 8443 {
		t.Fatalf("port %d", f.Port)
	}
}

func TestLoadSpecReaderJSONUnknownField(t *testing.T) {
	_, err := LoadSpecReader(strings.NewReader(`{"transport":"ssh","bogus":1}`), "json")
	if err == nil {
		t.Fatal("expected error for unknown json field")
	}
}

func TestSpecToConfigOptions(t *testing.T) {
	f := SpecFile{ServerAddr: "203.0.113.1", Port: 443}
	o := SpecToConfigOptions("reality", f)
	if o.ServerAddr != "203.0.113.1" || o.Port != 443 {
		t.Fatalf("got %+v", o)
	}
}
