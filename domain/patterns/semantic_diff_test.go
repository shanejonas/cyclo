package patterns

import "testing"

func TestSemanticDiffIdentical(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", TyClass: "int", Line: 1},
			{Kind: "Op", TyClass: "int", Line: 2},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
		},
	}
	// Same structure, different lines: cosmetic.
	pdg2 := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", TyClass: "int", Line: 10},
			{Kind: "Op", TyClass: "int", Line: 20},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
		},
	}
	result := SemanticDiff(pdg, pdg2)
	if result.Semantic {
		t.Errorf("identical structure should be cosmetic, got: %s", result.Reason)
	}
}

func TestSemanticDiffChanged(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", TyClass: "int", Line: 1},
			{Kind: "Op", TyClass: "int", Line: 2},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
		},
	}
	// Added a node: semantic change.
	pdg2 := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", TyClass: "int", Line: 1},
			{Kind: "Op", TyClass: "int", Line: 2},
			{Kind: "Op", TyClass: "int", Line: 3},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
		},
	}
	result := SemanticDiff(pdg, pdg2)
	if !result.Semantic {
		t.Errorf("changed structure should be semantic, got: %s", result.Reason)
	}
}
