package utils

import (
	"bytes"
	"testing"
)

func TestStripUTF8BOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	with := append(append([]byte(nil), bom...), []byte(`{"a":1}`)...)
	out := StripUTF8BOM(with)
	if !bytes.Equal(out, []byte(`{"a":1}`)) {
		t.Fatalf("got %q", out)
	}
	if len(StripUTF8BOM([]byte("no bom"))) != 6 {
		t.Fatal("should not strip without BOM")
	}
}
