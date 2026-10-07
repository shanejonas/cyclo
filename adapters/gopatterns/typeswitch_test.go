package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseTypeSwitchFunc(t *testing.T, src string) (*ast.FuncDecl, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		if d, ok := n.(*ast.FuncDecl); ok {
			fn = d
			return false
		}
		return true
	})
	if fn == nil {
		t.Fatal("no func found")
	}
	return fn, fset
}

func TestFindTypeSwitchesDetects(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak()
	case Cat:
		v.Speak()
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	hits := findTypeSwitches(fn, fset)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	h := hits[0]
	if h.Bound != "v" || h.Expr != "x" || h.Method != "Speak" {
		t.Errorf("wrong hit: %+v", h)
	}
	if len(h.Types) != 2 || h.Types[0] != "Dog" || h.Types[1] != "Cat" {
		t.Errorf("wrong types: %v", h.Types)
	}
}

func TestFindTypeSwitchesSkipsDifferentMethods(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Bark()
	case Cat:
		v.Meow()
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 0 {
		t.Errorf("expected 0 hits for different methods, got %d", len(hits))
	}
}

func TestFindTypeSwitchesSkipsDefaultWithBody(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak()
	case Cat:
		v.Speak()
	default:
		println("unknown")
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 0 {
		t.Errorf("expected 0 hits for default with body, got %d", len(hits))
	}
}

func TestFindTypeSwitchesSkipsSingleCase(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak()
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 0 {
		t.Errorf("expected 0 hits for single case, got %d", len(hits))
	}
}

func TestFindTypeSwitchesSkipsMultiStmt(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak()
		println("done")
	case Cat:
		v.Speak()
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 0 {
		t.Errorf("expected 0 hits for multi-stmt arm, got %d", len(hits))
	}
}

func TestFindTypeSwitchesSkipsValueSwitch(t *testing.T) {
	src := `package main
func f(x int) {
	switch x {
	case 1:
		println("one")
	case 2:
		println("two")
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 0 {
		t.Errorf("expected 0 hits for value switch, got %d", len(hits))
	}
}

func TestFindTypeSwitchesAllowsEmptyDefault(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak()
	case Cat:
		v.Speak()
	default:
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 1 {
		t.Errorf("expected 1 hit for empty default, got %d", len(hits))
	}
}

func TestFindTypeSwitchesWithArgs(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak("hello")
	case Cat:
		v.Speak("hello")
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	hits := findTypeSwitches(fn, fset)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Args != `"hello"` {
		t.Errorf("wrong args: %q", hits[0].Args)
	}
}

func TestFindTypeSwitchesSkipsDifferentArgs(t *testing.T) {
	src := `package main
func f(x interface{}) {
	switch v := x.(type) {
	case Dog:
		v.Speak("hello")
	case Cat:
		v.Speak("goodbye")
	}
}
`
	fn, fset := parseTypeSwitchFunc(t, src)
	if hits := findTypeSwitches(fn, fset); len(hits) != 0 {
		t.Errorf("expected 0 hits for different args, got %d", len(hits))
	}
}
