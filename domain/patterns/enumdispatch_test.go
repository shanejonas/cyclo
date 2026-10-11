package patterns

import (
	"strings"
	"testing"
)

func TestEnumDispatchCandidatesProposeTable(t *testing.T) {
	facts := []*MiningFacts{{
		ID:             "p.process",
		Name:           "process",
		Path:           "process.go",
		Line:           10,
		EndLine:        30,
		EnumDispatches: []EnumDispatchHit{{Line: 15, NumCases: 3}},
	}}
	got := enumDispatchCandidates(facts)
	if len(got) != 1 {
		t.Fatalf("expected 1 enum_dispatch candidate, got %d", len(got))
	}
	c := got[0]
	if c.Kind != EnumDispatch {
		t.Fatalf("expected kind enum_dispatch, got %q", c.Kind)
	}
	if len(c.Sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(c.Sites))
	}
	s := c.Sites[0]
	if s.Line != 10 || s.EndLine != 30 || s.Path != "process.go" {
		t.Fatalf("site should cover the function range: %+v", s)
	}
	if !strings.Contains(c.Observation, "15") {
		t.Fatalf("observation should name the switch line: %q", c.Observation)
	}
	if c.FixSpec == nil {
		t.Fatal("candidate must carry a FixSpec")
	}
	if c.FixSpec.Line != 15 {
		t.Fatalf("FixSpec line should be the switch line, got %d", c.FixSpec.Line)
	}
	if c.FixSpec.Kind != EnumDispatch {
		t.Fatalf("FixSpec kind should be enum_dispatch, got %q", c.FixSpec.Kind)
	}
}

func TestEnumDispatchCandidatesNoneWithoutHits(t *testing.T) {
	facts := []*MiningFacts{{
		ID:   "p.clean",
		Name: "clean",
		Path: "clean.go",
		Line: 1,
	}}
	if got := enumDispatchCandidates(facts); len(got) != 0 {
		t.Fatalf("expected no enum_dispatch candidates, got %d", len(got))
	}
}

func TestRunIncludesEnumDispatch(t *testing.T) {
	facts := []*MiningFacts{{
		ID:             "p.process",
		Name:           "process",
		Path:           "process.go",
		Line:           10,
		EndLine:        30,
		EnumDispatches: []EnumDispatchHit{{Line: 15, NumCases: 2}},
	}}
	report := RunMining(facts, Options{})
	found := false
	for _, c := range report.Candidates {
		if c.Kind == EnumDispatch {
			found = true
		}
	}
	if !found {
		t.Fatal("Run must include enum_dispatch candidates in the report")
	}
}
