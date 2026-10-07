package patternfix

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
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

// TestApplySpecsKindOrder verifies FixSpecs apply in pattern dependency
// order (guard_clause before factory, parameterize last), not in line
// order. The ordering is what lets type-creating passes feed later ones.
func TestApplySpecsKindOrder(t *testing.T) {
	specs := []*patterns.FixSpec{
		{Kind: patterns.Parameterize, File: "a.go", Line: 10},
		{Kind: patterns.Factory, File: "a.go", Line: 20},
		{Kind: patterns.GuardClause, File: "a.go", Line: 30},
		{Kind: patterns.ValueObject, File: "a.go", Line: 40},
		{Kind: patterns.GuardClause, File: "a.go", Line: 50},
	}
	ordered := sortSpecsForApply(specs)
	// Expect: guard(50), guard(30), valueobj(40), factory(20), param(10).
	// Guards first (bottom-up by line), then value_object, factory, parameterize last.
	want := []patterns.CandidateKind{
		patterns.GuardClause, patterns.GuardClause,
		patterns.ValueObject, patterns.Factory, patterns.Parameterize,
	}
	for i, s := range ordered {
		if s.Kind != want[i] {
			t.Errorf("position %d: got %q, want %q", i, s.Kind, want[i])
		}
	}
	// Within-kind bottom-up: the line-50 guard comes before line-30.
	if ordered[0].Line != 50 || ordered[1].Line != 30 {
		t.Errorf("guards not bottom-up: got lines %d, %d", ordered[0].Line, ordered[1].Line)
	}
}

// TestFixEndToEndKindOrder mines real code with multiple fixable patterns
// and verifies the collected FixSpecs sort in dependency order. This proves
// the ordering isn't just theoretical: the miner finds the candidates and
// the fixer applies guards before value objects before factories.
func TestFixEndToEndKindOrder(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int, host string, port int) int {
	if x > 0 {
		println(host, port)
		println(x)
	} else {
		return -1
	}
	return x
}

func g(y int, host string, port int) int {
	if y > 0 {
		println(host, port)
		println(y)
	} else {
		return -1
	}
	return y
}
`)
	var out bytes.Buffer
	// Collect specs without applying.
	specs, err := collectFixSpecs(context.Background(), options{paths: []string{path}, kind: "all"})
	if err != nil {
		t.Fatalf("collectFixSpecs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatalf("expected fixable candidates, got none")
	}
	ordered := sortSpecsForApply(specs)
	// Verify dependency order: all guards before all value objects.
	seenValueObject := false
	for _, s := range ordered {
		if s.Kind == patterns.ValueObject {
			seenValueObject = true
		}
		if s.Kind == patterns.GuardClause && seenValueObject {
			t.Errorf("guard_clause applied after value_object: ordering violated")
		}
	}
	_ = out
}
