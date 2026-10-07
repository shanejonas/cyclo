package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// parseNamedFunc parses src and returns the named function.
func parseNamedFunc(t *testing.T, src, name string) (*ast.FuncDecl, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn, fset
		}
	}
	t.Fatalf("function %q not found", name)
	return nil, nil
}

const enumDispatchSrc = `package p

type Color int

const (
	Red Color = iota
	Blue
	Green
)

func doRed()   {}
func doBlue()  {}
func doGreen() {}

func process(c Color) {
	switch c {
	case Color.Red:
		doRed()
	case Color.Blue:
		doBlue()
	case Color.Green:
		doGreen()
	}
}
`

func TestFindEnumDispatchesDetectsValueSwitch(t *testing.T) {
	fn, fset := parseNamedFunc(t, enumDispatchSrc, "process")
	hits := findEnumDispatches(fn, fset)
	if len(hits) != 1 {
		t.Fatalf("expected 1 enum_dispatch hit, got %d", len(hits))
	}
	if hits[0].NumCases != 3 {
		t.Fatalf("expected 3 cases, got %d", hits[0].NumCases)
	}
	if hits[0].Line != 16 {
		t.Fatalf("expected hit at line 16, got %d", hits[0].Line)
	}
}

func TestFindEnumDispatchesSkipsTypeSwitch(t *testing.T) {
	src := `package p

type Dog struct{}
type Cat struct{}

func (d Dog) Bark() {}
func (c Cat) Meow() {}

func emit(x any) {
	switch v := x.(type) {
	case Dog:
		v.Bark()
	case Cat:
		v.Meow()
	}
}
`
	fn, fset := parseNamedFunc(t, src, "emit")
	if hits := findEnumDispatches(fn, fset); len(hits) != 0 {
		t.Fatalf("type switches are not enum dispatches, got %d hits", len(hits))
	}
}

func TestFindEnumDispatchesSkipsDefault(t *testing.T) {
	src := `package p

type Color int

const (
	Red Color = iota
	Blue
)

func doRed()  {}
func doBlue() {}

func process(c Color) {
	switch c {
	case Red:
		doRed()
	case Blue:
		doBlue()
	default:
	}
}
`
	fn, fset := parseNamedFunc(t, src, "process")
	if hits := findEnumDispatches(fn, fset); len(hits) != 0 {
		t.Fatalf("switches with default are not supported, got %d hits", len(hits))
	}
}

func TestFindEnumDispatchesSkipsMultiStmt(t *testing.T) {
	src := `package p

type Color int

const (
	Red Color = iota
	Blue
)

func doRed()  {}
func doBlue() {}

func process(c Color) {
	switch c {
	case Red:
		doRed()
		doRed()
	case Blue:
		doBlue()
	}
}
`
	fn, fset := parseNamedFunc(t, src, "process")
	if hits := findEnumDispatches(fn, fset); len(hits) != 0 {
		t.Fatalf("multi-statement cases are not supported, got %d hits", len(hits))
	}
}

func TestFindEnumDispatchesSkipsSingleCase(t *testing.T) {
	src := `package p

type Color int

const Red Color = iota

func doRed() {}

func process(c Color) {
	switch c {
	case Red:
		doRed()
	}
}
`
	fn, fset := parseNamedFunc(t, src, "process")
	if hits := findEnumDispatches(fn, fset); len(hits) != 0 {
		t.Fatalf("single-case switches are not dispatches, got %d hits", len(hits))
	}
}

func TestFindEnumDispatchesSkipsNoTag(t *testing.T) {
	src := `package p

func doA() {}
func doB() {}

func process(c int) {
	switch {
	case c == 1:
		doA()
	case c == 2:
		doB()
	}
}
`
	fn, fset := parseNamedFunc(t, src, "process")
	if hits := findEnumDispatches(fn, fset); len(hits) != 0 {
		t.Fatalf("tagless switches are not enum dispatches, got %d hits", len(hits))
	}
}

// TestEnumDispatchEndToEnd verifies the detector and fixer agree: every
// detected switch is fixable by the real fixer.
func TestEnumDispatchEndToEnd(t *testing.T) {
	fn, fset := parseNamedFunc(t, enumDispatchSrc, "process")
	hits := findEnumDispatches(fn, fset)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	// Build the FixSpec exactly like enumDispatchCandidates does and run
	// the real fixer on it.
	spec := &patterns.FixSpec{
		Kind: patterns.EnumDispatch,
		File: "test.go",
		Line: hits[0].Line,
	}
	out, err := applyEnumDispatchFix(spec, []byte(enumDispatchSrc))
	if err != nil {
		t.Fatalf("fixer declined the detector's hit: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "colorDispatch") {
		t.Errorf("expected dispatch table in output:\n%s", outStr)
	}
	if !strings.Contains(outStr, "map[Color]func()") {
		t.Errorf("expected map[Color]func() in output:\n%s", outStr)
	}
}
