package qualitycheck

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
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
	if len(ranges["foo.go"]) != 1 || ranges["foo.go"][0] != (lineRange{20, 23}) {
		t.Fatalf("foo.go ranges = %v", ranges["foo.go"])
	}
	// Pure deletion hunk touches the single new-side line it precedes.
	if len(ranges["bar.go"]) != 1 || ranges["bar.go"][0] != (lineRange{3, 3}) {
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
		want   lineRange
		wantOK bool
	}{
		{"@@ -10,3 +20,4 @@", lineRange{20, 23}, true},
		{"@@ -1 +1 @@", lineRange{1, 1}, true},
		{"@@ -4 +3,0 @@", lineRange{3, 3}, true},
		{" context", lineRange{}, false},
		{"\\ No newline at end of file", lineRange{}, false},
	} {
		t.Run(test.line, func(t *testing.T) {
			got, ok := parseHunkRange(test.line)
			if ok != test.wantOK || got != test.want {
				t.Fatalf("parseHunkRange(%q) = %v, %v", test.line, got, ok)
			}
		})
	}
}

func TestTouchedFunctions(t *testing.T) {
	facts := []quality.Function{
		{Location: quality.Location{Path: "a.go", Line: 10, Name: "pkg.touched"}, Source: "func touched() {\n}\n"},
		{Location: quality.Location{Path: "a.go", Line: 20, Name: "pkg.untouched"}, Source: "func untouched() {\n}\n"},
		{Location: quality.Location{Path: "b.go", Line: 5, Name: "pkg.other"}, Source: "func other() {\n}\n"},
	}
	ranges := map[string][]lineRange{"a.go": {{11, 11}}}
	touched := touchedFunctions(facts, ranges, "/root", "/root")
	if !touched["a.go\x00pkg.touched"] {
		t.Fatal("touched function not detected")
	}
	if touched["a.go\x00pkg.untouched"] || touched["b.go\x00pkg.other"] {
		t.Fatal("untouched function reported as touched")
	}
}

func TestRootRelative(t *testing.T) {
	if got := rootRelative("a.go", "/root", "/root"); got != "a.go" {
		t.Fatalf("same dir: %q", got)
	}
	if got := rootRelative("a.go", "/root", "/root/sub"); got != "sub/a.go" {
		t.Fatalf("subdir: %q", got)
	}
}

// TestChangedEndToEnd builds a temp git repo, commits a clean file, then
// verifies --changed reports only findings in the modified function.
func TestChangedEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	clean := `package main

func touched() int {
	x := 0
	return x
}

func untouched() int {
	y := 0
	return y
}

func main() {}
`
	dirty := `package main

func touched() int {
	x := 0
	x = 1
	x = 2
	x = 3
	x = 4
	return x
}

func untouched() int {
	y := 0
	return y
}

func main() {}
`
	write("go.mod", "module example.com/tmp\n\ngo 1.24\n")
	write("main.go", clean)
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("add", "-A")
	run("commit", "-m", "clean")

	check := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		previous, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Chdir(previous) }()
		var output strings.Builder
		err = execute(context.Background(), optionsForChanged(args...), &output)
		return output.String(), err
	}

	// Clean tree: no findings.
	if output, err := check(t); err != nil {
		t.Fatalf("clean tree: %v\n%s", err, output)
	}

	// Dirty the touched function: --changed reports it, plain check would too.
	write("main.go", dirty)
	output, err := check(t)
	if !errors.Is(err, ErrFindings) {
		t.Fatalf("dirty tree err = %v\n%s", err, output)
	}
	if !strings.Contains(output, "touched") {
		t.Fatalf("missing touched finding:\n%s", output)
	}
	if strings.Contains(output, "untouched") {
		t.Fatalf("untouched function leaked into --changed output:\n%s", output)
	}

	// Explicit base behaves the same.
	output, err = check(t, "--base", "HEAD")
	if !errors.Is(err, ErrFindings) || !strings.Contains(output, "touched") {
		t.Fatalf("explicit base: %v\n%s", err, output)
	}

	// Unknown base is an operational failure, not findings.
	var stderr strings.Builder
	previous, _ := os.Getwd()
	_ = os.Chdir(dir)
	err = execute(context.Background(), optionsForChanged("--base", "nope"), &stderr)
	_ = os.Chdir(previous)
	if err == nil || errors.Is(err, ErrFindings) {
		t.Fatalf("unknown base err = %v", err)
	}
}

func optionsForChanged(args ...string) options {
	opts := options{format: "text"}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--changed":
			opts.changed = true
		case "--base":
			index++
			opts.base = args[index]
		}
	}
	if !opts.changed {
		opts.changed = true
	}
	return opts
}
