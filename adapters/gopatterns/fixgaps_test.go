package gopatterns

import (
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func TestEnumDispatchFix(t *testing.T) {
	src := `package main

type Color int

const (
	Red Color = iota
	Green
	Blue
)

func handle(c Color) {
	switch c {
	case Color.Red:
		doRed()
	case Color.Green:
		doGreen()
	case Color.Blue:
		doBlue()
	}
}

func doRed() {}
func doGreen() {}
func doBlue() {}
`
	spec := &patterns.FixSpec{
		Kind: patterns.EnumDispatch,
		File: "test.go",
		Line: 12, // line of switch
	}
	out, err := applyEnumDispatchFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("applyEnumDispatchFix failed: %v", err)
	}
	outStr := string(out)
	// Should contain dispatch table.
	if !strings.Contains(outStr, "colorDispatch") {
		t.Errorf("expected dispatch table, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "map[Color]func()") {
		t.Errorf("expected map[Color]func(), got:\n%s", outStr)
	}
	// Should not contain the original switch.
	if strings.Contains(outStr, "switch c {") {
		t.Errorf("switch should be replaced, got:\n%s", outStr)
	}
}

func TestEnumDispatchFixSkipsTypeSwitch(t *testing.T) {
	src := `package main

func handle(x interface{}) {
	switch v := x.(type) {
	case int:
		println(v)
	case string:
		println(v)
	}
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.EnumDispatch,
		File: "test.go",
		Line: 4,
	}
	_, err := applyEnumDispatchFix(spec, []byte(src))
	if err == nil {
		t.Error("expected error for type switch")
	}
}

func TestGenericFnFix(t *testing.T) {
	src := `package main

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.GenericFn,
		File: "test.go",
		Line: 3,
	}
	out, err := applyGenericFnFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("applyGenericFnFix failed: %v", err)
	}
	outStr := string(out)
	// Should contain generic function.
	if !strings.Contains(outStr, "func max[T") {
		t.Errorf("expected generic function, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "cmp.Ordered") {
		t.Errorf("expected cmp.Ordered constraint, got:\n%s", outStr)
	}
	// Should only have one max function (the generic one).
	count := strings.Count(outStr, "func max")
	if count != 1 {
		t.Errorf("expected 1 max function, got %d in:\n%s", count, outStr)
	}
	// Should import cmp.
	if !strings.Contains(outStr, `"cmp"`) {
		t.Errorf("expected cmp import, got:\n%s", outStr)
	}
}

func TestGenericFnFixSkipsDifferentBodies(t *testing.T) {
	src := `package main

func addInt(a, b int) int {
	return a + b
}

func subFloat(a, b float64) float64 {
	return a - b
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.GenericFn,
		File: "test.go",
		Line: 3,
	}
	_, err := applyGenericFnFix(spec, []byte(src))
	if err == nil {
		t.Error("expected error for different bodies")
	}
}
