package patterns

import "testing"

func TestStronglyConnectedBasic(t *testing.T) {
	// 0 -> 1 -> 2 -> 0 (cycle), 3 isolated.
	pdg := &Pdg{
		Nodes: []PdgNode{
			{Kind: "Op", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Op", Line: 3},
			{Kind: "Op", Line: 4},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
			{From: 2, To: 0, Kind: Data},
		},
	}
	sccs := stronglyConnected(pdg)
	if len(sccs) != 1 {
		t.Fatalf("expected 1 SCC, got %d: %v", len(sccs), sccs)
	}
	if len(sccs[0]) != 3 {
		t.Errorf("expected SCC of size 3, got %v", sccs[0])
	}
}

func TestStronglyConnectedNoCycle(t *testing.T) {
	// 0 -> 1 -> 2 (no cycle): no SCCs of size 2+.
	pdg := &Pdg{
		Nodes: []PdgNode{
			{Kind: "Op", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Op", Line: 3},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
		},
	}
	sccs := stronglyConnected(pdg)
	if len(sccs) != 0 {
		t.Errorf("expected no SCCs, got %v", sccs)
	}
}

func TestIsClusterThreshold(t *testing.T) {
	// 10 nodes meets minClusterSize.
	if !isCluster(make([]int, 10), 100) {
		t.Error("10 nodes should be a cluster")
	}
	// 3 nodes of 10 total = 30% >= 25% fraction.
	if !isCluster(make([]int, 3), 10) {
		t.Error("3/10 nodes should be a cluster by fraction")
	}
	// 2 nodes of 100 total = 2% < 25%, and < 10: not a cluster.
	if isCluster(make([]int, 2), 100) {
		t.Error("2/100 nodes should not be a cluster")
	}
}
