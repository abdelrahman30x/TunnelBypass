// Package debug: TB_DEBUG flag and slog.Debug bridge.
package debug

import (
	"fmt"
	"log"
	"log/slog"
	"os"
)

var enabled bool

// True after Init when --debug or TB_DEBUG is set.
func Enabled() bool {
	return enabled
}

// Init turns on debug mode from the flag or TB_DEBUG (1, true, yes, on — case-insensitive).
func Init(flagDebug bool) {
	enabled = flagDebug
	if enabled {
		return
	}
	switch os.Getenv("TB_DEBUG") {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		enabled = true
	}
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
