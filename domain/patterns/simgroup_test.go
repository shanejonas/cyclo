package patterns

import "testing"

func TestMakeSimilarityGroups(t *testing.T) {
	raw := [][]string{
		{"f3", "f1", "f2"}, // unsorted input
		{"g1", "g2"},
	}
	groups := makeSimilarityGroups(raw)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	g := groups[0]
	if g.GroupID != "simgroup-0" {
		t.Fatalf("expected GroupID simgroup-0, got %q", g.GroupID)
	}
	// Members sorted for determinism.
	want := []string{"f1", "f2", "f3"}
	for i, id := range want {
		if g.MemberIDs[i] != id {
			t.Fatalf("expected member %d = %q, got %q", i, id, g.MemberIDs[i])
		}
	}
	if g.RepresentativeID != "f1" {
		t.Fatalf("expected representative f1, got %q", g.RepresentativeID)
	}
	if g.Size != 3 {
		t.Fatalf("expected size 3, got %d", g.Size)
	}
}

func TestMakeSimilarityGroupsEmpty(t *testing.T) {
	if got := makeSimilarityGroups(nil); len(got) != 0 {
		t.Fatalf("expected no groups for nil input, got %v", got)
	}
}

func TestFindSimilarityGroupsSkipsNoPdg(t *testing.T) {
	// Facts without PDGs produce no groups.
	facts := []*FuncFacts{
		{ID: "f1", Name: "foo"},
		{ID: "f2", Name: "bar"},
	}
	if got := FindSimilarityGroups(facts, ccMatchThreshold); len(got) != 0 {
		t.Fatalf("expected no groups without PDGs, got %v", got)
	}
}

func TestFindSimilarityGroupsSingleFact(t *testing.T) {
	// A single function can't form a group.
	facts := []*FuncFacts{
		{ID: "f1", Name: "foo", Pdg: ccTestPdg(20, 1)},
	}
	if got := FindSimilarityGroups(facts, ccMatchThreshold); len(got) != 0 {
		t.Fatalf("expected no groups for a single function, got %v", got)
	}
}
