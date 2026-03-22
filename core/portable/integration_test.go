//go:build integration

package portable

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestProbeTCPWithRetry_MockListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := probeTCPWithRetry(ctx, addr, nil, "mock"); err != nil {
		t.Fatal(err)
	}
}
