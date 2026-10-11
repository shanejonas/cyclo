package gopatterns

import (
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// TestReextractedSimilarity validates the extractor end-to-end: PDGs
// re-extracted from Go source (not hand-written) must still clear rstyle's
// 600‰ cluster threshold for structurally parallel pairs, and stay far below
// it for unrelated code. This is the Phase 1 acceptance check from the spike.
func TestReextractedSimilarity(t *testing.T) {
	pdgs := loadShapes(t)
	sim := func(a, b string) uint32 {
		pa := findPdg(t, pdgs, a).Pdg
		pb := findPdg(t, pdgs, b).Pdg
		return patterns.SimilarityMilli(patterns.NewWl(&pa), patterns.NewWl(&pb))
	}
	cases := []struct {
		a, b  string
		want  uint32 // minimum similarity for parallel pairs
		max   uint32 // maximum similarity for unrelated pairs (0 = n/a)
		label string
	}{
		{"CreateUser", "CreateOrder", 600, 0, "validate-then-act pair"},
		{"FetchWithRetry", "LoadWithRetry", 600, 0, "retry pair"},
		{"SumFor", "SumRange", 600, 0, "for/range pair"},
		{"CreateUser", "NormalizeScores", 0, 600, "unrelated pair"},
		{"FetchWithRetry", "CreateUser", 0, 600, "unrelated pair"},
	}
	for _, c := range cases {
		got := sim(c.a, c.b)
		t.Logf("%s vs %s: %d‰", c.a, c.b, got)
		if c.want > 0 && got < c.want {
			t.Errorf("%s: similarity %d‰, want >= %d‰", c.label, got, c.want)
		}
		if c.max > 0 && got >= c.max {
			t.Errorf("%s: similarity %d‰, want < %d‰", c.label, got, c.max)
		}
	}
}

// TestSelfSimilarity is a sanity check: every extracted PDG is identical to
// itself.
func TestSelfSimilarity(t *testing.T) {
	for _, f := range loadShapes(t) {
		pdg := f.Pdg
		if got := patterns.SimilarityMilli(patterns.NewWl(patterns.MiningView(&pdg)), patterns.NewWl(patterns.MiningView(&pdg))); got != 1000 {
			t.Errorf("%s self-similarity = %d‰, want 1000‰", f.Name, got)
		}
	}
}
