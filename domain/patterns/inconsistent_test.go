package patterns

import "testing"

func TestFindInconsistentClonesUniform(t *testing.T) {
	// Three identical PDGs: similarity 1000, not divergent.
	pdgs := map[string]*MiningGraph{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(20, 100),
		"f3": ccTestPdg(20, 200),
	}
	groups := [][]string{{"f1", "f2", "f3"}}
	got := FindInconsistentClones(groups, pdgs)
	if len(got) != 0 {
		t.Fatalf("uniform group should not be flagged, got %v", got)
	}
}

func TestFindInconsistentClonesDivergent(t *testing.T) {
	// f1 and f2 are identical (20 nodes); f3 has 22 nodes (similarity 909,
	// in the [900, 950) divergent band). f3 simulates a clone that gained
	// extra logic — possibly a fix the others missed.
	pdgs := map[string]*MiningGraph{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(20, 100),
		"f3": ccTestPdg(22, 200),
	}
	groups := [][]string{{"f1", "f2", "f3"}}
	got := FindInconsistentClones(groups, pdgs)
	if len(got) != 1 {
		t.Fatalf("divergent group should be flagged, got %v", got)
	}
}

func TestFindInconsistentClonesEmpty(t *testing.T) {
	if got := FindInconsistentClones(nil, nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	if got := FindInconsistentClones([][]string{{"f1"}}, map[string]*MiningGraph{"f1": ccTestPdg(20, 1)}); len(got) != 0 {
		t.Fatalf("single-member group should not be flagged, got %v", got)
	}
}

func TestInconsistentCloneCandidates(t *testing.T) {
	facts := []*MiningFacts{
		{ID: "f1", Name: "processData", Path: "a.go", Line: 10},
		{ID: "f2", Name: "processDatum", Path: "b.go", Line: 20},
		{ID: "f3", Name: "processDatas", Path: "c.go", Line: 30},
	}
	groups := [][]string{{"f1", "f2", "f3"}}
	cands := InconsistentCloneCandidates(groups, facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != InconsistentClone {
		t.Fatalf("expected kind %q, got %q", InconsistentClone, c.Kind)
	}
	if len(c.Sites) != 3 {
		t.Fatalf("expected 3 sites, got %d", len(c.Sites))
	}
	if c.FixSpec != nil {
		t.Fatalf("inconsistent_clone should be detection-only (nil FixSpec)")
	}
}
