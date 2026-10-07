package gopatterns

import (
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func TestApplySpecificationFix(t *testing.T) {
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
	if user.Age > 18 && user.Active {
		return true
	}
	return false
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.Specification,
		File: "test.go",
		Line: 10,
		Params: map[string]string{
			"type":    "User",
			"varname": "user",
			"cond":    "user.Age > 18 && user.Active",
			"rulekey": "User|Age:>:18,Active:truthy:",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	result := string(out)
	// Should contain the predicate method, not a Specification struct.
	if strings.Contains(result, "Specification") {
		t.Errorf("should not generate Specification struct, got:\n%s", result)
	}
	if !strings.Contains(result, "func (user User) IsAgeOver18AndActive() bool") {
		t.Errorf("expected predicate method, got:\n%s", result)
	}
	// Should rewrite both if conditions (2 call sites; declaration has receiver syntax).
	count := strings.Count(result, "IsAgeOver18AndActive()")
	if count != 3 { // 2 call sites + 1 declaration
		t.Errorf("expected 3 predicate references, got %d in:\n%s", count, result)
	}
	// Original conditions should be gone.
	if strings.Contains(result, "if user.Age > 18 && user.Active") {
		t.Errorf("original condition should be rewritten, got:\n%s", result)
	}
}

func TestApplySpecificationFixExternalType(t *testing.T) {
	src := `package test

import "example.com/api/apidomain"

func check1(args apidomain.AgentArgs) bool {
	if args.Action == "pause" || args.Action == "cancel" {
		return true
	}
	return false
}

func check2(args apidomain.AgentArgs) bool {
	if args.Action == "pause" || args.Action == "cancel" {
		return true
	}
	return false
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.Specification,
		File: "test.go",
		Line: 8,
		Params: map[string]string{
			"type":    "apidomain.AgentArgs",
			"varname": "args",
			"cond":    `args.Action == "pause" || args.Action == "cancel"`,
			"rulekey": `apidomain.AgentArgs|Action:==:"pause",Action:==:"cancel"`,
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	result := string(out)
	// External type: plain function, not a method (can't add methods cross-package).
	if !strings.Contains(result, "func isActionIsPauseOrActionIsCancel(args apidomain.AgentArgs) bool") {
		t.Errorf("expected predicate function for external type, got:\n%s", result)
	}
	count := strings.Count(result, "isActionIsPauseOrActionIsCancel(args)")
	if count != 2 {
		t.Errorf("expected 2 rewritten conditions, got %d in:\n%s", count, result)
	}
}

func TestApplySpecificationFixSkipsExisting(t *testing.T) {
	src := `package test

type User struct {
	Age    int
	Active bool
}

func (user User) IsAgeOver18AndActive() bool {
	return user.Age > 18 && user.Active
}

func check1(user User) bool {
	if user.IsAgeOver18AndActive() {
		return true
	}
	return false
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.Specification,
		File: "test.go",
		Line: 14,
		Params: map[string]string{
			"type":    "User",
			"varname": "user",
			"rulekey": "User|Age:>:18,Active:truthy:",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// Should not duplicate the predicate.
	if strings.Count(string(out), "func (user User) IsAgeOver18AndActive() bool") != 1 {
		t.Errorf("should not duplicate existing predicate")
	}
}

func TestSpecificationDistinctThresholds(t *testing.T) {
	// Two rules with different literals must not share a key.
	parse := func(s string) ast.Expr {
		e, err := parser.ParseExpr(s)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	key1, _, _ := specRuleKey(parse("input.Grounded < 0.60 || input.Value < 0.48"), nil)
	key2, _, _ := specRuleKey(parse("input.Grounded < 0.90 || input.Value < 0.90"), nil)
	if key1 == key2 {
		t.Errorf("different thresholds must have different keys: %q vs %q", key1, key2)
	}
	if key1 == "" || key2 == "" {
		t.Errorf("keys should not be empty: %q, %q", key1, key2)
	}
}
