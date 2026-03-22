package portable

import (
	"fmt"
	"sort"
	"strings"
)

var registry = map[string]func() Transport{}

// Register adds a transport factory (typically from init).
func Register(name string, newFn func() Transport) {
	n := strings.ToLower(strings.TrimSpace(name))
	registry[n] = newFn
}

// Registered transport names, sorted (help text).
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func lookup(name string) (Transport, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	f, ok := registry[n]
	if !ok {
		return nil, fmt.Errorf("unknown portable transport %q", name)
	}
	return f(), nil
}

// New transport instance by name (same registry as RunNamed).
func Get(name string) (Transport, error) {
	return lookup(name)
}
