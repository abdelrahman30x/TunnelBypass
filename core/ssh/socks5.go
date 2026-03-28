package tbssh

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"
)

// StartSOCKS5IfConfigured is reserved; SOCKS5 is not configured via environment.
func StartSOCKS5IfConfigured(ctx context.Context, log *slog.Logger) {
	_ = ctx
	_ = log
}

func serveSOCKS5(ctx context.Context, listenAddr string, log *slog.Logger) error {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Info("socks5: listening (tcp connect only)", "addr", listenAddr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		go handleSOCKS5Client(ctx, conn, log)
	}
}

func handleSOCKS5Client(ctx context.Context, conn net.Conn, log *slog.Logger) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	buf := make([]byte, 256)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 5 {
		return
	}
	nmeth := int(buf[1])
	if nmeth <= 0 || nmeth > len(buf)-2 {
		return
	}
	if _, err := io.ReadFull(conn, buf[:nmeth]); err != nil {
		return
	}
	// No authentication
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[0] != 5 || buf[1] != 1 || buf[2] != 0 {
		return
	}
	var host string
	switch buf[3] {
	case 1:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		host = net.IP(buf[:4]).String()
	case 3:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		l := int(buf[0])
		if l <= 0 || l > len(buf) {
			return
		}
		if _, err := io.ReadFull(conn, buf[:l]); err != nil {
			return
		}
		host = string(buf[:l])
	case 4:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		host = net.IP(buf[:16]).String()
	default:
		_, _ = conn.Write([]byte{5, 8, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(buf[:2])
	dest := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	_ = conn.SetDeadline(time.Time{})
	d := net.Dialer{Timeout: 20 * time.Second}
	remote, err := d.DialContext(ctx, "tcp", dest)
	if err != nil {
		log.Debug("socks5: dial failed", "dest", dest, "err", err)
		_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()

	local := remote.LocalAddr()
	ipStr, portStr, _ := net.SplitHostPort(local.String())
	ip := net.ParseIP(ipStr)
	var repHead []byte
	if ip4 := ip.To4(); ip4 != nil {
		repHead = []byte{5, 0, 0, 1}
		repHead = append(repHead, ip4...)
	} else if ip != nil {
		repHead = []byte{5, 0, 0, 4}
		repHead = append(repHead, ip.To16()...)
	} else {
		repHead = []byte{5, 0, 0, 1, 0, 0, 0, 0}
	}
	pn64, _ := strconv.ParseUint(portStr, 10, 16)
	var pbuf [2]byte
	binary.BigEndian.PutUint16(pbuf[:], uint16(pn64))
	repHead = append(repHead, pbuf[:]...)
	if _, err := conn.Write(repHead); err != nil {
		return
	}

	go func() {
		_, _ = io.Copy(remote, conn)
		_ = remote.Close()
	}()
	_, _ = io.Copy(conn, remote)
}
