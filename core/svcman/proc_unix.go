//go:build !windows

package svcman

import (
	"os"
	"syscall"
)

// IsPIDAlive reports whether pid exists (Unix).
func IsPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
