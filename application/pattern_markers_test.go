package application

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func testPatternsReport() *patterns.PatternsReport {
	return &patterns.PatternsReport{
		Candidates: []patterns.Candidate{
			{
				Kind:        "guard_clause",
				Observation: "inverted conditional",
				Sites: []patterns.Site{
					{Path: "a.go", Line: 10, EndLine: 12, Name: "foo"},
					{Path: "a.go", Line: 30, Name: "bar"},
				},
			},
			{
				Kind:        "ccgraph_clone",
				Observation: "similar functions",
				Sites: []patterns.Site{
					{Path: "b.go", Line: 5, Name: "baz"},
				},
			},
		},
	}
}

func TestPatternMarkersFromReport(t *testing.T) {
	markers := patternMarkersFromReport(testPatternsReport())
	if len(markers) != 3 {
		t.Fatalf("expected 3 markers, got %d", len(markers))
	}
	// Sorted by path, then line.
	if markers[0].Path != "a.go" || markers[0].Line != 10 || markers[0].Kind != "guard_clause" {
		t.Fatalf("unexpected first marker: %+v", markers[0])
	}
	if markers[1].Line != 30 {
		t.Fatalf("unexpected second marker: %+v", markers[1])
	}
	if markers[2].Path != "b.go" || markers[2].Kind != "ccgraph_clone" {
		t.Fatalf("unexpected third marker: %+v", markers[2])
	}
}

func TestPatternMarkersFromReportNil(t *testing.T) {
	if got := patternMarkersFromReport(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := patternMarkersFromReport(&patterns.PatternsReport{}); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestPatternPathMatches(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"a.go", "a.go", true},
		{"./a.go", "a.go", true}, // tolerant of ./ prefix
		{"pkg/a.go", "a.go", true},
		{"a.go", "pkg/a.go", true},
		{"a.go", "b.go", false},
		{"aa.go", "a.go", false},
	}
	for _, c := range cases {
		if got := patternPathMatches(c.a, c.b); got != c.want {
			t.Errorf("patternPathMatches(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPatternMarkersAtLine(t *testing.T) {
	markers := patternMarkersFromReport(testPatternsReport())
	// Line 11 is inside the 10-12 range.
	got := patternMarkersAtLine(markers, "a.go", 11)
	if len(got) != 1 || got[0].Kind != "guard_clause" {
		t.Fatalf("expected guard_clause at line 11, got %+v", got)
	}
	// Line 30 has no EndLine; matches exactly.
	got = patternMarkersAtLine(markers, "a.go", 30)
	if len(got) != 1 {
		t.Fatalf("expected 1 marker at line 30, got %+v", got)
	}
	// No markers on other files.
	if got := patternMarkersAtLine(markers, "c.go", 10); len(got) != 0 {
		t.Fatalf("expected no markers, got %+v", got)
	}
	// Dedupes by kind.
	dupes := []PatternMarker{
		{Kind: "guard_clause", Path: "a.go", Line: 10},
		{Kind: "guard_clause", Path: "a.go", Line: 10},
		{Kind: "typednil", Path: "a.go", Line: 10},
	}
	if got := patternMarkersAtLine(dupes, "a.go", 10); len(got) != 2 {
		t.Fatalf("expected 2 deduped markers, got %+v", got)
	}
}

func TestTogglePatterns(t *testing.T) {
	m := Model{}
	if m.showPatterns {
		t.Fatal("expected showPatterns false on zero model")
	}
	m = m.togglePatterns()
	if !m.showPatterns {
		t.Fatal("expected showPatterns true after toggle")
	}
	m = m.togglePatterns()
	if m.showPatterns {
		t.Fatal("expected showPatterns false after second toggle")
	}
}

func TestHighlightPatternLine(t *testing.T) {
	rendered := "some code"
	if got := highlightPatternLine(rendered, nil); got != rendered {
		t.Fatal("expected unchanged with no markers")
	}
	markers := []PatternMarker{{Kind: "guard_clause", Path: "a.go", Line: 1}}
	got := highlightPatternLine(rendered, markers)
	if got == rendered || len(got) <= len(rendered) {
		t.Fatal("expected background-wrapped output")
	}
}

func TestSourcePatternRows(t *testing.T) {
	if rows := sourcePatternRows(nil, 80); rows != nil {
		t.Fatal("expected nil for no markers")
	}
	markers := []PatternMarker{
		{Kind: "guard_clause", Path: "a.go", Line: 10},
		{Kind: "ccgraph_clone", Path: "a.go", Line: 10},
	}
	rows := sourcePatternRows(markers, 80)
	if len(rows) == 0 {
		t.Fatal("expected rows")
	}
	// Kinds sorted: ccgraph_clone before guard_clause.
	found := false
	for _, r := range rows {
		if containsAll(r, "ccgraph_clone", "guard_clause") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected both kinds in rows: %v", rows)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestSourcePatternRowCount(t *testing.T) {
	markers := patternMarkersFromReport(testPatternsReport())
	// Lines 10-12: one marker row at line 10 (dedupe by kind per line).
	if got := sourcePatternRowCount(markers, "a.go", 10, 12, 80); got != 1 {
		t.Fatalf("expected 1 pattern row, got %d", got)
	}
	// Wrong path: zero rows.
	if got := sourcePatternRowCount(markers, "z.go", 10, 12, 80); got != 0 {
		t.Fatalf("expected 0 pattern rows, got %d", got)
	}
}

func TestPToggleKeybinding(t *testing.T) {
	m := Model{}
	msg := tea.KeyPressMsg{Code: []rune("p")[0], Text: "p"}
	next, cmd, handled := m.updateGlobalKey(msg.Keystroke())
	if !handled {
		t.Fatal("expected p to be handled")
	}
	nm := next
	if !nm.showPatterns {
		t.Fatal("expected showPatterns true after p")
	}
	if cmd == nil {
		t.Fatal("expected background load command on first p")
	}
	// Second press toggles off, no reload.
	next2, cmd2, _ := nm.updateGlobalKey(msg.Keystroke())
	if next2.showPatterns {
		t.Fatal("expected showPatterns false after second p")
	}
	if cmd2 != nil {
		t.Fatal("expected no command when toggling off")
	}
}

func TestPatternsMsgUpdate(t *testing.T) {
	m := Model{patternsLoading: true}
	markers := []PatternMarker{{Kind: "guard_clause", Path: "a.go", Line: 1}}
	updated, _ := m.Update(patternsMsg{markers: markers})
	nm := updated.(Model)
	if nm.patternsLoading {
		t.Fatal("expected patternsLoading false after patternsMsg")
	}
	if len(nm.patternMarkers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(nm.patternMarkers))
	}
}
