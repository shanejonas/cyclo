package patterns

import "testing"

func TestCharacteristicVector(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			{Kind: Let}, {Kind: Let},
			{Kind: Call},
			{Kind: Branch},
			{Kind: Op},
		},
		Edges: []PdgEdge{
			{Kind: Ctrl}, {Kind: Data}, {Kind: Data},
		},
	}
	vec := characteristicVector(pdg)
	// [decl=2, assign=1, control=1, call=1, other=0, ctrlEdges=1, dataEdges=2]
	expected := []float64{2, 1, 1, 1, 0, 1, 2}
	if len(vec) != 7 {
		t.Fatalf("dimensions=%d", len(vec))
	}
	for i, v := range expected {
		if vec[i] != v {
			t.Errorf("vec[%d] = %v, want %v", i, vec[i], v)
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{1, 2, 3}
	if s := cosineSimilarity(a, b); s < 0.999 || s > 1.001 {
		t.Errorf("identical vectors similarity = %v, want ~1.0", s)
	}
	c := []float64{1, 0, 0}
	d := []float64{0, 1, 0}
	if s := cosineSimilarity(c, d); s != 0 {
		t.Errorf("orthogonal vectors similarity = %v, want 0", s)
	}
}

func TestCharVecSimilar(t *testing.T) {
	pdg1 := &MiningGraph{
		Nodes: []PdgNode{{Kind: Call}, {Kind: Let}, {Kind: Op}},
		Edges: []PdgEdge{{Kind: Data}},
	}
	pdg2 := &MiningGraph{
		Nodes: []PdgNode{{Kind: Call}, {Kind: Let}, {Kind: Op}},
		Edges: []PdgEdge{{Kind: Data}},
	}
	w1 := NewWl(pdg1)
	w2 := NewWl(pdg2)
	if !charVecSimilar(w1, w2) {
		t.Error("identical PDGs should be characteristically similar")
	}
}

func TestDiameterBasedRounds(t *testing.T) {
	// Linear chain: diameter = n-1, should limit rounds
	pdg := &MiningGraph{
		Nodes: []PdgNode{{Kind: Op}, {Kind: Op}},
		Edges: []PdgEdge{{From: 0, To: 1, Kind: Data}},
	}
	w := NewWl(pdg)
	// Diameter of 2-node chain is 1, so rounds = min(1+1, 4) = 2
	if len(w.rounds) != 2 {
		t.Errorf("2-node chain: got %d rounds, want 2 (diameter-based)", len(w.rounds))
	}
}
