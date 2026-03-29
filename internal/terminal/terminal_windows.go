//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

const utf8CodePage = 65001

// EnableUTF8Console sets input/output code pages to UTF-8 so paths and non-ASCII text print
// correctly in classic conhost (helps Arabic/CJK usernames and directories).
func EnableUTF8Console() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	setCP := kernel32.NewProc("SetConsoleCP")
	setOutCP := kernel32.NewProc("SetConsoleOutputCP")
	_, _, _ = setCP.Call(uintptr(utf8CodePage))
	_, _, _ = setOutCP.Call(uintptr(utf8CodePage))
}

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
