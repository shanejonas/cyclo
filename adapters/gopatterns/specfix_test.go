package gopatterns

import (
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
			"rulekey": "User|Age:>,Active:truthy",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	result := string(out)
	// Should contain the Specification type.
	if !strings.Contains(result, "type UserSpecification struct{}") {
		t.Errorf("expected UserSpecification type, got:\n%s", result)
	}
	// Should contain IsSatisfiedBy method.
	if !strings.Contains(result, "func (UserSpecification) IsSatisfiedBy(user User) bool") {
		t.Errorf("expected IsSatisfiedBy method, got:\n%s", result)
	}
	// Should rewrite both if conditions.
	count := strings.Count(result, "(UserSpecification{}).IsSatisfiedBy(user)")
	if count != 2 {
		t.Errorf("expected 2 rewritten conditions, got %d in:\n%s", count, result)
	}
	// Original conditions should be gone.
	if strings.Contains(result, "if user.Age > 18 && user.Active") {
		t.Errorf("original condition should be rewritten, got:\n%s", result)
	}
}

func TestApplySpecificationFixSkipsExisting(t *testing.T) {
	src := `package test

type UserSpecification struct{}

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
`
	spec := &patterns.FixSpec{
		Kind: patterns.Specification,
		File: "test.go",
		Line: 10,
		Params: map[string]string{
			"type":    "User",
			"varname": "user",
			"rulekey": "User|Age:>,Active:truthy",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// Should not duplicate the type.
	if strings.Count(string(out), "type UserSpecification struct{}") != 1 {
		t.Errorf("should not duplicate existing specification type")
	}
}
