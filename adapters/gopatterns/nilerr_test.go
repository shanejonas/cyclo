package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// parseNilErrFunc parses a single function and returns its AST and type info.
func parseNilErrFunc(t *testing.T, src string) (*ast.FuncDecl, *token.FileSet, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var fn *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fn == nil {
			fn = fd
			return false
		}
		return true
	})
	if fn == nil {
		t.Fatal("no function found")
	}
	conf := types.Config{Importer: nil, FakeImportC: true}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	// Type-check just this file for signature resolution.
	_, _ = conf.Check("test", fset, []*ast.File{f}, info)
	return fn, fset, info
}

func TestNilErrReturnNilErr_NoFinding(t *testing.T) {
	// (T, error) with `return nil, err`: the error is propagated, not swallowed.
	src := `package p
func f() (int, error) {
	err := foo()
	if err != nil {
		return nil, err
	}
	return 0, nil
}
func foo() error { return nil }
`
	fn, fset, info := parseNilErrFunc(t, src)
	hits := findNilErrHits(fn, fset, info)
	if len(hits) != 0 {
		t.Errorf("expected no findings for `return nil, err`, got %d", len(hits))
	}
}

func TestNilErrReturnNilNil_Finding(t *testing.T) {
	// (T, error) with `return nil, nil` inside error guard: error is swallowed.
	src := `package p
func f() (int, error) {
	err := foo()
	if err != nil {
		return nil, nil
	}
	return 0, nil
}
func foo() error { return nil }
`
	fn, fset, info := parseNilErrFunc(t, src)
	hits := findNilErrHits(fn, fset, info)
	if len(hits) != 1 {
		t.Errorf("expected 1 finding for `return nil, nil`, got %d", len(hits))
	}
}

func TestNilErrReturnErr_NoFinding(t *testing.T) {
	// error with `return err`: the error is propagated.
	src := `package p
func f() error {
	err := foo()
	if err != nil {
		return err
	}
	return nil
}
func foo() error { return nil }
`
	fn, fset, info := parseNilErrFunc(t, src)
	hits := findNilErrHits(fn, fset, info)
	if len(hits) != 0 {
		t.Errorf("expected no findings for `return err`, got %d", len(hits))
	}
}

func TestNilErrReturnNil_Finding(t *testing.T) {
	// error with `return nil` inside error guard: error is swallowed.
	src := `package p
func f() error {
	err := foo()
	if err != nil {
		return nil
	}
	return nil
}
func foo() error { return nil }
`
	fn, fset, info := parseNilErrFunc(t, src)
	hits := findNilErrHits(fn, fset, info)
	if len(hits) != 1 {
		t.Errorf("expected 1 finding for `return nil`, got %d", len(hits))
	}
}

func TestNilErrErrorInMiddlePosition(t *testing.T) {
	// Multiple returns with error in the middle: inspect the correct position.
	// (int, error, string) with `return 0, err, ""`: no finding (error propagated).
	src := `package p
func f() (int, error, string) {
	err := foo()
	if err != nil {
		return 0, err, ""
	}
	return 0, nil, ""
}
func foo() error { return nil }
`
	fn, fset, info := parseNilErrFunc(t, src)
	hits := findNilErrHits(fn, fset, info)
	if len(hits) != 0 {
		t.Errorf("expected no findings for error in middle position with `err`, got %d", len(hits))
	}
}

func TestNilErrErrorInMiddlePositionNil_Finding(t *testing.T) {
	// (int, error, string) with `return 0, nil, ""`: finding (error swallowed).
	src := `package p
func f() (int, error, string) {
	err := foo()
	if err != nil {
		return 0, nil, ""
	}
	return 0, nil, ""
}
func foo() error { return nil }
`
	fn, fset, info := parseNilErrFunc(t, src)
	hits := findNilErrHits(fn, fset, info)
	if len(hits) != 1 {
		t.Errorf("expected 1 finding for error in middle position with `nil`, got %d", len(hits))
	}
}
