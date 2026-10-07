package patterns

import "testing"

func TestTypeSwitchCandidates(t *testing.T) {
	facts := []*FuncFacts{{
		ID:      "main.speakAll",
		Name:    "speakAll",
		Path:    "main.go",
		Line:    10,
		EndLine: 20,
		TypeSwitches: []TypeSwitchHit{{
			Line:   12,
			Bound:  "v",
			Expr:   "x",
			Method: "Speak",
			Types:  []string{"Dog", "Cat"},
			Args:   "",
		}},
	}}
	cands := typeSwitchCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != TypeSwitch {
		t.Errorf("wrong kind: %s", c.Kind)
	}
	if c.FixSpec == nil {
		t.Fatal("missing FixSpec")
	}
	if c.FixSpec.Line != 12 {
		t.Errorf("wrong FixSpec line: %d", c.FixSpec.Line)
	}
	if c.FixSpec.Params["method"] != "Speak" {
		t.Errorf("wrong method param: %s", c.FixSpec.Params["method"])
	}
}

func TestTypeSwitchCandidatesEmpty(t *testing.T) {
	facts := []*FuncFacts{{ID: "main.f", Name: "f"}}
	if cands := typeSwitchCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cands))
	}
}
