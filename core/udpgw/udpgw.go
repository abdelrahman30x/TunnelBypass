// Package udpgw: badvpn-style UDP-over-TCP gateway (built-in or external binary).
package udpgw

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	FlagKeepAlive = 1 << 0
	FlagRebind    = 1 << 1
	FlagDNS       = 1 << 2
	FlagIPv6      = 1 << 3
)

type Mode int

const (
	ModeAuto Mode = iota
	ModeInternal
	ModeExternal
)

type Options struct {
	Port                 int
	Mode                 Mode
	ExternalPath         string // badvpn-udpgw or compatible; empty uses TB_UDPGW_BINARY / auto-detect
	Logger               *slog.Logger
	MaxConcurrentClients int
}

type connEntry struct {
	id     uint16
	flags  byte
	remote *net.UDPAddr
	udp    *net.UDPConn
}

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 64*1024)
		return &b
	},
}

var frameHdrPool = sync.Pool{
	New: func() any {
		b := make([]byte, 2)
		return &b
	},
}

func ModeFromEnv() Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TB_UDPGW_MODE"))) {
	case "internal":
		return ModeInternal
	case "external":
		return ModeExternal
	default:
		return ModeAuto
	}
}

func ExternalPathFromEnv() string {
	return strings.TrimSpace(os.Getenv("TB_UDPGW_BINARY"))
}

func (o Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

func maxConcurrentClients(o Options) int {
	if o.MaxConcurrentClients > 0 {
		return o.MaxConcurrentClients
	}
	if v := strings.TrimSpace(os.Getenv("TB_UDPGW_MAX_CLIENTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 512
}

const tcpClientReadIdle = 3 * time.Minute

func resolveMode(o Options) Mode {
	m := o.Mode
	if m == ModeAuto {
		m = ModeFromEnv()
	}
	if m == ModeAuto {
		p := o.ExternalPath
		if p == "" {
			p = ExternalPathFromEnv()
		}
		if p != "" {
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				return ModeExternal
			}
		}
		return ModeInternal
	}
	return m
}

func resolveExternalPath(o Options) string {
	if o.ExternalPath != "" {
		return o.ExternalPath
	}
	return ExternalPathFromEnv()
}

func Run(ctx context.Context, o Options) error {
	if o.Port <= 0 {
		o.Port = 7300
	}
	mode := resolveMode(o)
	forcedExternal := o.Mode == ModeExternal || ModeFromEnv() == ModeExternal
	switch mode {
	case ModeExternal:
		path := resolveExternalPath(o)
		if path == "" {
			if forcedExternal {
				return fmt.Errorf("udpgw: external mode requires ExternalPath or TB_UDPGW_BINARY")
			}
			o.logger().Warn("udpgw: no external binary path; using internal server")
			return runInternal(ctx, o)
		}
		return runExternal(ctx, o, path, forcedExternal)
	default:
		return runInternal(ctx, o)
	}
}

func runExternal(ctx context.Context, o Options, bin string, forcedExternal bool) error {
	addr := fmt.Sprintf("127.0.0.1:%d", o.Port)
	// badvpn-udpgw style
	args := []string{"--listen-addr", addr}
	if extra := strings.TrimSpace(os.Getenv("TB_UDPGW_ARGS")); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	o.logger().Info("udpgw: starting external", "binary", bin, "args", args)
	if err := cmd.Start(); err != nil {
		if forcedExternal {
			return fmt.Errorf("udpgw external start: %w", err)
		}
		o.logger().Warn("udpgw: external failed to start, using internal", "err", err)
		return runInternal(ctx, o)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case err := <-done:
		if err != nil {
			if forcedExternal {
				return fmt.Errorf("udpgw external: %w", err)
			}
			o.logger().Warn("udpgw: external exited, falling back to internal", "err", err)
			return runInternal(ctx, o)
		}
		return nil
	}
}

func runInternal(ctx context.Context, o Options) error {
	addr := fmt.Sprintf("127.0.0.1:%d", o.Port)
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	o.logger().Info("udpgw: internal listener", "addr", addr)

	var wg sync.WaitGroup
	defer wg.Wait()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			handleClient(conn, o.logger())
		}(c)
	}
}

func handleClient(c net.Conn, log *slog.Logger) {
	defer c.Close()

	var wmu sync.Mutex
	connMap := make(map[uint16]*connEntry)
	var cmapMu sync.Mutex

	writeFrame := func(payload []byte) error {
		if len(payload) > 0xFFFF {
			return fmt.Errorf("payload too large")
		}
		hptr := frameHdrPool.Get().(*[]byte)
		h := (*hptr)[:2]
		binary.LittleEndian.PutUint16(h, uint16(len(payload)))
		wmu.Lock()
		defer wmu.Unlock()
		if _, err := c.Write(h); err != nil {
			frameHdrPool.Put(hptr)
			return err
		}
		_, err := c.Write(payload)
		frameHdrPool.Put(hptr)
		return err
	}

	readLoopDone := make(chan struct{})
	go func() {
		<-readLoopDone
		cmapMu.Lock()
		defer cmapMu.Unlock()
		for _, uc := range connMap {
			_ = uc.udp.Close()
		}
	}()

	for {
		_ = c.SetReadDeadline(time.Now().Add(tcpClientReadIdle))
		var hdr [2]byte
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			close(readLoopDone)
			return
		}
		n := int(binary.LittleEndian.Uint16(hdr[:]))
		if n < 3 {
			close(readLoopDone)
			return
		}
		bufp := bufPool.Get().(*[]byte)
		buf := (*bufp)[:n]
		if len(buf) < n {
			buf = make([]byte, n)
		} else {
			buf = buf[:n]
		}
		_ = c.SetReadDeadline(time.Now().Add(tcpClientReadIdle))
		if _, err := io.ReadFull(c, buf); err != nil {
			bufPool.Put(bufp)
			close(readLoopDone)
			return
		}

		flags := buf[0]
		conID := binary.LittleEndian.Uint16(buf[1:3])

		if flags&FlagKeepAlive != 0 {
			bufPool.Put(bufp)
			continue
		}

		var remote *net.UDPAddr
		var data []byte
		var isV6 bool

		if flags&FlagIPv6 != 0 {
			if len(buf) < 3+16+2 {
				bufPool.Put(bufp)
				continue
			}
			ip := net.IP(make([]byte, 16))
			copy(ip, buf[3:19])
			port := int(binary.BigEndian.Uint16(buf[19:21]))
			remote = &net.UDPAddr{IP: ip, Port: port}
			data = buf[21:]
			isV6 = true
		} else {
			if len(buf) < 3+4+2 {
				bufPool.Put(bufp)
				continue
			}
			ip := net.IPv4(buf[3], buf[4], buf[5], buf[6])
			port := int(binary.BigEndian.Uint16(buf[7:9]))
			remote = &net.UDPAddr{IP: ip, Port: port}
			data = buf[9:]
		}

		if flags&FlagDNS != 0 {
			remote.Port = 53
		}

		cmapMu.Lock()
		uc, ok := connMap[conID]
		if !ok || (flags&FlagRebind) != 0 || !udpAddrEqual(uc.remote, remote) {
			if ok {
				_ = uc.udp.Close()
			}
			udpConn, err := net.ListenUDP("udp", nil)
			if err != nil {
				cmapMu.Unlock()
				bufPool.Put(bufp)
				log.Debug("udpgw: ListenUDP failed", "err", err)
				continue
			}
			uc = &connEntry{id: conID, flags: flags, remote: remote, udp: udpConn}
			connMap[conID] = uc

			go func(localUC *connEntry, v6 bool) {
				rbptr := bufPool.Get().(*[]byte)
				rb := (*rbptr)[:cap(*rbptr)]
				defer bufPool.Put(rbptr)
				for {
					_ = localUC.udp.SetReadDeadline(time.Now().Add(60 * time.Second))
					rn, raddr, err := localUC.udp.ReadFromUDP(rb)
					if err != nil {
						ne, ok := err.(net.Error)
						if ok && ne.Timeout() {
							continue
						}
						return
					}
					payload := buildResponsePayload(localUC.id, v6, raddr, rb[:rn])
					if err := writeFrame(payload); err != nil {
						return
					}
				}
			}(uc, isV6)
		}
		cmapMu.Unlock()

		_, _ = uc.udp.WriteToUDP(data, uc.remote)
		bufPool.Put(bufp)
	}
}

func buildResponsePayload(conID uint16, v6 bool, raddr *net.UDPAddr, data []byte) []byte {
	respFlags := byte(0)
	if v6 || (raddr.IP.To4() == nil) {
		respFlags |= FlagIPv6
	}
	// Pre-size: flags + id + addr + port + data
	n := 3 + 4 + 2 + len(data)
	if respFlags&FlagIPv6 != 0 {
		n = 3 + 16 + 2 + len(data)
	}
	payload := make([]byte, 0, n)
	payload = append(payload, respFlags)
	tmp := make([]byte, 2)
	binary.LittleEndian.PutUint16(tmp, conID)
	payload = append(payload, tmp...)

	if respFlags&FlagIPv6 != 0 {
		ip16 := raddr.IP.To16()
		if ip16 == nil {
			ip16 = net.IPv6zero
		}
		payload = append(payload, ip16...)
	} else {
		ip4 := raddr.IP.To4()
		if ip4 == nil {
			ip4 = net.IPv4zero
		}
		payload = append(payload, ip4...)
	}
	pb := make([]byte, 2)
	binary.BigEndian.PutUint16(pb, uint16(raddr.Port))
	payload = append(payload, pb...)
	payload = append(payload, data...)
	return payload
}

func udpAddrEqual(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Port != b.Port {
		return false
	}
	return a.IP.Equal(b.IP)
}

func RunLegacy(port int) error {
	return Run(context.Background(), Options{Port: port, Mode: ModeInternal})
}
