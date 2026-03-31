package utils

import "testing"

func TestListenAddrNeedsIPv4WildcardFix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		listen string
		want   bool
	}{
		{"", true},
		{"::", true},
		{"[::]", true},
		{"::0", true},
		{"[::]:443", true},
		{":443", true},
		{":8443", true},
		{"0.0.0.0", false},
		{"0.0.0.0:443", false},
		{"127.0.0.1", false},
		{"127.0.0.1:443", false},
		{"[::1]:443", false},
	}
	for _, tc := range cases {
		got := ListenAddrNeedsIPv4WildcardFix(tc.listen)
		if got != tc.want {
			t.Errorf("ListenAddrNeedsIPv4WildcardFix(%q) = %v, want %v", tc.listen, got, tc.want)
		}
	}
}

func TestListenPortFromField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		listen string
		port   int
		ok     bool
	}{
		{":443", 443, true},
		{"0.0.0.0:8443", 8443, true},
		{"[::]:443", 443, true},
		{"", 0, false},
	}
	for _, tc := range cases {
		p, ok := ListenPortFromField(tc.listen)
		if ok != tc.ok || (ok && p != tc.port) {
			t.Errorf("ListenPortFromField(%q) = %d, %v; want %d, %v", tc.listen, p, ok, tc.port, tc.ok)
		}
	}
}
