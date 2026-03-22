//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableVTProcessing enables Windows virtual terminal sequences for CLI colors.
func EnableVTProcessing() error {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return err
	}
	mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	return windows.SetConsoleMode(handle, mode)
}
