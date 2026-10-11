package patterns

import "testing"

func TestEntityIdentityCandidates(t *testing.T) {
	facts := []*MiningFacts{{
		ID:   "test.compare",
		Name: "compare",
		Path: "test.go",
		Line: 10,
		EntityIdentities: []EntityIdentityHit{{
			Line:     12,
			TypeName: "User",
			IDField:  "ID",
			Fields:   []string{"Name", "Email"},
			Left:     "a",
			Right:    "b",
		}},
	}}
	cands := entityIdentityCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want 1", len(cands))
	}
	c := cands[0]
	if c.Kind != EntityIdentity {
		t.Errorf("kind = %q, want entity_identity", c.Kind)
	}
	if c.FixSpec == nil {
		t.Fatal("FixSpec should not be nil")
	}
	if c.FixSpec.Params["id_field"] != "ID" {
		t.Errorf("id_field = %q, want ID", c.FixSpec.Params["id_field"])
	}
}

func TestMissingIdentityCandidates(t *testing.T) {
	hits := []MissingIdentityHit{{
		TypeName: "Order",
		Path:     "order.go",
		Line:     5,
		EndLine:  10,
		UseCount: 3,
	}}
	cands := missingIdentityCandidates(hits)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want 1", len(cands))
	}
	c := cands[0]
	if c.Kind != MissingIdentity {
		t.Errorf("kind = %q, want missing_identity", c.Kind)
	}
	// Detection-only first gate: the entity_identity fixer does the work.
	if c.FixSpec != nil {
		t.Error("FixSpec should be nil (detection-only first gate)")
	}
}

func TestMutableIdentityCandidatesDetectionOnly(t *testing.T) {
	facts := []*MiningFacts{{
		ID:   "test.update",
		Name: "update",
		Path: "test.go",
		Line: 10,
		MutableIdentities: []MutableIdentityHit{{
			Line:     12,
			Field:    "ID",
			FuncName: "update",
		}},
	}}
	cands := mutableIdentityCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want 1", len(cands))
	}
	c := cands[0]
	if c.Kind != MutableIdentity {
		t.Errorf("kind = %q, want mutable_identity", c.Kind)
	}
	if c.FixSpec != nil {
		t.Error("FixSpec should be nil for detection-only kind")
	}
}
