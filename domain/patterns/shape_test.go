package patterns

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// typeOf parses `var _ = <src>` and returns the value's type.
func typeOf(t *testing.T, src string) types.Type {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", "package p\nvar _ = "+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, err := conf.Check("example.com/p", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Values) == 0 {
				continue
			}
			if tv, ok := info.Types[vs.Values[0]]; ok {
				return tv.Type
			}
		}
	}
	t.Fatal("no type found")
	return nil
}

func TestTypeClass(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"x"`, "string"},
		{`1`, "int"},
		{`true`, "bool"},
		{`1.5`, "float64"},
		{`[]int{}`, "[int]"},
		{`map[string]int{}`, "map[string]int"},
		{`make(chan string)`, "chan string"},
	}
	for _, c := range cases {
		if got := TypeClass(typeOf(t, c.src)); got != c.want {
			t.Errorf("TypeClass(%s) = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestTypeClassNamed(t *testing.T) {
	src := `package p

import "net/http"

type User struct{ Name string }

var _ = http.Request{}
var _ = User{}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	if _, err := conf.Check("example.com/p", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	classes := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		tv, ok := info.Types[lit]
		if !ok {
			return true
		}
		switch typ := lit.Type.(type) {
		case *ast.SelectorExpr:
			classes[typ.Sel.Name] = TypeClass(tv.Type)
		case *ast.Ident:
			classes[typ.Name] = TypeClass(tv.Type)
		}
		return true
	})
	// Stdlib named types keep their short name; user types are "_".
	if classes["Request"] != "Request" {
		t.Errorf("stdlib Request class = %q, want %q", classes["Request"], "Request")
	}
	if classes["User"] != "_" {
		t.Errorf("user User class = %q, want %q", classes["User"], "_")
	}
}

func TestSigClass(t *testing.T) {
	src := `package p

func simple(a string, b int) error { return nil }
func multi(a string) (int, error) { return 0, nil }
func none() {}
func variadic(prefix string, rest ...int) {}
func method(s string) {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	if _, err := conf.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for id, obj := range info.Defs {
		if fn, ok := obj.(*types.Func); ok {
			if sig, ok := fn.Type().(*types.Signature); ok {
				got[id.Name] = SigClass(sig)
			}
		}
	}
	want := map[string]string{
		"simple":   "fn(string, int) -> error",
		"multi":    "fn(string) -> (int, error)",
		"none":     "fn() -> ()",
		"variadic": "fn(string, ...int) -> ()",
		"method":   "fn(string) -> ()",
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("SigClass(%s) = %q, want %q", name, got[name], w)
		}
	}
}

func TestFuncID(t *testing.T) {
	src := `package p

type DB struct{}

func (d *DB) Exec(q string) {}
func Free(a int) {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.Default()}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	pkg, err := conf.Check("p", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	_ = pkg
	got := map[string]string{}
	for id, obj := range info.Defs {
		if fn, ok := obj.(*types.Func); ok {
			got[id.Name] = FuncID(fn)
		}
	}
	if got["Exec"] != "p.DB.Exec" {
		t.Errorf("FuncID(Exec) = %q, want %q", got["Exec"], "p.DB.Exec")
	}
	if got["Free"] != "p.Free" {
		t.Errorf("FuncID(Free) = %q, want %q", got["Free"], "p.Free")
	}
}

func TestFnv1aGolden(t *testing.T) {
	// Byte-identical with rstyle's FNV-1a (verified in the Phase 0 spike).
	if fnv1a([]byte("")) != 0xcbf29ce484222325 {
		t.Errorf("fnv1a(\"\") = %x, want cbf29ce484222325", fnv1a([]byte("")))
	}
}
