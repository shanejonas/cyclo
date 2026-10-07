package patterns

import "testing"

func TestSpecificationCandidatesGroupsByRule(t *testing.T) {
	facts := []*FuncFacts{
		{
			ID: "pkg.FuncA",
			SpecRules: []SpecificationHit{
				{Line: 10, RuleKey: "User|Age:>,Active:truthy", CondText: "user.Age > 18 && user.Active", VarName: "user", TypeName: "User"},
			},
		},
		{
			ID: "pkg.FuncB",
			SpecRules: []SpecificationHit{
				{Line: 20, RuleKey: "User|Age:>,Active:truthy", CondText: "u.Age > 18 && u.Active", VarName: "u", TypeName: "User"},
			},
		},
		{
			ID: "pkg.FuncC",
			SpecRules: []SpecificationHit{
				{Line: 30, RuleKey: "Order|Total:>,Paid:truthy", CondText: "o.Total > 100 && o.Paid", VarName: "o", TypeName: "Order"},
			},
		},
	}
	cands := specificationCandidates(facts)
	// Only the User rule appears in 2+ functions.
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != Specification {
		t.Errorf("expected kind specification, got %s", c.Kind)
	}
	if c.FixSpec == nil {
		t.Fatal("expected FixSpec, got nil")
	}
	if c.FixSpec.Params["type"] != "User" {
		t.Errorf("expected type User, got %s", c.FixSpec.Params["type"])
	}
}

func TestSpecificationCandidatesSkipsSingle(t *testing.T) {
	facts := []*FuncFacts{
		{
			ID: "pkg.FuncA",
			SpecRules: []SpecificationHit{
				{Line: 10, RuleKey: "User|Age:>", CondText: "user.Age > 18", VarName: "user", TypeName: "User"},
			},
		},
	}
	cands := specificationCandidates(facts)
	if len(cands) != 0 {
		t.Errorf("expected 0 candidates for single-function rule, got %d", len(cands))
	}
}
