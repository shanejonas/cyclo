package patterns

import "testing"

func TestChopBasic(t *testing.T) {
	// PDG: 0 -> 1 -> 2 -> 3, plus 0 -> 2 (skip edge).
	// Chop from 0 to 3 should include all nodes on paths.
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Op", Line: 3},
			{Kind: "Return", Line: 4},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
			{From: 2, To: 3, Kind: Data},
			{From: 0, To: 2, Kind: Data},
		},
	}
	chop := Chop(pdg, 0, 3)
	if len(chop) != 4 {
		t.Errorf("expected 4 nodes in chop, got %d: %v", len(chop), chop)
	}
}

func TestChopExcludesIrrelevant(t *testing.T) {
	// PDG: 0 -> 1 -> 3, and 2 -> 3 (2 is not on path from 0).
	// Chop from 0 to 3 should exclude 2.
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Param", Line: 3},
			{Kind: "Return", Line: 4},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 3, Kind: Data},
			{From: 2, To: 3, Kind: Data},
		},
	}
	chop := Chop(pdg, 0, 3)
	for _, n := range chop {
		if n == 2 {
			t.Errorf("node 2 should not be in chop from 0 to 3, got %v", chop)
		}
	}
	if len(chop) != 3 {
		t.Errorf("expected 3 nodes in chop, got %d: %v", len(chop), chop)
	}
}
