package patterns

import (
	"strings"
	"testing"
)

func TestGuardCandidatesProposeInversion(t *testing.T) {
	facts := []*FuncFacts{{
		ID:           "p.complete",
		Name:         "complete",
		Path:         "completions.go",
		Line:         660,
		EndLine:      700,
		GuardClauses: []int{673},
	}}
	got := guardCandidates(facts)
	if len(got) != 1 {
		t.Fatalf("expected 1 guard candidate, got %d", len(got))
	}
	c := got[0]
	if c.Kind != GuardClause {
		t.Fatalf("expected kind guard_clause, got %q", c.Kind)
	}
	if len(c.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(c.Sites))
	}
	s := c.Sites[0]
	if s.Line != 673 || s.EndLine != 700 || s.Path != "completions.go" {
		t.Fatalf("site should pinpoint the if statement: %+v", s)
	}
	if c.Observation == "" || c.Inference == "" || c.PossibleRefactor == "" {
		t.Fatal("candidate must carry observation, inference, and refactor text")
	}
	if !strings.Contains(c.PossibleRefactor, "guard") && !strings.Contains(c.PossibleRefactor, "early") {
		t.Fatalf("refactor text should mention the guard clause: %q", c.PossibleRefactor)
	}
}

func TestGuardCandidatesNoneWithoutHits(t *testing.T) {
	facts := []*FuncFacts{{
		ID:   "p.clean",
		Name: "clean",
		Path: "clean.go",
		Line: 1,
	}}
	if got := guardCandidates(facts); len(got) != 0 {
		t.Fatalf("expected no guard candidates, got %d", len(got))
	}
}

func TestRunIncludesGuardClauses(t *testing.T) {
	facts := []*FuncFacts{{
		ID:           "p.complete",
		Name:         "complete",
		Path:         "completions.go",
		Line:         660,
		EndLine:      700,
		GuardClauses: []int{673},
	}}
	report := Run(facts, Options{})
	found := false
	for _, c := range report.Candidates {
		if c.Kind == GuardClause {
			found = true
		}
	}
	if !found {
		t.Fatal("Run must include guard-clause candidates in the report")
	}
	text := Text(&report)
	if !strings.Contains(text, "guard_clause") {
		t.Fatalf("text report must name the guard_clause kind:\n%s", text)
	}
}
