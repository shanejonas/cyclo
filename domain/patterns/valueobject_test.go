package patterns

import (
	"strings"
	"testing"
)

// voFact builds a MiningFacts with the given primitive params.
func voFact(id, path string, line int, params ...ParamInfo) *MiningFacts {
	return &MiningFacts{
		ID:     id,
		Name:   id,
		Path:   path,
		Line:   line,
		Params: params,
	}
}

func TestValueObjectClumpFound(t *testing.T) {
	facts := []*MiningFacts{
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
	facts := []*MiningFacts{
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
	facts := []*MiningFacts{
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
	facts := []*MiningFacts{
		voFact("a.F", "a.go", 1),
		voFact("b.G", "b.go", 10),
		voFact("c.H", "c.go", 20),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps with no params, got %d", len(cands))
	}
}

func TestValueObjectMaximalOnly(t *testing.T) {
	facts := []*MiningFacts{
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

func TestValueObjectSkipsGenericNames(t *testing.T) {
	// (x, y) are placeholders, not a domain concept.
	facts := []*MiningFacts{
		voFact("a.F", "a.go", 1, ParamInfo{"x", "string"}, ParamInfo{"y", "string"}),
		voFact("b.G", "b.go", 10, ParamInfo{"x", "string"}, ParamInfo{"y", "string"}),
		voFact("c.H", "c.go", 20, ParamInfo{"x", "string"}, ParamInfo{"y", "string"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps for generic (x, y), got %d", len(cands))
	}
}

func TestValueObjectSkipsSingleLetters(t *testing.T) {
	facts := []*MiningFacts{
		voFact("a.F", "a.go", 1, ParamInfo{"a", "int"}, ParamInfo{"b", "int"}),
		voFact("b.G", "b.go", 10, ParamInfo{"a", "int"}, ParamInfo{"b", "int"}),
		voFact("c.H", "c.go", 20, ParamInfo{"a", "int"}, ParamInfo{"b", "int"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps for (a, b), got %d", len(cands))
	}
}

func TestValueObjectSkipsPlaceholders(t *testing.T) {
	facts := []*MiningFacts{
		voFact("a.F", "a.go", 1, ParamInfo{"foo", "string"}, ParamInfo{"bar", "string"}),
		voFact("b.G", "b.go", 10, ParamInfo{"foo", "string"}, ParamInfo{"bar", "string"}),
		voFact("c.H", "c.go", 20, ParamInfo{"foo", "string"}, ParamInfo{"bar", "string"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 0 {
		t.Errorf("expected 0 clumps for (foo, bar), got %d", len(cands))
	}
}

func TestValueObjectKeepsMeaningfulNames(t *testing.T) {
	facts := []*MiningFacts{
		voFact("a.F", "a.go", 1, ParamInfo{"start", "int"}, ParamInfo{"end", "int"}),
		voFact("b.G", "b.go", 10, ParamInfo{"start", "int"}, ParamInfo{"end", "int"}),
		voFact("c.H", "c.go", 20, ParamInfo{"start", "int"}, ParamInfo{"end", "int"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 1 {
		t.Errorf("expected 1 clump for (start, end), got %d", len(cands))
	}
}

func TestValueObjectKeepsMixedGroup(t *testing.T) {
	// One meaningful name saves the clump.
	facts := []*MiningFacts{
		voFact("a.F", "a.go", 1, ParamInfo{"x", "int"}, ParamInfo{"offset", "int"}),
		voFact("b.G", "b.go", 10, ParamInfo{"x", "int"}, ParamInfo{"offset", "int"}),
		voFact("c.H", "c.go", 20, ParamInfo{"x", "int"}, ParamInfo{"offset", "int"}),
	}
	if cands := valueObjectCandidates(facts); len(cands) != 1 {
		t.Errorf("expected 1 clump for mixed (x, offset), got %d", len(cands))
	}
}

func TestIsGenericParamName(t *testing.T) {
	generic := []string{"x", "y", "a", "b", "i", "k", "v", "foo", "bar", "tmp", "temp", "val", "arg", "p1", "p2", "X", "Y"}
	for _, n := range generic {
		if !isGenericParamName(n) {
			t.Errorf("expected %q to be generic", n)
		}
	}
	meaningful := []string{"start", "end", "path", "root", "amount", "currency", "name", "email"}
	for _, n := range meaningful {
		if isGenericParamName(n) {
			t.Errorf("expected %q to be meaningful", n)
		}
	}
}
