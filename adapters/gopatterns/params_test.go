package gopatterns

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// checkFunc type-checks a single-function snippet and returns the FuncDecl
// with its types.Info.
func checkFunc(t *testing.T, src string) (*ast.FuncDecl, *types.Info) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("test", fset, []*ast.File{f}, info); err != nil {
		t.Fatalf("type-check: %v", err)
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn, info
		}
	}
	t.Fatal("no function found")
	return nil, nil
}

func TestPrimitiveParamsBasic(t *testing.T) {
	fn, info := checkFunc(t, `package p
func Transfer(fromID string, toID string, amount int, currency string) {}
`)
	got := primitiveParams(fn, info)
	want := []patterns.ParamInfo{
		{"fromID", "string"}, {"toID", "string"},
		{"amount", "int"}, {"currency", "string"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("param %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestPrimitiveParamsSkipsNamedTypes(t *testing.T) {
	fn, info := checkFunc(t, `package p
type UserID string
func Find(id UserID, name string, age int) {}
`)
	got := primitiveParams(fn, info)
	// UserID is a named type, not a basic: excluded.
	if len(got) != 2 {
		t.Fatalf("expected 2 primitive params, got %v", got)
	}
	if got[0].Name != "name" || got[1].Name != "age" {
		t.Errorf("unexpected params: %v", got)
	}
}

func TestPrimitiveParamsSkipsStructs(t *testing.T) {
	fn, info := checkFunc(t, `package p
type Config struct{ X int }
func Run(c Config, verbose bool) {}
`)
	got := primitiveParams(fn, info)
	if len(got) != 1 || got[0].Name != "verbose" {
		t.Errorf("expected only verbose bool, got %v", got)
	}
}

func TestPrimitiveParamsNoParams(t *testing.T) {
	fn, info := checkFunc(t, `package p
func Empty() {}
`)
	if got := primitiveParams(fn, info); len(got) != 0 {
		t.Errorf("expected no params, got %v", got)
	}
}
