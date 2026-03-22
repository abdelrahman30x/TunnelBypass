package utils

import (
	"net"
	"strconv"
	"testing"
)

func TestIsPortAvailableWithListener(t *testing.T) {
	// Match IsPortAvailable: bind 0.0.0.0 (not loopback-only — Windows quirk).
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to open listener: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if IsPortAvailable("tcp", port) {
		t.Errorf("IsPortAvailable(tcp, %d) = true; want false (port is bound)", port)
	}
}

func TestIsPortAvailableInvalidPorts(t *testing.T) {
	tests := []int{0, -1, 65536, 99999}
	for _, port := range tests {
		if IsPortAvailable("tcp", port) {
			t.Errorf("IsPortAvailable(tcp, %d) = true; want false (out of range)", port)
		}
	}
}

func TestAllocatePortReturnsUsable(t *testing.T) {
	port := AllocatePort("tcp", 0) // 0 is out of [1,65535] so it falls through to random
	if port == 0 {
		t.Fatal("AllocatePort(tcp, 0) returned 0 — could not allocate any port")
	}
	if port < 1 || port > 65535 {
		t.Errorf("AllocatePort(tcp, 0) = %d; want in [1, 65535]", port)
	}

	lnBind, errBind := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
	if errBind != nil {
		t.Errorf("AllocatePort returned port %d but could not bind to it: %v", port, errBind)
	}
	if lnBind != nil {
		lnBind.Close()
	}
}

func TestIsPortAvailableUnknownNetwork(t *testing.T) {
	if IsPortAvailable("sctp", 8080) {
		t.Error("IsPortAvailable(sctp, 8080) = true; want false (unsupported network)")
	}
}
