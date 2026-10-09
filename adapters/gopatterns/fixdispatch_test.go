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

const traitMethodSrc = `package main

import "fmt"

type Dog struct{ name string }

func (d Dog) Speak() string {
	s := d.sound()
	return fmt.Sprintf("%s says %s", d.name, s)
}

func (d Dog) sound() string { return "woof" }

type Cat struct{ name string }

func (c Cat) Speak() string {
	s := c.sound()
	return fmt.Sprintf("%s says %s", c.name, s)
}

func (c Cat) sound() string { return "meow" }
`

func traitMethodSpec() *patterns.FixSpec {
	return &patterns.FixSpec{
		Kind: patterns.TraitMethod,
		File: "p.go",
		Line: 8,
		Definitions: []string{
			"p.go:12:example.com/traitmethod.Dog.sound",
			"p.go:21:example.com/traitmethod.Cat.sound",
		},
		Params: map[string]string{},
	}
}

// TestApplyFixTraitMethodQualifiedDefinitions verifies the interface fixer
// resolves definitions that carry qualified names ("pkg.Type.method"), the
// form the miner actually emits. A bare-name-only lookup silently finds
// nothing and the fixer no-ops.
func TestApplyFixTraitMethodQualifiedDefinitions(t *testing.T) {
	out, err := ApplyFix(traitMethodSpec(), []byte(traitMethodSrc))
	if err != nil {
		t.Fatalf("ApplyFix failed: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "type Trait interface") {
		t.Errorf("expected generated Trait interface, got:\n%s", outStr)
	}
	if !strings.Contains(outStr, "sound() string") {
		t.Errorf("expected sound() string in the interface, got:\n%s", outStr)
	}
}

// TestApplyFixTraitMethodIdempotent verifies a second application is a
// no-op: re-running the fixer (or --phased re-mining) must not redeclare
// the interface and break the build. The second spec uses fresh line
// numbers, as the miner would re-emit them after the first fix shifted
// the file.
func TestApplyFixTraitMethodIdempotent(t *testing.T) {
	once, err := ApplyFix(traitMethodSpec(), []byte(traitMethodSrc))
	if err != nil {
		t.Fatalf("first ApplyFix failed: %v", err)
	}
	again := traitMethodSpec()
	again.Definitions = []string{
		"p.go:17:example.com/traitmethod.Dog.sound",
		"p.go:26:example.com/traitmethod.Cat.sound",
	}
	twice, err := ApplyFix(again, once)
	if err != nil {
		t.Fatalf("second ApplyFix failed: %v", err)
	}
	if string(twice) != string(once) {
		t.Errorf("second application changed the source; want idempotent no-op")
	}
	if n := strings.Count(string(twice), "type Trait interface"); n != 1 {
		t.Errorf("found %d Trait declarations, want exactly 1", n)
	}
}

// TestApplyFixCapabilitySet verifies the Capabilities interface aggregates
// every method in the set.
func TestApplyFixCapabilitySet(t *testing.T) {
	spec := &patterns.FixSpec{
		Kind: patterns.CapabilitySet,
		File: "p.go",
		Line: 8,
		Definitions: []string{
			"p.go:12:example.com/traitmethod.Dog.sound",
			"p.go:21:example.com/traitmethod.Cat.sound",
			"p.go:25:example.com/traitmethod.Dog.greeting",
			"p.go:29:example.com/traitmethod.Cat.greeting",
		},
		Params: map[string]string{},
	}
	src := traitMethodSrc + `
func (d Dog) greeting(prefix string) string { return prefix + " hello" }

func (c Cat) greeting(prefix string) string { return prefix + " hiss" }
`
	out, err := ApplyFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("ApplyFix failed: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "type Capabilities interface") {
		t.Errorf("expected generated Capabilities interface, got:\n%s", outStr)
	}
	for _, sig := range []string{"sound() string", "greeting(prefix string) string"} {
		if !strings.Contains(outStr, sig) {
			t.Errorf("expected %q in the interface, got:\n%s", sig, outStr)
		}
	}
}
