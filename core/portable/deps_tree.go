package portable

import (
	"fmt"
	"strings"
)

// Text dependency tree or edge list for root (empty = all edges).
func FormatDepsTreeText(root string) (string, error) {
	root = strings.TrimSpace(root)
	var b strings.Builder
	if root != "" {
		if _, err := lookup(strings.ToLower(root)); err != nil {
			return "", err
		}
		seen := map[string]bool{}
		var walk func(string, int) error
		walk = func(n string, depth int) error {
			n = strings.ToLower(strings.TrimSpace(n))
			if seen[n] {
				return nil
			}
			seen[n] = true
			indent := strings.Repeat("  ", depth)
			_, _ = fmt.Fprintf(&b, "%s%s\n", indent, n)
			t, err := lookup(n)
			if err != nil {
				return err
			}
			for _, d := range t.Dependencies() {
				d = strings.ToLower(strings.TrimSpace(d))
				if d == "" {
					continue
				}
				if err := walk(d, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walk(strings.ToLower(root), 0); err != nil {
			return "", err
		}
		return b.String(), nil
	}
	edges, err := AllDependencyEdges()
	if err != nil {
		return "", err
	}
	if len(edges) == 0 {
		return "(no dependencies declared)\n", nil
	}
	_, _ = fmt.Fprintf(&b, "Edges: dependency -> dependent\n")
	for _, e := range edges {
		_, _ = fmt.Fprintf(&b, "  %s -> %s\n", e[0], e[1])
	}
	return b.String(), nil
}

// Mermaid flowchart fragment (edges only).
func FormatDepsTreeMermaid(root string) (string, error) {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "flowchart LR\n")
	if root != "" {
		edges, err := subgraphEdges(root)
		if err != nil {
			return "", err
		}
		for _, e := range edges {
			_, _ = fmt.Fprintf(&b, "  %s --> %s\n", mermaidID(e[0]), mermaidID(e[1]))
		}
		return b.String(), nil
	}
	edges, err := AllDependencyEdges()
	if err != nil {
		return "", err
	}
	for _, e := range edges {
		_, _ = fmt.Fprintf(&b, "  %s --> %s\n", mermaidID(e[0]), mermaidID(e[1]))
	}
	return b.String(), nil
}

func mermaidID(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	if s == "" {
		return "unknown"
	}
	return s
}

func subgraphEdges(root string) ([][2]string, error) {
	root = strings.ToLower(strings.TrimSpace(root))
	var out [][2]string
	seenE := map[string]bool{}
	seenN := map[string]bool{}
	var walk func(string) error
	walk = func(n string) error {
		n = strings.ToLower(strings.TrimSpace(n))
		if seenN[n] {
			return nil
		}
		seenN[n] = true
		t, err := lookup(n)
		if err != nil {
			return err
		}
		for _, d := range t.Dependencies() {
			d = strings.ToLower(strings.TrimSpace(d))
			if d == "" {
				continue
			}
			key := d + "|" + n
			if !seenE[key] {
				seenE[key] = true
				out = append(out, [2]string{d, n})
			}
			if err := walk(d); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return out, nil
}
