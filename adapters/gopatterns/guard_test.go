package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseFunc parses a single-function source snippet for guard detection.
func parseFunc(t *testing.T, src string) (*ast.FuncDecl, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn, fset
		}
	}
	t.Fatal("no function found")
	return nil, nil
}

// The cobra completions.go shape: happy path trapped in the if body,
// early return in the else.
const invertedSrc = `package p

func complete(lastArg string) (string, error) {
	var flagName string
	if index := len(lastArg); index >= 0 {
		flagName = lastArg[:index]
		lastArg = lastArg[index+1:]
		_ = flagName
	} else {
		return "", nil
	}
	return lastArg, nil
}
`

func TestFindGuardClausesDetectsInverted(t *testing.T) {
	fn, fset := parseFunc(t, invertedSrc)
	hits := findGuardClauses(fn, fset)
	if len(hits) != 1 {
		t.Fatalf("expected 1 guard clause, got %d", len(hits))
	}
	if hits[0].Line != 5 {
		t.Fatalf("expected hit at line 5, got %d", hits[0].Line)
	}
}

func TestFindGuardClausesIgnoresProperGuard(t *testing.T) {
	src := `package p

func complete(lastArg string) (string, error) {
	if len(lastArg) < 0 {
		return "", nil
	}
	var flagName string
	flagName = lastArg[:1]
	_ = flagName
	return lastArg, nil
}
`
	fn, fset := parseFunc(t, src)
	if hits := findGuardClauses(fn, fset); len(hits) != 0 {
		t.Fatalf("proper guard clause must not be flagged, got %d hits", len(hits))
	}
}

func TestFindGuardClausesIgnoresElseIfChain(t *testing.T) {
	src := `package p

func f(x int) int {
	if x > 0 {
		y := x * 2
		z := y + 1
		return z
	} else if x < 0 {
		return -x
	} else {
		return 0
	}
}
`
	fn, fset := parseFunc(t, src)
	if hits := findGuardClauses(fn, fset); len(hits) != 0 {
		t.Fatalf("else-if chain must not be flagged, got %d hits", len(hits))
	}
}

func TestFindGuardClausesIgnoresTrivialBody(t *testing.T) {
	src := `package p

func f(x int) int {
	if x > 0 {
		return x
	} else {
		return -x
	}
}
`
	fn, fset := parseFunc(t, src)
	// Single-statement if body: inversion buys nothing.
	// (Also the if body ends in return here.)
	if hits := findGuardClauses(fn, fset); len(hits) != 0 {
		t.Fatalf("trivial body must not be flagged, got %d hits", len(hits))
	}
}

func TestFindGuardClausesIgnoresReturningIfBody(t *testing.T) {
	src := `package p

func f(x int) (int, error) {
	if x > 0 {
		y := x * 2
		z := y + 1
		return z, nil
	} else {
		return 0, nil
	}
}
`
	fn, fset := parseFunc(t, src)
	// Both branches return: already guard-shaped, nothing to invert.
	if hits := findGuardClauses(fn, fset); len(hits) != 0 {
		t.Fatalf("returning if body must not be flagged, got %d hits", len(hits))
	}
}

func TestFindGuardClausesIgnoresNoElse(t *testing.T) {
	src := `package p

func f(x int) int {
	if x > 0 {
		y := x * 2
		z := y + 1
		_ = z
		return y
	}
	return -x
}
`
	fn, fset := parseFunc(t, src)
	if hits := findGuardClauses(fn, fset); len(hits) != 0 {
		t.Fatalf("if without else must not be flagged, got %d hits", len(hits))
	}
}

func TestIsInvertedGuardNested(t *testing.T) {
	src := `package p

func f(a, b int) int {
	if a > 0 {
		x := a * 2
		if b > 0 {
			y := x + b
			z := y * 3
			_ = z
		} else {
			return -1
		}
		_ = x
	} else {
		return -2
	}
	return 0
}
`
	fn, fset := parseFunc(t, src)
	hits := findGuardClauses(fn, fset)
	// Both the outer and inner ifs trap their happy paths.
	if len(hits) != 2 {
		t.Fatalf("expected 2 nested guard clauses, got %d", len(hits))
	}
}
