// Package tblog: default slog setup (TB_DEBUG, TB_LOG_LEVEL, TB_LOG).
package tblog

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

var root *slog.Logger

var runtimeAttrs []any

// Extra slog attributes (e.g. data_dir); replaces prior runtime attrs; call after paths are known.
func SetRuntimeAttrs(kv ...any) {
	runtimeAttrs = append([]any(nil), kv...)
	rebuildRoot()
}

// Default slog logger from TB_LOG, TB_LOG_LEVEL, TB_DEBUG.
func Init() {
	rebuildRoot()
}

func rebuildRoot() {
	level, addSource := parseLevel()
	opts := &slog.HandlerOptions{Level: level, AddSource: addSource}
	var h slog.Handler
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TB_LOG")), "json") {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	base := slog.New(h)
	pairs := []any{"app", "tunnelbypass"}
	pairs = append(pairs, runtimeAttrs...)
	root = base.With(pairs...)
	slog.SetDefault(root)
}

func parseLevel() (lvl slog.Level, addSource bool) {
	lvl = slog.LevelInfo
	addSource = false

	if v := strings.TrimSpace(strings.ToLower(os.Getenv("TB_LOG_LEVEL"))); v != "" {
		switch v {
		case "debug":
			lvl = slog.LevelDebug
			addSource = true
		case "info":
			lvl = slog.LevelInfo
		case "warn", "warning":
			lvl = slog.LevelWarn
		case "error":
			lvl = slog.LevelError
		}
		return lvl, addSource
	}

	if os.Getenv("TB_DEBUG") == "1" || strings.EqualFold(os.Getenv("TB_DEBUG"), "true") ||
		os.Getenv("TB_DEBUG") == "yes" || strings.EqualFold(os.Getenv("TB_DEBUG"), "on") {
		lvl = slog.LevelDebug
		addSource = true
	}
	return lvl, addSource
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

// Integer from env, or defaultVal if unset/invalid.
func IntFromEnv(key string, defaultVal int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}
