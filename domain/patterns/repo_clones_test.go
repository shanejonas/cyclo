package patterns

import (
	"testing"
)

// repoChainKinds is the 10-node linear chain used by the repo-clone tests:
// param -> op -> op -> call -> let -> op -> branch -> call -> op -> return.
func repoChainKinds() []NodeKind {
	return []NodeKind{Param, Op, Op, Call, Let, Op, Branch, Call, Op, Return}
}

// repoChainPdg builds a 10-node linear PDG. lineBase shifts all line
// numbers; kindSwap >= 0 replaces that node's kind with Call, producing a
// structurally different (non-isomorphic) variant.
func repoChainPdg(lineBase, kindSwap int) *Pdg {
	kinds := repoChainKinds()
	nodes := make([]PdgNode, len(kinds))
	for i, k := range kinds {
		if i == kindSwap {
			k = Call
		}
		n := PdgNode{Kind: k, Line: lineBase + i}
		switch i {
		case 0:
			n.TyClass = "int"
		case 1, 8:
			n.Detail = "add:int"
		case 2:
			n.Detail = "mul:int"
		case 3:
			n.SigClass = "fn(int)->int"
			n.CalleeID = "pkg.F"
		case 5:
			n.Detail = "cmp:<"
		case 7:
			n.SigClass = "fn()->()"
			n.CalleeID = "pkg.G"
		}
		if k == Call && n.SigClass == "" {
			n.SigClass = "fn()->()"
			n.CalleeID = "pkg.H"
		}
		nodes[i] = n
	}
	var edges []PdgEdge
	for i := 0; i+1 < len(nodes); i++ {
		edges = append(edges, PdgEdge{From: i, To: i + 1, Kind: Data})
	}
	return &Pdg{Nodes: nodes, Edges: edges}
}

func repoAllTen() []int {
	return []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
}

func TestMineRepoClonesFindsSharedSubgraph(t *testing.T) {
	pdgs := map[string]*Pdg{
		"f1": repoChainPdg(1, -1),
		"f2": repoChainPdg(100, -1),
		"f3": repoChainPdg(200, 5), // one kind swapped: not a clone
	}
	groups := MineRepoClones(pdgs)
	if len(groups) != 1 {
		t.Fatalf("expected 1 clone group, got %d", len(groups))
	}
	got := map[string]bool{}
	for _, sg := range groups[0] {
		got[sg.FuncID] = true
	}
	if len(got) != 2 || !got["f1"] || !got["f2"] {
		t.Fatalf("expected group {f1 f2}, got %v", got)
	}
	// Deterministic across runs (fixed LSH seed, sorted output).
	again := MineRepoClones(pdgs)
	if len(again) != 1 || len(again[0]) != len(groups[0]) {
		t.Fatal("MineRepoClones is not deterministic")
	}
}

func TestPartitionIsomorphicFiltersFalsePositives(t *testing.T) {
	// Hand-built cluster: f3's PDG is not isomorphic to f1/f2's, so the
	// fine filter must drop it even though LSH grouped them.
	pdgs := map[string]*Pdg{
		"f1": repoChainPdg(1, -1),
		"f2": repoChainPdg(2, -1),
		"f3": repoChainPdg(3, 5),
	}
	members := []Subgraph{
		{FuncID: "f1", Nodes: repoAllTen()},
		{FuncID: "f2", Nodes: repoAllTen()},
		{FuncID: "f3", Nodes: repoAllTen()},
	}
	classes := partitionIsomorphic(members, pdgs)
	if len(classes) != 1 {
		t.Fatalf("expected 1 isomorphic class, got %d", len(classes))
	}
	for _, sg := range classes[0] {
		if sg.FuncID == "f3" {
			t.Fatal("non-isomorphic f3 survived the fine filter")
		}
	}
}

func TestMineRepoClonesEmpty(t *testing.T) {
	if got := MineRepoClones(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	if got := MineRepoClones(map[string]*Pdg{"f1": nil}); len(got) != 0 {
		t.Fatalf("expected no groups for nil PDG, got %v", got)
	}
}

func TestMineRepoClonesSingleFunction(t *testing.T) {
	pdgs := map[string]*Pdg{"f1": repoChainPdg(1, -1)}
	if got := MineRepoClones(pdgs); len(got) != 0 {
		t.Fatalf("expected no groups for a single function, got %v", got)
	}
}
