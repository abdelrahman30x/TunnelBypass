//go:build !windows

package terminal

// EnableVTProcessing is a no-op on non-Windows platforms.
func EnableVTProcessing() error {
	return nil
}
