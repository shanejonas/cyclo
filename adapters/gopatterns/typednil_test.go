package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestTypedNilInterfaceBoundary(t *testing.T) {
	cases := []struct {
		name, body string
		want       int
	}{
		{"pointer guard", "var p *Target; x := p; _ = x == nil", 0},
		{"context pointer guard", "ctx := struct{Target *Target}{}; if x := ctx.Target; x != nil { _ = x }", 0},
		{"map guard", "var m map[string]any; x := m; _ = x == nil", 0},
		{"slice guard", "var s []int; x := s; _ = x != nil", 0},
		{"channel guard", "var c chan int; x := c; _ = nil == x", 0},
		{"function guard", "var f func(); x := f; _ = x == nil", 0},
		{"map lookup function guard", "registry := map[string]func(){}; factory := registry[\"missing\"]; _ = factory == nil", 0},
		{"untyped nil", "var x any = nil; _ = x == nil", 0},
		{"interface call", "x := resolve(); _ = x != nil", 0},
		{"interface assignment", "var source any; x := source; _ = x == nil", 0},
		{"nil interface conversion", "x := any(nil); _ = x == nil", 0},
		{"pointer boxed", "var p *Target; var x any = p; _ = x == nil", 1},
		{"pointer assignment", "var p *Target; var x any; x = p; _ = x != nil", 1},
		{"map boxed", "var m map[string]any; var x any = m; _ = nil == x", 1},
		{"slice boxed", "var s []int; var x any = s; _ = nil != x", 1},
		{"channel boxed", "var c chan int; var x any = c; _ = x == nil", 1},
		{"function boxed", "var f func(); var x any = f; _ = x == nil", 1},
		{"error boxed", "var p *Target; var x error = p; _ = x == nil", 1},
		{"explicit conversion", "var p *Target; x := any(p); _ = x == nil", 1},
		{"parenthesized conversion", "var p *Target; x := (any(p)); _ = x == nil", 1},
		{"named interface", "var p *Target; var x Box = p; _ = x == nil", 1},
		{"named slice", "var s Values; var x any = s; _ = x == nil", 1},
		{"named map", "var m Lookup; var x any = m; _ = x == nil", 1},
		{"named function", "var f Callback; var x any = f; _ = x == nil", 1},
		{"named pointer", "var p Pointer; var x any = p; _ = x == nil", 1},
		{"alias interface", "var p *Target; var x Alias = p; _ = x == nil", 1},
		{"non-nil literal", "var x any = &Target{}; _ = x == nil", 0},
		{"non-nil new", "var x any = new(Target); _ = x == nil", 0},
		{"non-nil map", "var x any = make(map[string]int); _ = x == nil", 0},
		{"constructor result", "p := build(); var x any = p; _ = x == nil", 0},
		{"checked constructor", "p, err := construct(); if err != nil { return }; var x any = p; _ = x == nil", 0},
		{"explicit nil pointer", "var x any = (*Target)(nil); _ = x == nil", 1},
		{"initialized nil pointer", "var p *Target = nil; var x any = p; _ = x == nil", 1},
		{"initialized nil slice", "var p []int = []int(nil); var x any = p; _ = x == nil", 1},
		{"alias of reassigned source", "var p *Target; p = &Target{}; var q = p; var x any = q; _ = x == nil", 0},
		{"source reassigned", "var p *Target; p = &Target{}; var x any = p; _ = x == nil", 0},
		{"source address escapes", "var p *Target; replace(&p); var x any = p; _ = x == nil", 0},
		{"interface overwritten", "var p *Target; var x any = p; x = &Target{}; _ = x == nil", 0},
		{"check before boxing", "var p *Target; var x any; _ = x == nil; x = p; _ = x", 0},

		{"shadowed name", "var p *Target; var x any = p; _ = x; { x := resolve(); _ = x == nil }", 0},
		{"outer checked after shadow", "var p *Target; var x any = p; { x := resolve(); _ = x == nil }; _ = x == nil", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn, fset, info := parseTypedNilFunc(t, tc.body)
			hits := findTypedNilHits(fn, fset, info)
			if len(hits) != tc.want {
				t.Fatalf("got %+v, want %d hits", hits, tc.want)
			}
		})
	}
}

func parseTypedNilFunc(t *testing.T, body string) (*ast.FuncDecl, *token.FileSet, *types.Info) {
	t.Helper()
	source := `package p
func check() {` + body + `}
type Target struct{}
func (*Target) Error() string { return "failed" }
func resolve() any { return nil }
func build() *Target { return &Target{} }
func construct() (*Target, error) { return &Target{}, nil }
func replace(p **Target) { *p = &Target{} }
type Box interface{}
type Alias = any
type Values []int
type Lookup map[string]any
type Callback func()
type Pointer *Target
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "typednil.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	config := types.Config{}
	if _, err := config.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatal(err)
	}
	return file.Decls[0].(*ast.FuncDecl), fset, info
}
