package patternfix

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempGo(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFixDryRun(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int) int {
	if x > 0 {
		println("positive")
		println(x)
	} else {
		return -1
	}
	return x
}
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "if x <= 0 {") {
		t.Errorf("expected diff with inverted guard, got:\n%s", s)
	}
	if !strings.Contains(s, "1 fixable") {
		t.Errorf("expected fix count, got:\n%s", s)
	}
	// Dry-run must not modify the file.
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "} else {") {
		t.Error("dry-run modified the file")
	}
}

func TestFixApply(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int) int {
	if x > 0 {
		println("positive")
		println(x)
	} else {
		return -1
	}
	return x
}
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{"--apply", path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	content, _ := os.ReadFile(path)
	s := string(content)
	if !strings.Contains(s, "if x <= 0 {") {
		t.Errorf("expected file rewritten, got:\n%s", s)
	}
	if strings.Contains(s, "} else {") {
		t.Errorf("else should be gone, got:\n%s", s)
	}
}

func TestFixNoCandidates(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int) int {
	if x < 0 {
		return -1
	}
	return x
}
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "0 fixable") {
		t.Errorf("expected 0 fixes, got:\n%s", out.String())
	}
}

func TestFixBadKind(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), []string{"--kind", "bogus", "."}, &out)
	if err == nil {
		t.Error("expected error for bad kind")
	}
}

func TestFixAnemicModelKind(t *testing.T) {
	path := writeTempGo(t, `package p

type Order struct{ Total int }

func calcTotal(o *Order) int { return o.Total }

func discTotal(o *Order, pct int) int { return o.Total * pct / 100 }

func validTotal(o *Order) bool { return o.Total >= 0 }

func use(o *Order) int { return calcTotal(o) + discTotal(o, 10) }
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{"--kind", "anemic_model", path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "1 fixable candidate(s)") {
		t.Errorf("expected 1 fixable candidate, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "func (o *Order) calcTotal()") {
		t.Errorf("expected method conversion in diff, got:\n%s", out.String())
	}
}

func TestFixAllIncludesAnemicModel(t *testing.T) {
	path := writeTempGo(t, `package p

type Order struct{ Total int }

func calcTotal(o *Order) int { return o.Total }

func discTotal(o *Order, pct int) int { return o.Total * pct / 100 }

func validTotal(o *Order) bool { return o.Total >= 0 }
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{"--kind", "all", path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "func (o *Order) calcTotal()") {
		t.Errorf("expected anemic_model conversion in --kind all output, got:\n%s", out.String())
	}
}

// changedRepo builds a temp git repo with go.mod and one committed Go file,
// returning the dir. The caller chdirs into it to run --changed.
func changedRepo(t *testing.T, committed string) string {
	t.Helper()
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
	write("go.mod", "module example.com/tmp\n\ngo 1.24\n")
	write("main.go", committed)
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("add", "-A")
	run("commit", "-m", "clean")
	return dir
}

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(previous) }
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const changedClean = `package main

func touched(x int) int {
	if x > 0 {
		println("positive")
		println(x)
	} else {
		return -1
	}
	return x
}

func untouched(x int) int {
	if x > 0 {
		println("positive")
		println(x)
	} else {
		return -1
	}
	return x
}
`

// TestFixChangedOnlyFixesTouchedFunctions commits a file with two identical
// inverted guards, dirties only one (whitespace change so the guard stays),
// and verifies --changed fixes just the touched one while plain fix does both.
func TestFixChangedOnlyFixesTouchedFunctions(t *testing.T) {
	dir := changedRepo(t, changedClean)
	defer chdir(t, dir)()

	// Touch the first function with a comment so the diff marks it changed.
	dirty := strings.Replace(changedClean, "func touched(x int) int {", "func touched(x int) int {\n\t// touched", 1)
	writeRepoFile(t, dir, "main.go", dirty)

	var out bytes.Buffer
	if err := Run(context.Background(), []string{"--changed", "."}, &out); err != nil {
		t.Fatalf("Run --changed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "in changed functions") {
		t.Errorf("expected changed-mode message, got:\n%s", s)
	}
	if !strings.Contains(s, "1 fixable") {
		t.Errorf("expected 1 fixable candidate, got:\n%s", s)
	}

	// Plain fix (no --changed) sees both guards.
	out.Reset()
	if err := Run(context.Background(), []string{"."}, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "2 fixable") {
		t.Errorf("expected 2 fixable candidates without --changed, got:\n%s", out.String())
	}
}

// TestFixChangedApply writes only the touched function's fix.
func TestFixChangedApply(t *testing.T) {
	dir := changedRepo(t, changedClean)
	defer chdir(t, dir)()

	dirty := strings.Replace(changedClean, "func touched(x int) int {", "func touched(x int) int {\n\t// touched", 1)
	writeRepoFile(t, dir, "main.go", dirty)

	var out bytes.Buffer
	if err := Run(context.Background(), []string{"--changed", "--apply", "."}, &out); err != nil {
		t.Fatalf("Run --changed --apply: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	s := string(content)
	// The touched function's guard is inverted; the untouched one is not.
	touchedFixed := strings.Contains(s, "// touched\n\tif x <= 0 {")
	untouchedFixed := strings.Count(s, "if x <= 0 {") > 1
	if !touchedFixed {
		t.Errorf("touched function not fixed:\n%s", s)
	}
	if untouchedFixed {
		t.Errorf("untouched function was fixed:\n%s", s)
	}
}

// TestFixChangedUnknownBase is an operational failure, not a silent no-op.
func TestFixChangedUnknownBase(t *testing.T) {
	dir := changedRepo(t, changedClean)
	defer chdir(t, dir)()

	var out bytes.Buffer
	err := Run(context.Background(), []string{"--changed", "--base", "nope", "."}, &out)
	if err == nil {
		t.Fatalf("expected error for unknown base, got:\n%s", out.String())
	}
}
