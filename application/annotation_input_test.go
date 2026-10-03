package application

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAnnotationInputKeepsTheRuneLimitAndRejectedDraft(t *testing.T) {
	cases := []struct {
		draft, incoming, want string
	}{
		{strings.Repeat("界", 159), "🙂", strings.Repeat("界", 159) + "🙂"},
		{strings.Repeat("界", 160), "🙂", strings.Repeat("界", 160)},
		{strings.Repeat("\xff", 160), "\xfe", strings.Repeat("\xff", 160)},
		{"keep this draft", "", "keep this draft"},
	}
	for _, tc := range cases {
		model := Model{sourceWorkspace: sourceWorkspace{annotating: true, annotationDraft: tc.draft}, revision: 7}
		next := model.updateAnnotationInput(tea.KeyPressMsg{Text: tc.incoming})
		if next.annotationDraft != tc.want || next.revision != 8 || !next.annotating {
			t.Fatalf("draft=%q input=%q: result draft=%q revision=%d annotating=%v", tc.draft, tc.incoming, next.annotationDraft, next.revision, next.annotating)
		}
	}
}

func TestSourceRangesKeepUnavailableAndTrailingRowsDistinct(t *testing.T) {
	cases := []struct {
		source     string
		start, end int
		valid      bool
	}{
		{"", 10, 10, false},
		{"one", 10, 10, true},
		{"one\n", 11, 11, true},
		{"one\r\n\ttwo\r\n", 11, 12, true},
		{"one\n", 10, 12, false},
		{"one\n", 9, 10, false},
		{"one\n", 11, 10, false},
	}
	for _, tc := range cases {
		model := sourceWorkspaceModel()
		model.report.Files[0].Functions[0].Source = tc.source
		if got := model.validSourceRange(tc.start, tc.end); got != tc.valid {
			t.Fatalf("source=%q range=%d-%d: valid=%v, want %v", tc.source, tc.start, tc.end, got, tc.valid)
		}
	}
}
