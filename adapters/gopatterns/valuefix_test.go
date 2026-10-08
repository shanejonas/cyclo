package gopatterns

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func fixValueSource(t *testing.T, src string) (string, []ValueFix) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, fixes, err := FixValueObjects(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	return string(out), fixes
}

const voFixture = `package main

func transfer(fromID string, toID string, amount int, currency string) int {
	total := amount * 2
	println(fromID, toID, total, currency)
	return total
}

func refund(txID string, amount int, currency string) int {
	println(txID, amount, currency)
	return amount
}

func quote(amount int, currency string, region string) int {
	println(amount, currency, region)
	return amount
}

func main() {
	transfer("a", "b", 100, "USD")
	refund("tx1", 50, "EUR")
	quote(25, "GBP", "UK")
}
`

func TestValueFixBasic(t *testing.T) {
	out, fixes := fixValueSource(t, voFixture)
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	fx := fixes[0]
	if fx.TypeName != "AmountCurrency" {
		t.Errorf("TypeName = %q, want AmountCurrency", fx.TypeName)
	}
	if fx.Kind != "value_object" {
		t.Errorf("Kind = %q, want value_object", fx.Kind)
	}
	for _, want := range []string{
		"type AmountCurrency struct",
		"Amount   int",
		"Currency string",
		"amountCurrency AmountCurrency",
		"amountCurrency.Amount",
		`AmountCurrency{Amount: 100, Currency: "USD"}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	// The old params must be gone from signatures.
	if strings.Contains(out, "amount int, currency string") {
		t.Errorf("old params still present\n%s", out)
	}
}

func TestValueFixIdempotent(t *testing.T) {
	out1, fixes1 := fixValueSource(t, voFixture)
	if len(fixes1) == 0 {
		t.Fatal("expected fixes on first run")
	}
	out2, fixes2 := fixValueSource(t, out1)
	if len(fixes2) != 0 {
		t.Errorf("second run should be a no-op, got %d fixes\n%s", len(fixes2), out2)
	}
}

func TestValueFixSkipsExported(t *testing.T) {
	src := `package main

func Transfer(amount int, currency string) int { return amount }

func refund(amount int, currency string) int { return amount }

func quote(amount int, currency string) int { return amount }
`
	_, fixes := fixValueSource(t, src)
	if len(fixes) != 0 {
		t.Errorf("exported function in clump should skip, got %d fixes", len(fixes))
	}
}

func TestValueFixSkipsAssignedParam(t *testing.T) {
	src := `package main

func a(amount int, currency string) int {
	amount = amount + 1
	return amount
}

func b(amount int, currency string) int { return amount }

func c(amount int, currency string) int { return amount }
`
	_, fixes := fixValueSource(t, src)
	if len(fixes) != 0 {
		t.Errorf("assigned param should skip, got %d fixes", len(fixes))
	}
}

func TestValueFixSkipsShadowing(t *testing.T) {
	src := `package main

func a(amount int, currency string) int {
	amount := 5
	return amount
}

func b(amount int, currency string) int { return amount }

func c(amount int, currency string) int { return amount }
`
	_, fixes := fixValueSource(t, src)
	if len(fixes) != 0 {
		t.Errorf("shadowed param should skip, got %d fixes", len(fixes))
	}
}

func TestValueFixSkipsMethod(t *testing.T) {
	src := `package main

type svc struct{}

func (s svc) a(amount int, currency string) int { return amount }

func b(amount int, currency string) int { return amount }

func c(amount int, currency string) int { return amount }
`
	_, fixes := fixValueSource(t, src)
	if len(fixes) != 0 {
		t.Errorf("method in clump should skip, got %d fixes", len(fixes))
	}
}

func TestValueFixTooFewFuncs(t *testing.T) {
	src := `package main

func a(amount int, currency string) int { return amount }

func b(amount int, currency string) int { return amount }
`
	_, fixes := fixValueSource(t, src)
	if len(fixes) != 0 {
		t.Errorf("2 functions should not clump, got %d fixes", len(fixes))
	}
}

func TestValueFixOutputCompiles(t *testing.T) {
	out, fixes := fixValueSource(t, voFixture)
	if len(fixes) == 0 {
		t.Fatal("expected fixes")
	}
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "test.go", out, parser.ParseComments); err != nil {
		t.Fatalf("fixed output does not parse: %v\n%s", err, out)
	}
}

func TestFindValueClumps(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", voFixture, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	clumps := FindValueClumps(f)
	if len(clumps) != 1 {
		t.Fatalf("expected 1 clump, got %d", len(clumps))
	}
	c := clumps[0]
	if len(c.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(c.Params))
	}
	if len(c.Funcs) != 3 {
		t.Fatalf("expected 3 funcs, got %d", len(c.Funcs))
	}
}

func TestValueObjectPreservesFuncAnnotation(t *testing.T) {
	// Regression: grouping pid/windowID must not move the // cyclo-allow
	// annotation for performCalculatorStep into the rewritten return.
	src := `package test

import "context"

// cyclo-allow(side_effect_density): Obtains a fresh observation.
func performCalculatorStep(ctx context.Context, pid int, windowID int) error {
	return saveCalculatorScreenshot(ctx, pid, windowID)
}

// cyclo-allow(side_effect_density): Another function.
func anotherStep(ctx context.Context, pid int, windowID int) error {
	return saveCalculatorScreenshot(ctx, pid, windowID)
}

func saveCalculatorScreenshot(ctx context.Context, pid int, windowID int) error {
	return nil
}
`
	out, _ := fixValueSource(t, src)
	// The annotation must be immediately above performCalculatorStep, not
	// inside the return expression.
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.Contains(line, "func performCalculatorStep") {
			if i == 0 || !strings.Contains(lines[i-1], "cyclo-allow") {
				t.Errorf("annotation not above performCalculatorStep:\n%s", out)
			}
		}
	}
	// The return expression must not contain a misplaced annotation.
	// Find the return line for performCalculatorStep and check until the
	// closing brace of that function.
	inPerform := false
	braceDepth := 0
	for _, line := range lines {
		if strings.Contains(line, "func performCalculatorStep") {
			inPerform = true
		}
		if inPerform {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if strings.Contains(line, "cyclo-allow") && !strings.Contains(line, "func ") {
				// Annotation inside function body = misplaced (should be above func).
				t.Errorf("annotation inside performCalculatorStep body:\n%s", out)
				break
			}
			if braceDepth == 0 && strings.Contains(line, "}") {
				break
			}
		}
	}
	// The struct literal must be well-formed (no broken line splits).
	if strings.Contains(out, "pidWindowID.\n") {
		t.Errorf("broken struct literal formatting:\n%s", out)
	}
}
