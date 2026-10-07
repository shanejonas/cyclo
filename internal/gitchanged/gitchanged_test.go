package gitchanged

import (
	"testing"
)

func TestParseFileRanges(t *testing.T) {
	output := `diff --git a/foo.go b/foo.go
index 123..456 100644
--- a/foo.go
+++ b/foo.go
@@ -10,3 +20,4 @@ func foo() {
 context
-old
+new
+extra
 context
diff --git a/bar.go b/bar.go
index 123..456 100644
--- a/bar.go
+++ b/bar.go
@@ -4 +3,0 @@ func bar() {
-removed
`
	ranges := parseFileRanges(output)
	if len(ranges) != 2 {
		t.Fatalf("files = %v", ranges)
	}
	if len(ranges["foo.go"]) != 1 || ranges["foo.go"][0] != (LineRange{20, 23}) {
		t.Fatalf("foo.go ranges = %v", ranges["foo.go"])
	}
	// Pure deletion hunk touches the single new-side line it precedes.
	if len(ranges["bar.go"]) != 1 || ranges["bar.go"][0] != (LineRange{3, 3}) {
		t.Fatalf("bar.go ranges = %v", ranges["bar.go"])
	}
}

func TestParseFileRangesQuotedPath(t *testing.T) {
	output := "diff --git \"a/my dir/f.go\" \"b/my dir/f.go\"\n@@ -1 +1 @@\n-a\n+b\n"
	ranges := parseFileRanges(output)
	if len(ranges["my dir/f.go"]) != 1 {
		t.Fatalf("quoted path ranges = %v", ranges)
	}
}

func TestParseHunkRange(t *testing.T) {
	for _, test := range []struct {
		line   string
		want   LineRange
		wantOK bool
	}{
		{"@@ -10,3 +20,4 @@", LineRange{20, 23}, true},
		{"@@ -1 +1 @@", LineRange{1, 1}, true},
		{"@@ -4 +3,0 @@", LineRange{3, 3}, true},
		{" context", LineRange{}, false},
		{"\\ No newline at end of file", LineRange{}, false},
	} {
		t.Run(test.line, func(t *testing.T) {
			got, ok := parseHunkRange(test.line)
			if ok != test.wantOK || got != test.want {
				t.Fatalf("parseHunkRange(%q) = %v, %v", test.line, got, ok)
			}
		})
	}
}

func TestRootRelative(t *testing.T) {
	if got := RootRelative("a.go", "/root", "/root"); got != "a.go" {
		t.Fatalf("same dir: %q", got)
	}
	if got := RootRelative("a.go", "/root", "/root/sub"); got != "sub/a.go" {
		t.Fatalf("subdir: %q", got)
	}
}

// TestChangedEndToEnd builds a temp git repo, commits a clean file, then
