package gopatterns

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// typeCheck parses and type-checks src, returning the file, fset, info,
// and the checked package.
func typeCheck(t *testing.T, src string) (*ast.File, *token.FileSet, *types.Info, *types.Package) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("p", fset, []*ast.File{f}, info)
	if err != nil {
		t.Fatalf("type check: %v", err)
	}
	return f, fset, info, pkg
}

func findFunc(f *ast.File, name string) *ast.FuncDecl {
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == name {
			return fd
		}
	}
	return nil
}

func TestFindAggregateMods(t *testing.T) {
	src := `package p
type Order struct { Total int }
type OrderLine struct { Price int }
func checkout(o *Order, l *OrderLine) {
	o.Total = 100
	l.Price += 50
}
func single(o *Order) {
	o.Total = 1
}
`
	f, _, info, _ := typeCheck(t, src)
	hits := FindAggregateMods(findFunc(f, "checkout"), info)
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	names := map[string]bool{}
	for _, h := range hits {
		names[h.TypeName] = true
	}
	if !names["Order"] || !names["OrderLine"] {
		t.Errorf("got %v, want Order and OrderLine", names)
	}
	hits = FindAggregateMods(findFunc(f, "single"), info)
	if len(hits) != 1 || hits[0].TypeName != "Order" {
		t.Errorf("single: got %v, want [Order]", hits)
	}
}

func TestFindAggregateModsSkipsNonStruct(t *testing.T) {
	src := `package p
func f(m map[string]int, s []int) {
	m["a"] = 1
	s[0] = 2
}
`
	f, _, info, _ := typeCheck(t, src)
	if hits := FindAggregateMods(findFunc(f, "f"), info); len(hits) != 0 {
		t.Errorf("got %v, want none (maps/slices are not structs)", hits)
	}
}

func TestIsRepositoryFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"internal/repository/order.go", true},
		{"internal/repo/order.go", true},
		{"pkg/repo.go", true},
		{"dao/user.go", true},
		{"store/session.go", true},
		{"adapters/sqlite/annotations.go", true}, // adapters/ is the repo layer (Shane's convention)
		{"adapters/gopatterns/ddd.go", true},     // any adapters/ path is infrastructure
		{"adapters/goquality/analyzer.go", true},
		{"internal/service/order.go", false},
		{"pkg/reporter.go", false}, // "reporter" is not "repo"
		{"internal/restore.go", false},
		{"domain/order/service.go", false},
	}
	for _, c := range cases {
		if got := isRepositoryFile(c.path); got != c.want {
			t.Errorf("isRepositoryFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestFindDbCallsHeuristic(t *testing.T) {
	src := `package p
type DB struct{}
func (d *DB) Query(q string) {}
func (d *DB) Exec(q string) {}
func checkout(db *DB) {
	db.Query("select")
	db.Exec("update")
}
func pure(x int) int { return x + 1 }
`
	f, fset, info, _ := typeCheck(t, src)
	hits := FindDbCalls(findFunc(f, "checkout"), fset, info, "service/order.go")
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].Call != "db.Query" {
		t.Errorf("call = %q, want db.Query", hits[0].Call)
	}
	if hits := FindDbCalls(findFunc(f, "pure"), fset, info, "service/order.go"); len(hits) != 0 {
		t.Errorf("pure: got %d hits, want 0", len(hits))
	}
}

func TestFindDbCallsSkipsRepositoryFiles(t *testing.T) {
	src := `package p
type DB struct{}
func (d *DB) Query(q string) {}
func findAll(db *DB) { db.Query("select") }
`
	f, fset, info, _ := typeCheck(t, src)
	if hits := FindDbCalls(findFunc(f, "findAll"), fset, info, "internal/repository/order.go"); len(hits) != 0 {
		t.Errorf("got %d hits, want 0 (repository file)", len(hits))
	}
}

func TestFindFactoryLits(t *testing.T) {
	src := `package p
type Config struct {
	Host string
	Port int
	User string
	Pass string
	Name string
	DB   string
}
func a() {
	c := Config{Host: "h", Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
	_ = c
}
func b() {
	c := Config{Host: "x", Port: 2, User: "v", Pass: "w", Name: "m", DB: "z"}
	_ = c
}
func small() {
	c := Config{Host: "h"}
	_ = c
}
func validated(host string) Config {
	if host == "" {
		host = "localhost"
	}
	return Config{Host: host, Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
}
func fallible(host string) (Config, error) {
	if host == "" {
		return Config{}, nil
	}
	return Config{Host: host, Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}, nil
}
`
	f, fset, info, pkg := typeCheck(t, src)
	hits := findFactoryLits(findFunc(f, "a"), fset, info, pkg)
	if len(hits) != 1 {
		t.Fatalf("a: got %d hits, want 1", len(hits))
	}
	if hits[0].TypeName != "Config" || hits[0].NumFields != 6 {
		t.Errorf("hit = %+v, want Config with 6 fields", hits[0])
	}
	if hits[0].HasLogic {
		t.Errorf("a: HasLogic = true, want false (plain field assignment)")
	}
	if hits := findFactoryLits(findFunc(f, "small"), fset, info, pkg); len(hits) != 0 {
		t.Errorf("small: got %d hits, want 0 (under threshold)", len(hits))
	}
	// Validation logic: if statement setting defaults.
	hits = findFactoryLits(findFunc(f, "validated"), fset, info, pkg)
	if len(hits) != 1 {
		t.Fatalf("validated: got %d hits, want 1", len(hits))
	}
	if !hits[0].HasLogic {
		t.Errorf("validated: HasLogic = false, want true (if statement)")
	}
	// Fallible construction: returns (Config, error).
	hits = findFactoryLits(findFunc(f, "fallible"), fset, info, pkg)
	if len(hits) != 1 {
		t.Fatalf("fallible: got %d hits, want 1", len(hits))
	}
	if !hits[0].HasLogic {
		t.Errorf("fallible: HasLogic = false, want true (returns error)")
	}
}

func TestFindFactoryLitsDistantIf(t *testing.T) {
	src := `package p
type Config struct {
	Host string
	Port int
	User string
	Pass string
	Name string
	DB   string
}
func distant(flag bool) Config {
	if flag {
		println("unrelated")
	}
	x := 1
	y := 2
	z := 3
	w := 4
	v := 5
	_ = x + y + z + w + v
	return Config{Host: "h", Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
}
func typeRef(name string) Config {
	c := Config{Host: "h", Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
	x := 1
	y := 2
	z := 3
	w := 4
	v := 5
	u := 6
	_ = x + y + z + w + v + u
	_ = name
	if c.Host == "" {
		c.Host = "localhost"
	}
	return c
}
`
	f, fset, info, pkg := typeCheck(t, src)
	// If statement is 8+ lines above the literal and doesn't reference
	// the type: not construction logic.
	hits := findFactoryLits(findFunc(f, "distant"), fset, info, pkg)
	if len(hits) != 1 {
		t.Fatalf("distant: got %d hits, want 1", len(hits))
	}
	if hits[0].HasLogic {
		t.Errorf("distant: HasLogic = true, want false (if is far and unrelated)")
	}
	// If statement references a variable of the struct type: construction
	// logic even though it's after the literal.
	hits = findFactoryLits(findFunc(f, "typeRef"), fset, info, pkg)
	if len(hits) != 1 {
		t.Fatalf("typeRef: got %d hits, want 1", len(hits))
	}
	if !hits[0].HasLogic {
		t.Errorf("typeRef: HasLogic = false, want true (if references Config var)")
	}
}
