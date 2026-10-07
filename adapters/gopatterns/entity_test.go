package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
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

// loadTestPackage writes src to a temp module and loads it with
// packages.Load, returning the first package.
func loadTestPackage(t *testing.T, src string) *packages.Package {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testp\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0644); err != nil {
		t.Fatalf("p.go: %v", err)
	}
	cfg := &packages.Config{Dir: dir, Mode: packages.LoadSyntax}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages loaded")
	}
	return pkgs[0]
}

func missingIdentityNames(t *testing.T, src string) map[string]bool {
	t.Helper()
	pkg := loadTestPackage(t, src)
	// root is the temp dir: derive from the package's first file position
	filename := pkg.Fset.PositionFor(pkg.Syntax[0].Pos(), false).Filename
	root := filepath.Dir(filename)
	hits := findMissingIdentities(pkg, root)
	out := map[string]bool{}
	for _, h := range hits {
		out[h.TypeName] = true
	}
	return out
}

func TestMissingIdentitySkipsValueObjects(t *testing.T) {
	src := `package p
// Weights is pure data: no methods, never mutated.
type Weights struct {
	Mutation int64
	IO       int64
}
func use1(w Weights) int { return int(w.Mutation) }
func use2(w Weights) int { return int(w.IO) }
func use3(w Weights) int { return int(w.Mutation + w.IO) }
`
	names := missingIdentityNames(t, src)
	if names["Weights"] {
		t.Errorf("Weights is a value object (no methods, never mutated) and should be skipped")
	}
}

func TestMissingIdentityFlagsEntities(t *testing.T) {
	src := `package p
// Order is entity-like: it is mutated.
type Order struct {
	Total int
}
func (o *Order) ApplyDiscount(d int) { o.Total -= d }
func use1(o Order) int { return o.Total }
func use2(o Order) int { return o.Total }
func use3(o *Order) { o.ApplyDiscount(1) }
`
	names := missingIdentityNames(t, src)
	if !names["Order"] {
		t.Errorf("Order is mutated with 3+ uses: want missing_identity hit, got %v", names)
	}
}

func TestMissingIdentitySkipsImmutableWithMethods(t *testing.T) {
	src := `package p
// Weights has behavior but is never mutated: a value object, not an entity.
type Weights struct {
	Mutation int64
	IO       int64
}
func (w Weights) value() int64 { return w.Mutation + w.IO }
func use1(w Weights) int64 { return w.value() }
func use2(w Weights) int64 { return w.Mutation }
func use3(w Weights) int64 { return w.IO }
`
	names := missingIdentityNames(t, src)
	if names["Weights"] {
		t.Errorf("Weights is never mutated (value object with behavior) and should be skipped")
	}
}

func TestMissingIdentityFlagsMutatedStructs(t *testing.T) {
	src := `package p
// Counter has no methods but is mutated: entity-like, not a value object.
type Counter struct {
	N int
}
func use1(c *Counter) { c.N++ }
func use2(c Counter) int { return c.N }
func use3(c Counter) int { return c.N * 2 }
`
	names := missingIdentityNames(t, src)
	if !names["Counter"] {
		t.Errorf("Counter is mutated with 3+ uses: want missing_identity hit, got %v", names)
	}
}
