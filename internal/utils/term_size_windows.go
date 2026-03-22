//go:build windows

package utils

import (
	"os"

	"golang.org/x/sys/windows"
)

// terminalSize returns console size in characters (cols, rows). Returns (0,0) if unknown.
func terminalSize() (int, int) {
	h := windows.Handle(os.Stdout.Fd())
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return 0, 0
	}
	cols := int(info.Window.Right-info.Window.Left) + 1
	rows := int(info.Window.Bottom-info.Window.Top) + 1
	if cols < 0 || rows < 0 {
		return 0, 0
	}
	return cols, rows
}
