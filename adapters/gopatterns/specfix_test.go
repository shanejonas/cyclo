package gopatterns

import (
	"go/format"
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

func TestPredicateNameEmptyString(t *testing.T) {
	// `token.AccessToken == "" || token.Error != ""` should not produce IsIs.
	parse := func(s string) ast.Expr {
		e, err := parser.ParseExpr(s)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	name := predicateName(parse(`token.AccessToken == "" || token.Error != ""`))
	if name != "IsAccessTokenEmptyOrErrorNotEmpty" {
		t.Errorf("got %q, want IsAccessTokenEmptyOrErrorNotEmpty", name)
	}
}

func TestApplySpecificationFixSkipsUnqualifiedExternal(t *testing.T) {
	// Type without qualifier and not declared locally: skip, don't emit broken code.
	src := `package test

func check1(r Response) bool {
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return true
	}
	return false
}

func check2(r Response) bool {
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return true
	}
	return false
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.Specification,
		File: "test.go",
		Line: 5,
		Params: map[string]string{
			"type":    "Response",
			"varname": "r",
			"rulekey": "Response|StatusCode:<:200,StatusCode:>=:300",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// Should be unchanged: no qualifier means we can't generate a safe signature.
	if string(out) != src {
		t.Errorf("should skip unqualified external type, got:\n%s", string(out))
	}
}

func TestApplySpecificationFixPreservesPointerType(t *testing.T) {
	// Pointer to external type: the predicate must take *http.Response,
	// not http.Response, or the call sites won't compile.
	src := `package test

import "net/http"

func check1(response *http.Response) bool {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return true
	}
	return false
}

func check2(response *http.Response) bool {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return true
	}
	return false
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.Specification,
		File: "test.go",
		Line: 5,
		Params: map[string]string{
			"type":    "*http.Response",
			"varname": "response",
			"rulekey": "*http.Response|StatusCode:<:200,StatusCode:>=:300",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	outStr := string(out)
	// The generated predicate must have the pointer type.
	if !strings.Contains(outStr, "response *http.Response") {
		t.Errorf("predicate should take *http.Response, got:\n%s", outStr)
	}
	// And it must compile (parse + typecheck via gofmt at least).
	if _, err := format.Source(out); err != nil {
		t.Errorf("output not gofmt-clean: %v", err)
	}
}
