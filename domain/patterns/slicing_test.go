package patterns

import "testing"

// chainPdg builds a linear PDG: Param -> Op x (n-2) -> Return, all data edges.
func chainPdg(n int) *MiningGraph {
	nodes := make([]PdgNode, n)
	edges := make([]PdgEdge, 0, n-1)
	nodes[0] = PdgNode{Kind: Param, Line: 1}
	for i := 1; i < n-1; i++ {
		nodes[i] = PdgNode{Kind: Op, Line: i + 1}
		edges = append(edges, PdgEdge{From: i - 1, To: i, Kind: Data})
	}
	nodes[n-1] = PdgNode{Kind: Return, Line: n}
	edges = append(edges, PdgEdge{From: n - 2, To: n - 1, Kind: Data})
	return &MiningGraph{Nodes: nodes, Edges: edges}
}

func sliceTestFact(id string, pdg *MiningGraph) *MiningFacts {
	return &MiningFacts{
		ID:   id,
		Name: "testFunc",
		Path: "test.go",
		Line: 1,
		Pdg:  pdg,
	}
}

func TestFindBarrierSlicesFlagsLarge(t *testing.T) {
	// 25-node chain: barrier slice from return is 25 nodes >= 20.
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(25))}
	findings := FindBarrierSlices(facts)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].SliceSize != 25 {
		t.Errorf("expected slice size 25, got %d", findings[0].SliceSize)
	}
}

func TestFindBarrierSlicesSkipsSmall(t *testing.T) {
	// 5-node chain: below the 20-node threshold.
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(5))}
	if findings := FindBarrierSlices(facts); len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestFindBarrierSlicesSkipsNilPdg(t *testing.T) {
	facts := []*MiningFacts{{ID: "pkg.f", Name: "f", Path: "f.go"}}
	if findings := FindBarrierSlices(facts); len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestFindThinSlicesFlagsLongChain(t *testing.T) {
	// 12-node chain: thin slice from return is 12 >= 10.
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(12))}
	findings := FindThinSlices(facts)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestFindThinSlicesSkipsShort(t *testing.T) {
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(5))}
	if findings := FindThinSlices(facts); len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestFindChopsFlagsLarge(t *testing.T) {
	// 25-node chain: chop from param to return covers all 25 >= 20.
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(25))}
	findings := FindChops(facts)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].ChopSize != 25 {
		t.Errorf("expected chop size 25, got %d", findings[0].ChopSize)
	}
}

func TestFindChopsSkipsSmall(t *testing.T) {
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(5))}
	if findings := FindChops(facts); len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestBarrierSliceCandidatesShape(t *testing.T) {
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(25))}
	findings := FindBarrierSlices(facts)
	cands := BarrierSliceCandidates(findings, facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != BarrierSliceKind {
		t.Errorf("expected kind barrier_slice, got %s", c.Kind)
	}
	if c.FixSpec != nil {
		t.Error("detection-only: FixSpec must be nil")
	}
	if len(c.Sites) != 1 {
		t.Errorf("expected 1 site, got %d", len(c.Sites))
	}
}

func TestThinSliceCandidatesShape(t *testing.T) {
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(12))}
	findings := FindThinSlices(facts)
	cands := ThinSliceCandidates(findings, facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if cands[0].Kind != ThinSliceKind {
		t.Errorf("expected kind thin_slice, got %s", cands[0].Kind)
	}
	if cands[0].FixSpec != nil {
		t.Error("detection-only: FixSpec must be nil")
	}
}

func TestChopCandidatesShape(t *testing.T) {
	facts := []*MiningFacts{sliceTestFact("pkg.f", chainPdg(25))}
	findings := FindChops(facts)
	cands := ChopCandidates(findings, facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	if cands[0].Kind != ChopKind {
		t.Errorf("expected kind chop, got %s", cands[0].Kind)
	}
	if cands[0].FixSpec != nil {
		t.Error("detection-only: FixSpec must be nil")
	}
}

func TestSliceScoreCaps(t *testing.T) {
	if s := sliceScore(5); s != 450 {
		t.Errorf("expected 450, got %d", s)
	}
	if s := sliceScore(100); s != 800 {
		t.Errorf("expected cap 800, got %d", s)
	}
}
