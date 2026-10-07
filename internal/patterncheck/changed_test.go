package patterncheck

import (
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/internal/gitchanged"
)

func TestCandidateTouchesDiff(t *testing.T) {
	ranges := map[string][]gitchanged.LineRange{
		"a.go": {{10, 15}},
	}
	candidate := func(sites ...patterns.Site) patterns.Candidate {
		return patterns.Candidate{Sites: sites}
	}
	site := func(path string, line, endLine int) patterns.Site {
		return patterns.Site{Path: path, Line: line, EndLine: endLine}
	}

	// Body overlap: site spans lines 8-20, diff touches 10-15.
	if !candidateTouchesDiff(candidate(site("a.go", 8, 20)), ranges, "/root", "/root") {
		t.Fatal("body overlap not detected")
	}
	// Declaration-line-only touch: site starts at 12.
	if !candidateTouchesDiff(candidate(site("a.go", 12, 30)), ranges, "/root", "/root") {
		t.Fatal("declaration-line touch not detected")
	}
	// No overlap: site ends before the diff range.
	if candidateTouchesDiff(candidate(site("a.go", 1, 9)), ranges, "/root", "/root") {
		t.Fatal("non-overlapping site reported as touched")
	}
	// Different file.
	if candidateTouchesDiff(candidate(site("b.go", 8, 20)), ranges, "/root", "/root") {
		t.Fatal("other file reported as touched")
	}
	// One touched site among several is enough.
	mixed := candidate(site("b.go", 1, 5), site("a.go", 100, 200), site("a.go", 8, 20))
	if !candidateTouchesDiff(mixed, ranges, "/root", "/root") {
		t.Fatal("mixed sites: touched site not detected")
	}
	// No sites at all.
	if candidateTouchesDiff(candidate(), ranges, "/root", "/root") {
		t.Fatal("empty candidate reported as touched")
	}
}
