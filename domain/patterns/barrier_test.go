package patterns

import "testing"

func TestBarrierSliceExcludesBarrier(t *testing.T) {
	// 0 -> 1 -> 2 (data). Barrier on 1: slice from 2 is just {2}.
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Op", Line: 3},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
		},
	}
	slice := BarrierSlice(pdg, 2, map[int]bool{1: true})
	if len(slice) != 1 || slice[0] != 2 {
		t.Errorf("expected [2], got %v", slice)
	}
}

func TestBarrierSliceBlocksTransitive(t *testing.T) {
	// 0 -> 1 -> 2 -> 3. Barrier on 2: slice from 3 is {3}.
	// Nodes 0 and 1 are only reachable through the barrier.
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
		},
	}
	slice := BarrierSlice(pdg, 3, map[int]bool{2: true})
	if len(slice) != 1 || slice[0] != 3 {
		t.Errorf("expected [3], got %v", slice)
	}
}

func TestBarrierSliceKeepsOpenPaths(t *testing.T) {
	// Diamond: 0 -> 1 -> 3 and 0 -> 2 -> 3. Barrier on 1:
	// slice from 3 is {0, 2, 3} — the path through 2 stays open.
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Op", Line: 3},
			{Kind: "Return", Line: 4},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 0, To: 2, Kind: Data},
			{From: 1, To: 3, Kind: Data},
			{From: 2, To: 3, Kind: Data},
		},
	}
	slice := BarrierSlice(pdg, 3, map[int]bool{1: true})
	got := map[int]bool{}
	for _, n := range slice {
		got[n] = true
	}
	for _, want := range []int{0, 2, 3} {
		if !got[want] {
			t.Errorf("expected node %d in slice, got %v", want, slice)
		}
	}
	if got[1] {
		t.Errorf("barrier node 1 should be excluded, got %v", slice)
	}
}

func TestBarrierSliceNoBarriers(t *testing.T) {
	// With no barriers, behaves like a plain backward slice.
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
		},
	}
	slice := BarrierSlice(pdg, 1, nil)
	if len(slice) != 2 {
		t.Errorf("expected 2 nodes, got %v", slice)
	}
}

func TestBarrierSliceLines(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 1},
			{Kind: "Op", Line: 2},
			{Kind: "Op", Line: 3},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
		},
	}
	lines := BarrierSliceLines(pdg, 2, map[int]bool{1: true})
	if len(lines) != 1 || lines[0] != 3 {
		t.Errorf("expected [3], got %v", lines)
	}
}

func TestLinesToBarriers(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: "Param", Line: 10},
			{Kind: "Op", Line: 20},
			{Kind: "Op", Line: 20}, // same line as node 1
			{Kind: "Return", Line: 30},
		},
	}
	barriers := LinesToBarriers(pdg, []int{20})
	if len(barriers) != 2 || !barriers[1] || !barriers[2] {
		t.Errorf("expected barriers {1, 2}, got %v", barriers)
	}
	if barriers[0] || barriers[3] {
		t.Errorf("nodes 0 and 3 should not be barriers, got %v", barriers)
	}
}
