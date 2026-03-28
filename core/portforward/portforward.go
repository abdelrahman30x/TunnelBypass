// Package portforward provides TCP port forwarding (proxy) functionality.
// Used to create a stable external port that forwards to a dynamic internal port.
package portforward

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Forwarder represents a TCP port forwarder.
type Forwarder struct {
	listenAddr    string
	targetAddr    string
	listener      net.Listener
	logger        *slog.Logger
	activeConns   sync.WaitGroup
	closed        atomic.Bool
	connCount     atomic.Int64
	totalBytesIn  atomic.Int64
	totalBytesOut atomic.Int64
}

// Config holds configuration for the forwarder.
type Config struct {
	ListenAddr string       // Address to listen on (e.g., "127.0.0.1:2222")
	TargetAddr string       // Address to forward to (e.g., "127.0.0.1:33506")
	Logger     *slog.Logger // Optional logger
}

// New creates a new port forwarder.
func New(cfg Config) *Forwarder {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Forwarder{
		listenAddr: cfg.ListenAddr,
		targetAddr: cfg.TargetAddr,
		logger:     logger.With("component", "portforward"),
	}
}

// ListenAddr returns the address the forwarder is listening on.
func (f *Forwarder) ListenAddr() string {
	return f.listenAddr
}

// TargetAddr returns the target address connections are forwarded to.
func (f *Forwarder) TargetAddr() string {
	return f.targetAddr
}

// Run starts the forwarder and blocks until ctx is cancelled or an error occurs.
func (f *Forwarder) Run(ctx context.Context) error {
	if f.closed.Load() {
		return fmt.Errorf("forwarder already closed")
	}

	ln, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", f.listenAddr, err)
	}
	f.listener = ln

	// Update listen address in case port was 0 (auto-assigned)
	f.listenAddr = ln.Addr().String()

	f.logger.Info("port forwarder started",
		"listen", f.listenAddr,
		"target", f.targetAddr)

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		f.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if f.closed.Load() {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			default:
				f.logger.Warn("accept error", "err", err)
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		f.activeConns.Add(1)
		go f.handleConnection(ctx, conn)
	}
}

// handleConnection handles a single client connection.
func (f *Forwarder) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer f.activeConns.Done()

	connID := f.connCount.Add(1)
	logger := f.logger.With("conn_id", connID, "client", clientConn.RemoteAddr())

	logger.Debug("new connection")

	// Connect to target
	targetConn, err := net.DialTimeout("tcp", f.targetAddr, 10*time.Second)
	if err != nil {
		logger.Warn("failed to connect to target", "target", f.targetAddr, "err", err)
		clientConn.Close()
		return
	}
	defer targetConn.Close()

	logger.Debug("connected to target", "target", f.targetAddr)

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Target
	go func() {
		defer wg.Done()
		n, _ := io.Copy(targetConn, clientConn)
		f.totalBytesIn.Add(n)
		targetConn.(*net.TCPConn).CloseWrite()
	}()

	// Target -> Client
	go func() {
		defer wg.Done()
		n, _ := io.Copy(clientConn, targetConn)
		f.totalBytesOut.Add(n)
		clientConn.(*net.TCPConn).CloseWrite()
	}()

	// Wait for both directions to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Debug("connection closed normally")
	case <-ctx.Done():
		logger.Debug("connection closed due to context")
	}
}

// Close gracefully closes the forwarder.
func (f *Forwarder) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		if f.listener != nil {
			f.listener.Close()
		}
		// Wait for active connections with timeout
		done := make(chan struct{})
		go func() {
			f.activeConns.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			f.logger.Warn("timeout waiting for connections to close")
		}
		f.logger.Info("port forwarder stopped",
			"total_connections", f.connCount.Load(),
			"bytes_in", f.totalBytesIn.Load(),
			"bytes_out", f.totalBytesOut.Load())
	}
	return nil
}

// Stats returns current forwarder statistics.
func (f *Forwarder) Stats() Stats {
	return Stats{
		ListenAddr:  f.listenAddr,
		TargetAddr:  f.targetAddr,
		Connections: f.connCount.Load(),
		BytesIn:     f.totalBytesIn.Load(),
		BytesOut:    f.totalBytesOut.Load(),
	}
}

// Stats holds forwarder statistics.
type Stats struct {
	ListenAddr  string
	TargetAddr  string
	Connections int64
	BytesIn     int64
	BytesOut    int64
}
