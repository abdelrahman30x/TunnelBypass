//go:build !windows

package terminal

// EnableVTProcessing is a no-op on non-Windows platforms.
func EnableVTProcessing() error {
	return nil
}

// EnableUTF8Console is a no-op on non-Windows platforms (UTF-8 is the default).
func EnableUTF8Console() {}
