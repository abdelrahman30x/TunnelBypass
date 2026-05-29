package sshpayload

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func runPipeHandler(t *testing.T, opt Options) (net.Conn, net.Conn, <-chan struct{}) {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleConn(context.Background(), server, opt)
	}()
	return client, server, done
}

func pipeDialer(backend net.Conn, dialed chan<- struct{}) DialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if dialed != nil {
			close(dialed)
		}
		return backend, nil
	}
}

func readUntilHeaderEnd(t *testing.T, c net.Conn) string {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer c.SetReadDeadline(time.Time{})
	var b strings.Builder
	tmp := make([]byte, 128)
	for {
		n, err := c.Read(tmp)
		if n > 0 {
			b.Write(tmp[:n])
			s := b.String()
			if strings.Contains(s, "\r\n\r\n") || strings.Contains(s, "\n\n") {
				return s
			}
		}
		if err != nil {
			t.Fatalf("read response: %v", err)
		}
	}
}

func TestPatchWebSocketPayloadBridgesExtraBytes(t *testing.T) {
	backendClient, backendServer := net.Pipe()
	defer backendServer.Close()
	dialed := make(chan struct{})

	client, _, done := runPipeHandler(t, Options{
		TargetAddr:  "ssh-backend",
		PayloadPath: "/secret",
		DialContext: pipeDialer(backendClient, dialed),
	})
	defer client.Close()

	req := "PATCH /secret HTTP/1.1\r\nHost: first.example\r\nHost: beta.zoom.us\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\nHELLO"
	if _, err := client.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	resp := readUntilHeaderEnd(t, client)
	if !strings.HasPrefix(resp, "HTTP/1.1 101 Switching Protocols") {
		t.Fatalf("response = %q", resp)
	}
	select {
	case <-dialed:
	case <-time.After(time.Second):
		t.Fatal("backend was not dialed")
	}

	_ = backendServer.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 5)
	if _, err := io.ReadFull(backendServer, buf); err != nil {
		t.Fatalf("backend did not receive extra bytes: %v", err)
	}
	if string(buf) != "HELLO" {
		t.Fatalf("backend got %q, want HELLO", string(buf))
	}

	if _, err := backendServer.Write([]byte("SSH-OK")); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, 6)
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatalf("client did not receive backend data: %v", err)
	}
	if string(got) != "SSH-OK" {
		t.Fatalf("client got %q, want SSH-OK", string(got))
	}
	_ = backendServer.Close()
	<-done
}

func TestGetPayloadReturns200OK(t *testing.T) {
	backendClient, backendServer := net.Pipe()
	defer backendServer.Close()
	client, _, done := runPipeHandler(t, Options{
		TargetAddr:  "ssh-backend",
		PayloadPath: "/secret",
		DialContext: pipeDialer(backendClient, nil),
	})
	defer client.Close()

	if _, err := client.Write([]byte("GET /secret HTTP/1.1\r\nHost: example.com\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	resp := readUntilHeaderEnd(t, client)
	if !strings.HasPrefix(resp, "HTTP/1.1 200 OK") {
		t.Fatalf("response = %q", resp)
	}
	_ = backendServer.Close()
	<-done
}

func TestConnectPayloadReturnsConnectionEstablished(t *testing.T) {
	backendClient, backendServer := net.Pipe()
	defer backendServer.Close()
	client, _, done := runPipeHandler(t, Options{
		TargetAddr:  "ssh-backend",
		PayloadPath: "/secret",
		DialContext: pipeDialer(backendClient, nil),
	})
	defer client.Close()

	if _, err := client.Write([]byte("CONNECT /secret HTTP/1.1\r\nHost: example.com\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	resp := readUntilHeaderEnd(t, client)
	if !strings.HasPrefix(resp, "HTTP/1.1 200 Connection Established") {
		t.Fatalf("response = %q", resp)
	}
	_ = backendServer.Close()
	<-done
}

func TestWrongPathReturns404WithoutDial(t *testing.T) {
	dialed := make(chan struct{})
	client, _, done := runPipeHandler(t, Options{
		TargetAddr:  "ssh-backend",
		PayloadPath: "/secret",
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			close(dialed)
			return nil, nil
		},
	})
	defer client.Close()

	if _, err := client.Write([]byte("PATCH /wrong HTTP/1.1\r\nHost: example.com\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	resp := readUntilHeaderEnd(t, client)
	if !strings.HasPrefix(resp, "HTTP/1.1 404 Not Found") {
		t.Fatalf("response = %q", resp)
	}
	select {
	case <-dialed:
		t.Fatal("backend should not be dialed for wrong path")
	case <-time.After(100 * time.Millisecond):
	}
	<-done
}

func TestOversizedHeaderClosesWithoutDial(t *testing.T) {
	dialed := make(chan struct{})
	client, _, done := runPipeHandler(t, Options{
		TargetAddr:     "ssh-backend",
		PayloadPath:    "/secret",
		MaxHeaderBytes: 32,
		HeaderTimeout:  time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			close(dialed)
			return nil, nil
		},
	})
	defer client.Close()

	if _, err := client.Write([]byte("PATCH /secret HTTP/1.1\r\nHost: " + strings.Repeat("a", 64))); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err := client.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("expected connection close")
	}
	select {
	case <-dialed:
		t.Fatal("backend should not be dialed for oversized header")
	case <-time.After(100 * time.Millisecond):
	}
	<-done
}
