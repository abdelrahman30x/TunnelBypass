//go:build !windows

package utils

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// terminalSize returns terminal size in characters (cols, rows). Returns (0,0) if unknown.
func terminalSize() (int, int) {
	type winsize struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}

	var ws winsize
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, os.Stdout.Fd(), uintptr(unix.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0, 0
	}
	if ws.Col == 0 || ws.Row == 0 {
		return 0, 0
	}
	return int(ws.Col), int(ws.Row)
}
