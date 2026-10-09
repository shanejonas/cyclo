package patterns

import "testing"

func TestThinSliceExcludesControl(t *testing.T) {
	// PDG: 0 (param) -> 1 (op, data), 0 -> 2 (ctrl), 2 -> 1 (ctrl).
	// Thin slice from 1 should include 0 (data producer) but not 2 (control).
	pdg := &Pdg{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Ctrl", Line: 3},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 0, To: 2, Kind: Data},
			{From: 2, To: 1, Kind: Ctrl},
		},
	}
	slice := ThinSlice(pdg, 1)
	has0, has2 := false, false
	for _, n := range slice {
		if n == 0 {
			has0 = true
		}
		if n == 2 {
			has2 = true
		}
	}
	if !has0 {
		t.Errorf("thin slice should include data producer 0, got %v", slice)
	}
	if has2 {
		t.Errorf("thin slice should exclude control node 2, got %v", slice)
	}
}
