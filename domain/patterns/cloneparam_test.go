package patterns

import "testing"

// cloneParamTestFacts builds two FuncFacts with similar PDGs that differ in
// one literal (a hole for parameterization). Each PDG has 2+ calls so the
// functions pass hasSubstance.
func cloneParamTestFacts() []*FuncFacts {
	// Two 5-node PDGs differing only in one literal node's Detail.
	mkPdg := func(detail string) *Pdg {
		nodes := []PdgNode{
			{Kind: Call, Line: 1, Detail: "call:fetch"},
			{Kind: Op, Line: 2, Detail: "add:int"},
			{Kind: Op, Line: 3, Detail: detail},
			{Kind: Call, Line: 4, Detail: "call:store"},
			{Kind: Op, Line: 5, Detail: "ret:int"},
		}
		edges := []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
			{From: 2, To: 3, Kind: Data},
			{From: 3, To: 4, Kind: Data},
		}
		return &Pdg{Nodes: nodes, Edges: edges}
	}
	return []*FuncFacts{
		{ID: "pkg.Foo", Name: "Foo", Path: "a.go", Line: 10, EndLine: 20, Pdg: mkPdg("lit:1")},
		{ID: "pkg.Bar", Name: "Bar", Path: "b.go", Line: 30, EndLine: 40, Pdg: mkPdg("lit:2")},
	}
}

func TestParameterizeFromCloneGroupTooSmall(t *testing.T) {
	facts := cloneParamTestFacts()
	params := DefaultParams()
	if c := ParameterizeFromCloneGroup(facts, nil, params); c != nil {
		t.Error("empty group should return nil")
	}
	if c := ParameterizeFromCloneGroup(facts, []string{"pkg.Foo"}, params); c != nil {
		t.Error("single-member group should return nil")
	}
}

func TestParameterizeFromCloneGroupMissingIDs(t *testing.T) {
	facts := cloneParamTestFacts()
	params := DefaultParams()
	// Unknown IDs resolve to nothing; fewer than 2 facts with PDGs -> nil.
	if c := ParameterizeFromCloneGroup(facts, []string{"pkg.Nope", "pkg.AlsoNope"}, params); c != nil {
		t.Error("unknown IDs should return nil")
	}
	if c := ParameterizeFromCloneGroup(facts, []string{"pkg.Foo", "pkg.Nope"}, params); c != nil {
		t.Error("only one resolvable member should return nil")
	}
}

func TestParameterizeFromCloneGroupNoPdgs(t *testing.T) {
	facts := []*FuncFacts{
		{ID: "pkg.A", Name: "A", Path: "a.go", Line: 1, EndLine: 5},
		{ID: "pkg.B", Name: "B", Path: "b.go", Line: 1, EndLine: 5},
	}
	params := DefaultParams()
	if c := ParameterizeFromCloneGroup(facts, []string{"pkg.A", "pkg.B"}, params); c != nil {
		t.Error("members without PDGs should return nil")
	}
}

func TestParameterizeFromCloneGroupProducesCandidate(t *testing.T) {
	facts := cloneParamTestFacts()
	params := DefaultParams()
	c := ParameterizeFromCloneGroup(facts, []string{"pkg.Foo", "pkg.Bar"}, params)
	if c == nil {
		t.Fatal("expected a parameterize candidate for the clone pair")
	}
	if c.Kind != Parameterize {
		t.Errorf("candidate kind = %v, want Parameterize", c.Kind)
	}
	if len(c.Sites) != 2 {
		t.Errorf("candidate has %d sites, want 2", len(c.Sites))
	}
	// Determinism: same input twice gives the same candidate.
	c2 := ParameterizeFromCloneGroup(facts, []string{"pkg.Foo", "pkg.Bar"}, params)
	if c2 == nil {
		t.Fatal("second run should also produce a candidate")
	}
	if c.Observation != c2.Observation || c.PossibleRefactor != c2.PossibleRefactor {
		t.Error("ParameterizeFromCloneGroup is not deterministic")
	}
}

func TestCloneGroupParameterizeCandidatesSkipsNil(t *testing.T) {
	facts := cloneParamTestFacts()
	params := DefaultParams()
	groups := [][]string{
		{"pkg.Foo", "pkg.Bar"}, // should yield a candidate
		{"pkg.Foo"},            // too small, skipped
		{"pkg.Nope"},           // unknown, skipped
	}
	cands := CloneGroupParameterizeCandidates(facts, groups, params)
	if len(cands) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(cands))
	}
	if len(cands) == 1 && cands[0].Kind != Parameterize {
		t.Errorf("candidate kind = %v, want Parameterize", cands[0].Kind)
	}
}
