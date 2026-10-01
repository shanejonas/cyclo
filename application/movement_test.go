package application

import (
	"math"
	"strings"
	"testing"
)

func TestMoveIndexClampsWithoutIntegerOverflow(t *testing.T) {
	for _, tc := range []struct {
		current, delta, length, want int
	}{
		{1, math.MaxInt, 5, 4},
		{-1, math.MinInt, 5, 0},
		{math.MaxInt, 1, 5, 4},
		{math.MinInt, -1, 5, 0},
		{math.MaxInt, math.MinInt, 5, 0},
		{math.MinInt, math.MaxInt, 5, 0},
		{2, 1, 5, 3},
		{2, -1, 5, 1},
		{4, 1, 5, 4},
		{0, -1, 5, 0},
		{1, math.MaxInt, 0, 0},
	} {
		if got := moveIndex(tc.current, tc.delta, tc.length); got != tc.want {
			t.Fatalf("moveIndex(%d, %d, %d) = %d, want %d", tc.current, tc.delta, tc.length, got, tc.want)
		}
	}
}

func TestSavedAnnotationScrollingClampsExtremeDeltas(t *testing.T) {
	model := sourceWorkspaceModel()
	model.sourceOffset = 1
	note := Annotation{Message: "note", Text: strings.Repeat("saved line\n", 40)}
	rows := len(savedAnnotationRows(note, paneContentWidth(model.sourcePaneWidth())))
	for _, delta := range []int{math.MaxInt, math.MinInt} {
		want := 0
		if delta > 0 {
			want = max(rows-model.sourceViewportHeight(), 0)
		}
		if next := model.scrollSavedAnnotation(note, delta); next.sourceOffset != want {
			t.Fatalf("delta %d: saved offset %d, want %d", delta, next.sourceOffset, want)
		}
	}
}
