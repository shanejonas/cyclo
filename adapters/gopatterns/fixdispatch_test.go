package gopatterns

import (
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func TestApplyFixGuardClause(t *testing.T) {
	src := `package main

import "fmt"

func process(x int) int {
	if x > 0 {
		y := x * 2
		z := y + 1
		fmt.Println(y, z)
	} else {
		return -1
	}
	return 0
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.GuardClause,
		File: "main.go",
		Line: 6,
		Params: map[string]string{
			"if_line": "6",
		},
	}
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("ApplyFix failed: %v", err)
	}
	outStr := string(out)
	// The guard clause should be inverted.
	if !strings.Contains(outStr, "if x <= 0") {
		t.Errorf("expected inverted condition, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "return -1") {
		t.Errorf("expected early return, got:\n%s", outStr)
	}
	// The happy path should be at top level (not nested in if).
	if strings.Contains(outStr, "if x > 0 {") {
		t.Errorf("original if should be gone, got:\n%s", outStr)
	}
}

func TestApplyFixNilSpec(t *testing.T) {
	_, err := ApplyFix(nil, []byte("package main"))
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestApplyFixUnknownKind(t *testing.T) {
	spec := &patterns.FixSpec{
		Kind: "nonexistent",
		File: "main.go",
	}
	_, err := ApplyFix(spec, []byte("package main"))
	if err == nil {
		t.Error("expected error for unknown kind")
	}
}
