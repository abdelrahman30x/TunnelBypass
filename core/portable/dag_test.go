package portable

import "testing"

func TestOrderedDependenciesSSH(t *testing.T) {
	deps, err := OrderedDependencies("ssh")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0] != "udpgw" {
		t.Fatalf("got %v want [udpgw]", deps)
	}
}

func TestOrderedDependenciesVless(t *testing.T) {
	deps, err := OrderedDependencies("vless")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Fatalf("vless should have no deps, got %v", deps)
	}
}

func TestOrderedDependenciesWSS(t *testing.T) {
	deps, err := OrderedDependencies("wss")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 || deps[0] != "udpgw" || deps[1] != "ssh" {
		t.Fatalf("got %v want [udpgw ssh]", deps)
	}
}

func TestAllDependencyEdges(t *testing.T) {
	edges, err := AllDependencyEdges()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if e[0] == "udpgw" && e[1] == "ssh" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected udpgw->ssh edge, got %#v", edges)
	}
	foundWSS := false
	for _, e := range edges {
		if e[0] == "ssh" && e[1] == "wss" {
			foundWSS = true
		}
	}
	if !foundWSS {
		t.Fatalf("expected ssh->wss edge, got %#v", edges)
	}
}
