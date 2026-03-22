//go:build windows

package svcman

import (
	"golang.org/x/sys/windows"
)

// IsPIDAlive reports whether pid exists (Windows).
func IsPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(h)
	return true
}
