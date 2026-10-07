package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// mustParseAnemicFile parses src for fixer tests.
func mustParseAnemicFile(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return fset, f
}

const anemicOrderSrc = `package p

type Order struct {
	Items []string
	Total int
}

func calculateTotal(o *Order) int {
	sum := 0
	for _, item := range o.Items {
		sum += len(item)
	}
	o.Total = sum
	return sum
}

func applyDiscount(o *Order, pct int) {
	o.Total = o.Total * (100 - pct) / 100
}

func validateOrder(o *Order) bool {
	return len(o.Items) > 0 && o.Total >= 0
}

func use(o *Order) int {
	total := calculateTotal(o)
	applyDiscount(o, 10)
	if validateOrder(o) {
		return total
	}
	return -1
}
`

func TestFixAnemicModelsBasic(t *testing.T) {
	fset, f := mustParseAnemicFile(t, anemicOrderSrc)
	out, fixes, err := FixAnemicModels(fset, f, []byte(anemicOrderSrc))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 3 {
		t.Fatalf("expected 3 fixes, got %d: %+v", len(fixes), fixes)
	}
	for _, fx := range fixes {
		if fx.FixKind() != "anemic_model" {
			t.Errorf("wrong kind: %q", fx.FixKind())
		}
		if fx.TypeName != "Order" {
			t.Errorf("wrong type: %q", fx.TypeName)
		}
	}
	s := string(out)
	for _, want := range []string{
		"func (o *Order) calculateTotal() int",
		"func (o *Order) applyDiscount(pct int)",
		"func (o *Order) validateOrder() bool",
		"o.calculateTotal()",
		"o.applyDiscount(10)",
		"o.validateOrder()",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n%s", want, s)
		}
	}
	if strings.Contains(s, "func calculateTotal(o *Order)") {
		t.Errorf("original signature still present\n%s", s)
	}
}

func TestFixAnemicModelsIdempotent(t *testing.T) {
	fset, f := mustParseAnemicFile(t, anemicOrderSrc)
	out, _, err := FixAnemicModels(fset, f, []byte(anemicOrderSrc))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	fset2, f2 := mustParseAnemicFile(t, string(out))
	out2, fixes2, err := FixAnemicModels(fset2, f2, out)
	if err != nil {
		t.Fatalf("second fix: %v", err)
	}
	if len(fixes2) != 0 {
		t.Fatalf("second run should be a no-op, got %d fixes", len(fixes2))
	}
	if string(out2) != string(out) {
		t.Fatalf("second run changed output")
	}
}

func TestFixAnemicModelsSkipsExported(t *testing.T) {
	src := `package p

type Order struct{ Total int }

func CalculateTotal(o *Order) int { return o.Total }

func applyDiscount(o *Order, pct int) { o.Total -= pct }

func validateOrder(o *Order) bool { return o.Total >= 0 }
`
	fset, f := mustParseAnemicFile(t, src)
	_, fixes, err := FixAnemicModels(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("exported func should block the struct, got %d fixes", len(fixes))
	}
}

func TestFixAnemicModelsSkipsTooFew(t *testing.T) {
	src := `package p

type Order struct{ Total int }

func calculateTotal(o *Order) int { return o.Total }

func applyDiscount(o *Order, pct int) { o.Total -= pct }
`
	fset, f := mustParseAnemicFile(t, src)
	_, fixes, err := FixAnemicModels(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("2 funcs should not trigger, got %d fixes", len(fixes))
	}
}

func TestFixAnemicModelsSkipsMethodful(t *testing.T) {
	src := `package p

type Order struct{ Total int }

func (o *Order) existing() int { return o.Total }

func calculateTotal(o *Order) int { return o.Total }

func applyDiscount(o *Order, pct int) { o.Total -= pct }

func validateOrder(o *Order) bool { return o.Total >= 0 }
`
	fset, f := mustParseAnemicFile(t, src)
	_, fixes, err := FixAnemicModels(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("struct with methods should be skipped, got %d fixes", len(fixes))
	}
}

func TestFixAnemicModelsSkipsValueUse(t *testing.T) {
	src := `package p

type Order struct{ Total int }

func calculateTotal(o *Order) int { return o.Total }

func applyDiscount(o *Order, pct int) { o.Total -= pct }

func validateOrder(o *Order) bool { return o.Total >= 0 }

var fn = calculateTotal
`
	fset, f := mustParseAnemicFile(t, src)
	out, fixes, err := FixAnemicModels(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	for _, fx := range fixes {
		if fx.FuncName == "calculateTotal" {
			t.Fatalf("func used as value should be skipped: %+v", fixes)
		}
	}
	_ = out
}

func TestFixAnemicModelsSkipsNonFirstParam(t *testing.T) {
	src := `package p

type Order struct{ Total int }

func calculateTotal(x int, o *Order) int { return o.Total + x }

func applyDiscount(o *Order, pct int) { o.Total -= pct }

func validateOrder(o *Order) bool { return o.Total >= 0 }

func shipOrder(o *Order) bool { return o.Total > 0 }
`
	fset, f := mustParseAnemicFile(t, src)
	out, fixes, err := FixAnemicModels(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	// calculateTotal has the struct as second param: skipped, leaving 3.
	if len(fixes) != 3 {
		t.Fatalf("expected 3 fixes (non-first-param skipped), got %d", len(fixes))
	}
	if strings.Contains(string(out), "func (o *Order) calculateTotal") {
		t.Fatalf("calculateTotal should not have been converted")
	}
}

func TestFixAnemicModelsValueReceiver(t *testing.T) {
	src := `package p

type Point struct{ X, Y int }

func sumCoords(pt Point) int { return pt.X + pt.Y }

func scalePoint(pt Point, f int) Point { return Point{pt.X * f, pt.Y * f} }

func isOrigin(pt Point) bool { return pt.X == 0 && pt.Y == 0 }

func use(pt Point) int { return sumCoords(pt) + scalePoint(pt, 2).X }
`
	fset, f := mustParseAnemicFile(t, src)
	out, fixes, err := FixAnemicModels(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 3 {
		t.Fatalf("expected 3 fixes, got %d", len(fixes))
	}
	s := string(out)
	if !strings.Contains(s, "func (pt Point) sumCoords() int") {
		t.Errorf("value receiver not preserved\n%s", s)
	}
	if !strings.Contains(s, "pt.sumCoords()") {
		t.Errorf("call site not rewritten\n%s", s)
	}
}

func TestFixAnemicModelsParses(t *testing.T) {
	fset, f := mustParseAnemicFile(t, anemicOrderSrc)
	out, _, err := FixAnemicModels(fset, f, []byte(anemicOrderSrc))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "out.go", out, 0); err != nil {
		t.Fatalf("output does not parse: %v", err)
	}
}

const anemicServiceSrc = `package p

type Account struct {
	Balance int
}

type Money struct {
	Amount int
}

func getBalance(a *Account) int {
	return a.Balance
}

func setBalance(a *Account, v int) {
	a.Balance = v
}

func clearBalance(a *Account) {
	a.Balance = 0
}

// deposit touches two struct types: a domain service, not anemic.
func deposit(a *Account, m Money) {
	a.Balance += m.Amount
}

// transfer touches two struct types: a domain service, not anemic.
func transfer(from *Account, to *Account, m Money) {
	from.Balance -= m.Amount
	to.Balance += m.Amount
}
`

func TestFixAnemicModelsSkipsServiceLike(t *testing.T) {
	fset, f := mustParseAnemicFile(t, anemicServiceSrc)
	out, fixes, err := FixAnemicModels(fset, f, []byte(anemicServiceSrc))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	for _, fx := range fixes {
		if fx.FuncName == "transfer" {
			t.Fatalf("transfer is a domain service and must not be converted: %+v", fixes)
		}
	}
	s := string(out)
	if strings.Contains(s, "func (from *Account) transfer") {
		t.Fatalf("transfer should not become a method\n%s", s)
	}
	// The single-type funcs should still convert.
	if !strings.Contains(s, "func (a *Account) getBalance() int") {
		t.Fatalf("getBalance should still convert\n%s", s)
	}
}
