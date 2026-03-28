// Package debug: --debug flag and slog.Debug bridge.
package debug

import (
	"fmt"
	"log"
	"log/slog"
)

var enabled bool

// True after Init when --debug is set.
func Enabled() bool {
	return enabled
}

// Init turns on debug mode from the CLI flag only.
func Init(flagDebug bool) {
	enabled = flagDebug
}

// Standard library log format when debug is on.
func ConfigureLog() {
	if !enabled {
		return
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix("[tunnelbypass] ")
}

// slog.Debug when debug is enabled.
func Logf(format string, args ...interface{}) {
	if !enabled {
		return
	}
	slog.Debug(fmt.Sprintf(format, args...))
}
