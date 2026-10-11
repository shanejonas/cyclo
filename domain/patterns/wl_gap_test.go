package patterns

import "testing"

// gapBase is a 10-node PDG modeling a realistic function: validate, branch,
// loop with two calls, return.
func gapBase() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "string", Line: 1},
			{Kind: Call, CalleeID: "strings.TrimSpace", SigClass: "fn(string) -> string", Line: 2},
			{Kind: Branch, Line: 3},
			{Kind: Call, CalleeID: "fmt.Errorf", SigClass: "fn(string) -> error", Line: 4},
			{Kind: Return, Line: 5},
			{Kind: Loop, Line: 6},
			{Kind: Call, CalleeID: "github.com/me/app.process", SigClass: "fn(string) -> string", Line: 7},
			{Kind: Op, Detail: "arith:+", Line: 8},
			{Kind: Call, CalleeID: "fmt.Println", SigClass: "fn(string) -> ()", Line: 9},
			{Kind: Return, Line: 10},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Ctrl, ArgPos: 0},
			{From: 2, To: 3, Kind: Ctrl, ArgPos: 0},
			{From: 2, To: 5, Kind: Ctrl, ArgPos: 1},
			{From: 3, To: 4, Kind: Data, ArgPos: 0},
			{From: 5, To: 6, Kind: Ctrl, ArgPos: 0},
			{From: 5, To: 7, Kind: Ctrl, ArgPos: 1},
			{From: 1, To: 6, Kind: Data, ArgPos: 0},
			{From: 6, To: 8, Kind: Data, ArgPos: 0},
			{From: 7, To: 6, Kind: Data, ArgPos: 0},
			{From: 6, To: 9, Kind: Data, ArgPos: 0},
		},
	}
}

// dropNode returns a copy of pdg with node idx removed (and its edges).
func dropNode(pdg *MiningGraph, idx int) *MiningGraph {
	out := &MiningGraph{}
	for i, n := range pdg.Nodes {
		if i != idx {
			out.Nodes = append(out.Nodes, n)
		}
	}
	remap := func(v int) int {
		if v > idx {
			return v - 1
		}
		return v
	}
	for _, e := range pdg.Edges {
		if e.From == idx || e.To == idx {
			continue
		}
		e.From, e.To = remap(e.From), remap(e.To)
		out.Edges = append(out.Edges, e)
	}
	return out
}

// addNoise appends a detached Op node (an inserted statement with no
// data flow to the rest, like a logging call).
func addNoise(pdg *MiningGraph, n int) *MiningGraph {
	out := &MiningGraph{
		Nodes: append([]PdgNode{}, pdg.Nodes...),
		Edges: append([]PdgEdge{}, pdg.Edges...),
	}
	for i := 0; i < n; i++ {
		out.Nodes = append(out.Nodes, PdgNode{Kind: Op, Detail: "arith:+", Line: 100 + i})
	}
	return out
}

// TestGappedCloneSimilarities evaluates the 600 threshold on synthetic gapped
// clones: base vs base-minus-N-statements and base-plus-N-noise-statements.
func TestGappedCloneSimilarities(t *testing.T) {
	base := gapBase()
	wb := NewWl(base)
	cases := []struct {
		name string
		pdg  *MiningGraph
	}{
		{"identical", gapBase()},
		{"minus1", dropNode(base, 7)},              // drop the arith op
		{"minus2", dropNode(dropNode(base, 7), 3)}, // drop op and errorf call
		{"plus1", addNoise(base, 1)},
		{"plus2", addNoise(base, 2)},
	}
	for _, c := range cases {
		sim := SimilarityMilli(wb, NewWl(c.pdg))
		wtd := SimilarityWeighted(wb, NewWl(c.pdg))
		t.Logf("%-10s flat=%d weighted=%d", c.name, sim, wtd)
	}
}

// TestGappedClonesAboveThreshold asserts the kernel still clears 600 for
// small gaps (1 insertion/deletion), the CCGraph approximate-matching claim.
// The control-weighted kernel additionally survives 2-statement deletions
// that drop the flat kernel below threshold.
func TestGappedClonesAboveThreshold(t *testing.T) {
	base := gapBase()
	wb := NewWl(base)
	for _, c := range []struct {
		name string
		pdg  *MiningGraph
	}{
		{"minus1", dropNode(base, 7)},
		{"plus1", addNoise(base, 1)},
	} {
		if sim := SimilarityMilli(wb, NewWl(c.pdg)); sim < 600 {
			t.Errorf("%s: similarity %d, want >= 600 (gapped clone should be found)", c.name, sim)
		}
	}
	// Two deleted statements: flat kernel falls to 460, weighted holds 605.
	twoDel := dropNode(dropNode(base, 7), 3)
	if sim := SimilarityMilli(wb, NewWl(twoDel)); sim >= 600 {
		t.Logf("note: flat kernel also clears 600 for 2 deletions (%d)", sim)
	}
	if sim := SimilarityWeighted(wb, NewWl(twoDel)); sim < 600 {
		t.Errorf("minus2 weighted: %d, want >= 600", sim)
	}
}
