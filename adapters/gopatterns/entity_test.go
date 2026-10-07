package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func parseTestFunc(t *testing.T, src string) (*ast.FuncDecl, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("no function found")
	}
	return fn, fset
}

func TestIsIDField(t *testing.T) {
	cases := []struct {
		name  string
		want  bool
	}{
		{"ID", true},
		{"Id", true},
		{"id", true},
		{"UUID", true},
		{"uuid", true},
		{"GUID", true},
		{"Name", false},
		{"Email", false},
		{"UserID", false}, // not exact match
	}
	for _, c := range cases {
		if got := isIDField(c.name); got != c.want {
			t.Errorf("isIDField(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFindMutableIdentitiesSkipsConstructors(t *testing.T) {
	src := `package p
type User struct { ID string }
func NewUser() *User { u := &User{}; u.ID = "1"; return u }
func CreateUser() *User { u := &User{}; u.ID = "2"; return u }
func UpdateUser(u *User) { u.ID = "3" }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var newFn, createFn, updateFn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			switch fd.Name.Name {
			case "NewUser":
				newFn = fd
			case "CreateUser":
				createFn = fd
			case "UpdateUser":
				updateFn = fd
			}
		}
	}
	if hits := FindMutableIdentities(newFn, fset); len(hits) != 0 {
		t.Errorf("NewUser should be skipped, got %d hits", len(hits))
	}
	if hits := FindMutableIdentities(createFn, fset); len(hits) != 0 {
		t.Errorf("CreateUser should be skipped, got %d hits", len(hits))
	}
	if hits := FindMutableIdentities(updateFn, fset); len(hits) != 1 {
		t.Errorf("UpdateUser should have 1 hit, got %d", len(hits))
	} else if hits[0].Field != "ID" {
		t.Errorf("hit field = %q, want ID", hits[0].Field)
	}
}

func TestFindMutableIdentitiesIgnoresNonID(t *testing.T) {
	src := `package p
type User struct { ID string; Name string }
func UpdateUser(u *User) { u.Name = "x" }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			fn = fd
		}
	}
	if hits := FindMutableIdentities(fn, fset); len(hits) != 0 {
		t.Errorf("non-ID assignment should be skipped, got %d hits", len(hits))
	}
}
