package cli

import (
	"reflect"
	"testing"
)

func TestReorderRunArgsForFlagParse_userCase(t *testing.T) {
	in := []string{
		"--portable", "--dry-run", "--self-host", "reality", "--port", "8443", "--sni", "example.com",
	}
	want := []string{
		"--portable", "--dry-run", "--self-host", "--port", "8443", "--sni", "example.com", "reality",
	}
	got := reorderRunArgsForFlagParse(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestReorderRunArgsForFlagParse_noChangeWhenTransportLast(t *testing.T) {
	in := []string{"--portable", "--self-host", "--port", "8443", "reality"}
	got := reorderRunArgsForFlagParse(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("got %#v want %#v", got, in)
	}
}

func TestReorderRunArgsForFlagParse_noChangeMultipleTransports(t *testing.T) {
	in := []string{"reality", "hysteria", "--port", "443"}
	got := reorderRunArgsForFlagParse(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("ambiguous: should not reorder, got %#v", got)
	}
}

func TestReorderRunArgsForFlagParse_dataDirPathNotTransport(t *testing.T) {
	in := []string{"--data-dir", `D:\foo`, "reality"}
	got := reorderRunArgsForFlagParse(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("got %#v want %#v", got, in)
	}
}
