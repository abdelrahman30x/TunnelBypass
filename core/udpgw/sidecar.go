package udpgw

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"tunnelbypass/internal/utils"
)

// UDPGW sidecar; binds o.Port or 7300. listenAddr, stop (waits for exit), err if listen never ready.
func StartSidecar(parent context.Context, o Options, host string) (listenAddr string, stop func(), err error) {
	if host == "" {
		host = "127.0.0.1"
	}
	p := o.Port
	if p <= 0 {
		p = 7300
	}
	p = utils.AllocatePort("tcp", p)
	if p == 0 {
		p = o.Port
		if p <= 0 {
			p = 7300
		}
	}
	o.Port = p

	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = Run(ctx, o)
	}()

	addr := net.JoinHostPort(host, strconv.Itoa(p))
	if !waitTCPReady(ctx, addr, 20*time.Second) {
		cancel()
		wg.Wait()
		return "", nil, fmt.Errorf("udpgw: did not listen on %s", addr)
	}

	stopFn := func() {
		cancel()
		wg.Wait()
	}
	return addr, stopFn, nil
}

func waitTCPReady(ctx context.Context, addr string, total time.Duration) bool {
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		c, err := net.DialTimeout("tcp", addr, 80*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(40 * time.Millisecond):
		}
	}
	return false
}
