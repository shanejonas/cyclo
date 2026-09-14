package gocyclo

import "testing"

func TestDeletionOnlyHunkBoundaries(t *testing.T) {
	for _, test := range []struct {
		name, diff       string
		oldLine, newLine int
	}{
		{"before first line", "@@ -1 +0,0 @@\n-removed\n", 1, 1},
		{"inside function", "@@ -4 +3,0 @@\n-removed\n", 4, 4},
		{"after last line", "@@ -8 +7,0 @@\n-removed\n\\ No newline at end of file\n", 8, 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			lines := parseDiff(test.diff)
			if len(lines) != 1 {
				t.Fatalf("lines = %+v", lines)
			}
			line := lines[0]
			if line.Kind != "deleted" || line.OldLine != test.oldLine || line.NewLine != test.newLine || line.Text != "removed" {
				t.Fatalf("incorrect deletion: %+v", line)
			}
		})
	}
}
