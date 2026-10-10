package patterns

import "testing"

// TestNeighborhoodWLSelfSimilarity: a graph is maximally similar to itself.
func TestNeighborhoodWLSelfSimilarity(t *testing.T) {
	base := gapBase()
	w := NewNeighborhoodWL(base)
	if sim := SimilarityNeighborhoodWLMilli(w, w); sim != 1000 {
		t.Errorf("self similarity = %d, want 1000", sim)
	}
}

// TestNeighborhoodWLDeterministic: same PDG built twice gives identical similarity.
func TestNeighborhoodWLDeterministic(t *testing.T) {
	base := gapBase()
	a, b := NewNeighborhoodWL(base), NewNeighborhoodWL(gapBase())
	if SimilarityNeighborhoodWLMilli(a, b) != 1000 {
		t.Errorf("identical graphs: got %d, want 1000", SimilarityNeighborhoodWLMilli(a, b))
	}
	// And again, to catch map-iteration nondeterminism.
	c := NewNeighborhoodWL(gapBase())
	if SimilarityNeighborhoodWLMilli(a, c) != SimilarityNeighborhoodWLMilli(a, b) {
		t.Errorf("nondeterministic neighborhood-WL similarity")
	}
}

// TestNeighborhoodWLGappedClones logs neighborhood-WL vs flat-WL similarities on synthetic gapped
// clones, for the benchmark comparison.
func TestNeighborhoodWLGappedClones(t *testing.T) {
	base := gapBase()
	wb := NewNeighborhoodWL(base)
	fb := NewWl(base)
	cases := []struct {
		name string
		pdg  *Pdg
	}{
		{"identical", gapBase()},
		{"minus1", dropNode(base, 7)},
		{"minus2", dropNode(dropNode(base, 7), 3)},
		{"plus1", addNoise(base, 1)},
		{"plus2", addNoise(base, 2)},
	}
	for _, c := range cases {
		neighborhoodWL := SimilarityNeighborhoodWLMilli(wb, NewNeighborhoodWL(c.pdg))
		flat := SimilarityMilli(fb, NewWl(c.pdg))
		t.Logf("%-10s neighborhoodWL=%d flat=%d", c.name, neighborhoodWL, flat)
	}
}

// TestNeighborhoodWLUnrelatedLow: structurally unrelated graphs score well below the
// 900 match threshold.
func TestNeighborhoodWLUnrelatedLow(t *testing.T) {
	a := NewNeighborhoodWL(gapBase())
	b := NewNeighborhoodWL(&Pdg{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "int", Line: 1},
			{Kind: Lit, LitKind: "int", Line: 2},
			{Kind: Op, Detail: "cmp:<", Line: 3},
			{Kind: Return, Line: 4},
		},
		Edges: []PdgEdge{
			{From: 0, To: 2, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 1},
			{From: 2, To: 3, Kind: Data, ArgPos: 0},
		},
	})
	if sim := SimilarityNeighborhoodWLMilli(a, b); sim >= 900 {
		t.Errorf("unrelated graphs: similarity %d, want < 900", sim)
	}
}

// TestNeighborhoodWLPipelineRuns exercises the neighborhood-WL entry point end to end on a tiny
// corpus: two identical functions must group together.
func TestNeighborhoodWLPipelineRuns(t *testing.T) {
	pdgs := map[string]*Pdg{
		"a": gapBase(),
		"b": gapBase(),
	}
	names := map[string]string{"a": "handle", "b": "handle"}
	groups := CCGraphClonesWithNeighborhoodWL(pdgs, names)
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Errorf("neighborhood-WL pipeline: got %v, want one group of two", groups)
	}
}
