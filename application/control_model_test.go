package application

import (
	"math"
	"strings"
	"testing"
)

func TestRPCSourceScrollingKeepsBlankAndTrailingRows(t *testing.T) {
	cases := []struct {
		source string
		rows   int
	}{
		{"", 1},
		{"one\n", 2},
		{"one\n\nlast", 3},
		{"one\r\n\ttwo\r\n", 3},
		{"if err != nil {\n    return err\n}\n", 4},
	}
	for _, tc := range cases {
		model := sourceWorkspaceModel()
		model.report.Files[0].Functions[0].Source = tc.source
		reply := make(chan controlReply, 1)
		next, _ := model.scrollSource(controlCommand{lines: 100, reply: reply})
		if response := <-reply; response.err != nil {
			t.Fatal(response.err)
		}
		if next.sourceOffset != tc.rows-1 {
			t.Fatalf("scroll offset = %d, want %d for %q", next.sourceOffset, tc.rows-1, tc.source)
		}
		if rendered := sourceCodeLines(model.report.Files[0].Functions[0]); len(rendered) != tc.rows {
			t.Fatalf("rendered rows = %d, want %d for %q", len(rendered), tc.rows, tc.source)
		}
	}
}

func TestRPCSourceScrollingClampsExtremeDeltas(t *testing.T) {
	model := sourceWorkspaceModel()
	model.sourceOffset = 1
	for _, delta := range []int{math.MaxInt, math.MinInt} {
		reply := make(chan controlReply, 1)
		next, _ := model.scrollSource(controlCommand{lines: delta, reply: reply})
		if response := <-reply; response.err != nil {
			t.Fatal(response.err)
		}
		want := 0
		if delta > 0 {
			want = strings.Count(model.report.Files[0].Functions[0].Source, "\n")
		}
		if next.sourceOffset != want {
			t.Fatalf("delta %d: offset %d, want %d", delta, next.sourceOffset, want)
		}
	}
}
