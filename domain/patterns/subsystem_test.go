package patterns

import (
	"sort"
	"testing"
)

// callFact builds a FuncFacts whose PDG calls each target in callees.
func callFact(id, pkg string, callees ...string) *FuncFacts {
	nodes := make([]PdgNode, 0, len(callees))
	for _, c := range callees {
		nodes = append(nodes, PdgNode{Kind: Call, CalleeID: c})
	}
	return &FuncFacts{
		ID:   id,
		Name: id,
		Path: pkg + "/file.go",
		Pdg:  &Pdg{Nodes: nodes},
	}
}

func TestBuildCallGraph(t *testing.T) {
	facts := []*FuncFacts{
		callFact("a", "p", "b", "c"),
		callFact("b", "p", "c"),
		callFact("c", "p"),
	}
	graph := BuildCallGraph(facts)
	if len(graph["a"]) != 2 {
		t.Errorf("a has %d callees, want 2", len(graph["a"]))
	}
	if len(graph["b"]) != 1 || graph["b"][0] != "c" {
		t.Errorf("b callees = %v, want [c]", graph["b"])
	}
	if len(graph["c"]) != 0 {
		t.Errorf("c callees = %v, want []", graph["c"])
	}
}

func TestBuildCallGraphDedupes(t *testing.T) {
	// Calling the same function twice records one edge.
	facts := []*FuncFacts{
		callFact("a", "p", "b", "b", "b"),
		callFact("b", "p"),
	}
	graph := BuildCallGraph(facts)
	if len(graph["a"]) != 1 {
		t.Errorf("a has %d callees, want 1 (deduped)", len(graph["a"]))
	}
}

func TestFindSubsystemsConnectedComponents(t *testing.T) {
	// Build 3 components, each with 60 functions (>= 50 threshold).
	var facts []*FuncFacts
	// chain1: c1f0->c1f1->...->c1f59 (linked chain)
	for i := 0; i < 60; i++ {
		facts = append(facts, callFact("c1f"+testItoa(i), "pkg1"))
	}
	for i := 0; i < 59; i++ {
		facts[i].Pdg.Nodes = []PdgNode{{Kind: Call, CalleeID: facts[i+1].ID}}
	}
	// chain2: c2f0->c2f1->...->c2f59 (linked chain)
	base := len(facts)
	for i := 0; i < 60; i++ {
		facts = append(facts, callFact("c2f"+testItoa(i), "pkg2"))
	}
	for i := 0; i < 59; i++ {
		facts[base+i].Pdg.Nodes = []PdgNode{{Kind: Call, CalleeID: facts[base+i+1].ID}}
	}
	// chain3: 60 functions all calling a common hub (star topology)
	base = len(facts)
	for i := 0; i < 60; i++ {
		facts = append(facts, callFact("c3f"+testItoa(i), "pkg3"))
	}
	hub := callFact("c3hub", "pkg3")
	facts = append(facts, hub)
	for i := 0; i < 60; i++ {
		facts[base+i].Pdg.Nodes = []PdgNode{{Kind: Call, CalleeID: hub.ID}}
	}

	subs := FindSubsystems(facts)
	if len(subs) != 3 {
		t.Fatalf("found %d subsystems, want 3", len(subs))
	}
	for _, s := range subs {
		if len(s) != 60 && len(s) != 61 {
			t.Errorf("subsystem size = %d, want 60 or 61", len(s))
		}
	}
}

// itoa is a simple int-to-string for test IDs.
func testItoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func TestFindSubsystemsDropsSmall(t *testing.T) {
	// 10 connected functions: below the 50 threshold, should be dropped.
	var facts []*FuncFacts
	for i := 0; i < 10; i++ {
		facts = append(facts, callFact("f"+testItoa(i), "p"))
	}
	for i := 0; i < 9; i++ {
		facts[i].Pdg.Nodes = []PdgNode{{Kind: Call, CalleeID: facts[i+1].ID}}
	}
	subs := FindSubsystems(facts)
	if len(subs) != 0 {
		t.Errorf("found %d subsystems, want 0 (too small)", len(subs))
	}
}

func TestFindSubsystemsSplitsLargeByPackage(t *testing.T) {
	// 1100 functions in one component, split across 2 packages.
	// Each package group has 550 >= 50, so both survive the split.
	var facts []*FuncFacts
	for i := 0; i < 1100; i++ {
		pkg := "pkgA"
		if i >= 550 {
			pkg = "pkgB"
		}
		facts = append(facts, callFact("f"+testItoa(i), pkg))
	}
	// Chain them all into one component.
	for i := 0; i < 1099; i++ {
		facts[i].Pdg.Nodes = []PdgNode{{Kind: Call, CalleeID: facts[i+1].ID}}
	}
	subs := FindSubsystems(facts)
	if len(subs) != 2 {
		t.Fatalf("found %d subsystems, want 2 (split by package)", len(subs))
	}
	sizes := []int{len(subs[0]), len(subs[1])}
	sort.Ints(sizes)
	if sizes[0] != 550 || sizes[1] != 550 {
		t.Errorf("subsystem sizes = %v, want [550 550]", sizes)
	}
}

func TestFindSubsystemsIsolatedNodes(t *testing.T) {
	// 60 isolated functions (no calls): each is its own component of size 1,
	// all dropped. No subsystems.
	var facts []*FuncFacts
	for i := 0; i < 60; i++ {
		facts = append(facts, callFact("f"+testItoa(i), "p"))
	}
	subs := FindSubsystems(facts)
	if len(subs) != 0 {
		t.Errorf("found %d subsystems, want 0 (all isolated)", len(subs))
	}
}
