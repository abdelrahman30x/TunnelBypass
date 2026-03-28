// Package tblog: default slog setup.
package tblog

import (
	"log/slog"
	"os"
)

var root *slog.Logger

var runtimeAttrs []any

var debugLogging bool

// Extra slog attributes (e.g. data_dir); replaces prior runtime attrs; call after paths are known.
func SetRuntimeAttrs(kv ...any) {
	runtimeAttrs = append([]any(nil), kv...)
	rebuildRoot()
}

// ApplyDebug sets slog level from the CLI --debug flag (call after debug.Init).
func ApplyDebug(debug bool) {
	debugLogging = debug
	rebuildRoot()
}

// Init initializes the default slog logger.
func Init() {
	rebuildRoot()
}

func rebuildRoot() {
	level := slog.LevelInfo
	addSource := false
	if debugLogging {
		level = slog.LevelDebug
		addSource = true
	}
	opts := &slog.HandlerOptions{Level: level, AddSource: addSource}
	h := slog.NewTextHandler(os.Stdout, opts)
	base := slog.New(h)
	pairs := []any{"app", "tunnelbypass"}
	pairs = append(pairs, runtimeAttrs...)
	root = base.With(pairs...)
	slog.SetDefault(root)
}

// Root logger (initializes if needed).
func L() *slog.Logger {
	if root == nil {
		Init()
	}
	return root
}

// Logger with sub=<name>.
func Sub(name string) *slog.Logger {
	return L().With("sub", name)
}

// IntFromEnv returns defaultVal (call sites use fixed defaults; kept for API compatibility).
func IntFromEnv(_ string, defaultVal int) int {
	return defaultVal
}
