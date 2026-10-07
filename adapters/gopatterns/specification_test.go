package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestFindSpecificationHits(t *testing.T) {
	src := `package test
type User struct {
	Age    int
	Active bool
}
func check1(user User) bool {
	if user.Age > 18 && user.Active {
		return true
	}
	return false
}
func check2(user User) bool {
	if user.Age < 13 || !user.Active {
		return false
	}
	return true
}
func single(user User) bool {
	if user.Active {
		return true
	}
	return false
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	// We need types.Info; use a minimal approach - parse and find manually
	var hits int
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if ifStmt, ok := n.(*ast.IfStmt); ok {
				ops := flattenBoolOps(ifStmt.Cond)
				if len(ops) >= 2 {
					hits++
				}
			}
			return true
		})
	}
	// check1 has 1 (Age>18 && Active), check2 has 1 (Age<13 || !Active), single has 0
	if hits != 2 {
		t.Errorf("expected 2 multi-operand conditions, got %d", hits)
	}
}

func TestFlattenBoolOps(t *testing.T) {
	src := `package test
func f(a, b, c bool) {
	if a && b && c {}
	if a || b {}
	if a {}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var counts []int
	ast.Inspect(f, func(n ast.Node) bool {
		if ifStmt, ok := n.(*ast.IfStmt); ok {
			counts = append(counts, len(flattenBoolOps(ifStmt.Cond)))
		}
		return true
	})
	// a&&b&&c flattens to 3, a||b to 2, a (not bool op) to 1 (itself)
	if len(counts) != 3 || counts[0] != 3 || counts[1] != 2 || counts[2] != 1 {
		t.Errorf("unexpected flatten results: %v", counts)
	}
}

func TestSpecRejectsTwoVariableRule(t *testing.T) {
	// c.x != first.x references two variables; extracting it would leave
	// a dangling `first` in IsSatisfiedBy. Must not produce a rule key.
	src := `package test
type Cmp struct{ typeName string }
func f(c Cmp, first Cmp) bool {
	if c.typeName != first.typeName || c.typeName != "" {
		return true
	}
	return false
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if ifStmt, ok := n.(*ast.IfStmt); ok {
				key, _, _ := specRuleKey(ifStmt.Cond, nil)
				if key != "" {
					t.Errorf("two-variable rule should not produce a key, got %q", key)
				}
			}
			return true
		})
	}
}

func TestSpecAcceptsLiteralComparison(t *testing.T) {
	// user.Age > 18 compares against a literal: single-subject, valid.
	src := `package test
type User struct{ Age int }
func f(user User) bool {
	if user.Age > 18 && user.Age < 65 {
		return true
	}
	return false
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if ifStmt, ok := n.(*ast.IfStmt); ok {
				key, varName, _ := specRuleKey(ifStmt.Cond, nil)
				if key != "" && varName == "user" {
					found = true
				}
			}
			return true
		})
	}
	if !found {
		t.Error("literal comparison should produce a single-subject rule key")
	}
}
