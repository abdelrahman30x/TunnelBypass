package portable

import (
	"fmt"
	"strings"
)

// Transports that must start before target, in startup order.
func OrderedDependencies(target string) ([]string, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	var order []string
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var dfs func(string) error
	dfs = func(n string) error {
		if visited[n] {
			return nil
		}
		if visiting[n] {
			return fmt.Errorf("portable: dependency cycle involving %q", n)
		}
		visiting[n] = true
		defer func() {
			visiting[n] = false
			visited[n] = true
		}()
		tr, err := lookup(n)
		if err != nil {
			return err
		}
		for _, d := range tr.Dependencies() {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if _, err := lookup(d); err != nil {
				return fmt.Errorf("unknown dependency %q of %q", d, n)
			}
			if err := dfs(d); err != nil {
				return err
			}
		}
		if n != target {
			order = append(order, n)
		}
		return nil
	}
	if err := dfs(target); err != nil {
		return nil, err
	}
	return order, nil
}

// Edges (from, to): to depends on from.
func AllDependencyEdges() ([][2]string, error) {
	var edges [][2]string
	for _, name := range Names() {
		tr, err := lookup(name)
		if err != nil {
			return nil, err
		}
		n := strings.ToLower(strings.TrimSpace(name))
		for _, d := range tr.Dependencies() {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			if _, err := lookup(d); err != nil {
				return nil, fmt.Errorf("unknown dependency %q of %q", d, n)
			}
			edges = append(edges, [2]string{d, n})
		}
	}
	return edges, nil
}
