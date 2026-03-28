package utils

import "runtime"

// AppName returns the executable name to be used in command suggestions according to the OS.
func AppName() string {
	if runtime.GOOS == "windows" {
		return "tunnelbypass"
	}
	return "./tunnelbypass"
}
