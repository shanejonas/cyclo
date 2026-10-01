package application

import "testing"

func TestSavedAnnotationScrollExtentMatchesRenderedRows(t *testing.T) {
	annotations := []Annotation{
		{},
		{Message: "a note that wraps across several narrow rows", Text: "first\nsecond\n"},
		{Message: "first\nsecond\n", Text: "first\r\n\tsecond\r\n"},
		{Message: "\x1b[31mcolored note\x1b[0m", Text: "\tindented\n\nlast"},
	}
	for _, annotation := range annotations {
		for _, width := range []int{-1, 0, 1, 12, 80} {
			want := len(savedAnnotationRows(annotation, width))
			if got := savedAnnotationRowCount(annotation, width); got != want {
				t.Fatalf("scroll extent = %d, rendered rows = %d, width = %d, note = %#v", got, want, width, annotation)
			}
		}
	}
}
