package patterns

import (
	"strings"
	"testing"
)

// voFact builds a FuncFacts with the given primitive params.
func voFact(id, path string, line int, params ...ParamInfo) *FuncFacts {
	return &FuncFacts{
		ID:     id,
		Name:   id,
		Path:   path,
		Line:   line,
		Params: params,
	}
}

func TestValueObjectClumpFound(t *testing.T) {
	facts := []*FuncFacts{
		voFact("a.Transfer", "a.go", 1,
			ParamInfo{"fromID", "string"}, ParamInfo{"toID", "string"},
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}),
		voFact("b.Refund", "b.go", 10,
			ParamInfo{"txID", "string"},
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}),
		voFact("c.Quote", "c.go", 20,
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"},
			ParamInfo{"region", "string"}),
	}
	cands := valueObjectCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 clump, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != ValueObject {
		t.Errorf("kind = %q, want value_object", c.Kind)
	}
	if c.ScoreMilli != valueObjectScoreMilli {
		t.Errorf("score = %d, want %d", c.ScoreMilli, valueObjectScoreMilli)
	}
	if len(c.Sites) != 3 {
		t.Errorf("sites = %d, want 3", len(c.Sites))
	}
	if !strings.Contains(c.Observation, "amount:int") || !strings.Contains(c.Observation, "currency:string") {
		t.Errorf("observation missing clump keys: %q", c.Observation)
	}
	if !strings.Contains(c.PossibleRefactor, "Amount int") || !strings.Contains(c.PossibleRefactor, "Currency string") {
		t.Errorf("refactor missing struct fields: %q", c.PossibleRefactor)
	}
}

func TestValueObjectNeedsThreeFuncs(t *testing.T) {
	facts := []*FuncFacts{
		voFact("a.F", "a.go", 1,
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}),
		voFact("b.G", "b.go", 10,
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps for 2 funcs, got %d", len(cands))
	}
}

func TestValueObjectNeedsTwoParams(t *testing.T) {
	facts := []*FuncFacts{
		voFact("a.F", "a.go", 1, ParamInfo{"amount", "int"}),
		voFact("b.G", "b.go", 10, ParamInfo{"amount", "int"}),
		voFact("c.H", "c.go", 20, ParamInfo{"amount", "int"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps for single param, got %d", len(cands))
	}
}

func TestValueObjectIgnoresNonPrimitive(t *testing.T) {
	// Params without primitive types are never recorded by the extractor,
	// so facts with empty Params are skipped.
	facts := []*FuncFacts{
		voFact("a.F", "a.go", 1),
		voFact("b.G", "b.go", 10),
		voFact("c.H", "c.go", 20),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps with no params, got %d", len(cands))
	}
}

func TestValueObjectMaximalOnly(t *testing.T) {
	facts := []*FuncFacts{
		voFact("a.F", "a.go", 1,
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}, ParamInfo{"region", "string"}),
		voFact("b.G", "b.go", 10,
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}, ParamInfo{"region", "string"}),
		voFact("c.H", "c.go", 20,
			ParamInfo{"amount", "int"}, ParamInfo{"currency", "string"}, ParamInfo{"region", "string"}),
	}
	cands := valueObjectCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 maximal clump, got %d", len(cands))
	}
	if !strings.Contains(cands[0].Observation, "region:string") {
		t.Errorf("expected the 3-param clump, got: %q", cands[0].Observation)
	}
}

func TestValueObjectEmpty(t *testing.T) {
	if cands := valueObjectCandidates(nil); len(cands) != 0 {
		t.Errorf("expected 0 clumps for nil, got %d", len(cands))
	}
}
