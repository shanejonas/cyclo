package patterns

import (
	"strings"
	"testing"
)

func TestAnemicModelCandidatesProposeMethods(t *testing.T) {
	hits := []AnemicModelHit{{
		TypeName: "Order",
		Path:     "order.go",
		Line:     10,
		EndLine:  15,
		Funcs:    []string{"CalculateTotal", "ApplyDiscount", "ValidateOrder"},
	}}
	cands := anemicModelCandidates(hits)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != AnemicModel {
		t.Fatalf("expected anemic_model kind, got %q", c.Kind)
	}
	if c.ScoreMilli != 400 {
		t.Fatalf("expected score 400, got %d", c.ScoreMilli)
	}
	if !strings.Contains(c.Observation, "Order") || !strings.Contains(c.Observation, "3 functions") {
		t.Fatalf("observation should name type and count: %q", c.Observation)
	}
	if !strings.Contains(c.PossibleRefactor, "CalculateTotal") {
		t.Fatalf("refactor should name the functions: %q", c.PossibleRefactor)
	}
	if len(c.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(c.Sites))
	}
	s := c.Sites[0]
	if s.Line != 10 || s.EndLine != 15 || s.Path != "order.go" {
		t.Fatalf("site should cover the struct definition: %+v", s)
	}
}

func TestAnemicModelSkipsTooFewFuncs(t *testing.T) {
	hits := []AnemicModelHit{{
		TypeName: "Order",
		Path:     "order.go",
		Line:     10,
		EndLine:  15,
		Funcs:    []string{"CalculateTotal", "ApplyDiscount"},
	}}
	if cands := anemicModelCandidates(hits); len(cands) != 0 {
		t.Fatalf("expected no candidate for 2 funcs, got %d", len(cands))
	}
}

func TestAnemicModelEmptyHits(t *testing.T) {
	if cands := anemicModelCandidates(nil); len(cands) != 0 {
		t.Fatalf("expected no candidates for nil hits, got %d", len(cands))
	}
}

func TestAnemicModelWiredIntoRun(t *testing.T) {
	hits := []AnemicModelHit{{
		TypeName: "Cart",
		Path:     "cart.go",
		Line:     5,
		EndLine:  8,
		Funcs:    []string{"AddItem", "RemoveItem", "Checkout"},
	}}
	report := Run(nil, Options{AnemicModels: hits})
	found := false
	for _, c := range report.Candidates {
		if c.Kind == AnemicModel {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Run should include anemic_model candidates from Options")
	}
}
