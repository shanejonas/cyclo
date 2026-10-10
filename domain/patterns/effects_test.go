package patterns

import (
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

// TestEffectBitsMapping verifies the Kind -> bitflag mapping.
func TestEffectBitsMapping(t *testing.T) {
	// This is in adapters/gopatterns, but we test the constants here.
	if EffectNone != 0 {
		t.Errorf("EffectNone should be 0, got %d", EffectNone)
	}
	// Each flag should be a distinct power of two.
	flags := []uint16{EffectMutates, EffectIO, EffectNetwork, EffectGlobal, EffectUnsafe, EffectTime, EffectRandom, EffectPanic, EffectUnknown}
	seen := map[uint16]bool{}
	for _, f := range flags {
		if f == 0 || f&(f-1) != 0 {
			t.Errorf("flag %d is not a power of two", f)
		}
		if seen[f] {
			t.Errorf("duplicate flag %d", f)
		}
		seen[f] = true
	}
}

// TestEffectDiffDetectsMismatch verifies that aligned nodes with different
// effects produce a HoleEffect.
func TestEffectDiffDetectsMismatch(t *testing.T) {
	a := &PdgNode{Kind: Call, CalleeID: "fmt.Println", Effects: EffectIO}
	b := &PdgNode{Kind: Call, CalleeID: "fmt.Println", Effects: EffectNone}
	d, ok := effectDiff(a, b)
	if !ok {
		t.Fatal("expected effect diff for IO vs pure")
	}
	if d.kind != HoleEffect {
		t.Errorf("expected HoleEffect, got %s", d.kind)
	}
}

// TestEffectDiffIgnoresMatch verifies that identical effects are not holes.
func TestEffectDiffIgnoresMatch(t *testing.T) {
	a := &PdgNode{Kind: Call, CalleeID: "fmt.Println", Effects: EffectIO}
	b := &PdgNode{Kind: Call, CalleeID: "os.Stdout.Write", Effects: EffectIO}
	if _, ok := effectDiff(a, b); ok {
		t.Error("expected no effect diff for identical effects")
	}
}

// TestEffectDiffIgnoresNonCall verifies that non-Call nodes are skipped.
func TestEffectDiffIgnoresNonCall(t *testing.T) {
	a := &PdgNode{Kind: Op, Effects: EffectIO}
	b := &PdgNode{Kind: Op, Effects: EffectNone}
	if _, ok := effectDiff(a, b); ok {
		t.Error("expected no effect diff for non-Call nodes")
	}
}

// TestCompatibleEffectsAllPure verifies that pure functions are compatible.
func TestCompatibleEffectsAllPure(t *testing.T) {
	mkFn := func() *FuncFacts {
		return &FuncFacts{Pdg: &Pdg{Nodes: []PdgNode{
			{Kind: Call, CalleeID: "strings.Join", Effects: EffectNone},
		}}}
	}
	fns := []*FuncFacts{mkFn(), mkFn()}
	if !compatibleEffects(fns) {
		t.Error("expected pure functions to be compatible")
	}
}

// TestCompatibleEffectsMismatch verifies that IO vs pure is incompatible.
func TestCompatibleEffectsMismatch(t *testing.T) {
	ioFn := &FuncFacts{Pdg: &Pdg{Nodes: []PdgNode{
		{Kind: Call, CalleeID: "fmt.Println", Effects: EffectIO},
	}}}
	pureFn := &FuncFacts{Pdg: &Pdg{Nodes: []PdgNode{
		{Kind: Call, CalleeID: "strings.Join", Effects: EffectNone},
	}}}
	if compatibleEffects([]*FuncFacts{ioFn, pureFn}) {
		t.Error("expected IO vs pure to be incompatible")
	}
}

// TestCompatibleEffectsBothIO verifies that two IO functions are compatible.
func TestCompatibleEffectsBothIO(t *testing.T) {
	mkFn := func(callee string) *FuncFacts {
		return &FuncFacts{Pdg: &Pdg{Nodes: []PdgNode{
			{Kind: Call, CalleeID: callee, Effects: EffectIO},
		}}}
	}
	fns := []*FuncFacts{mkFn("fmt.Println"), mkFn("os.Stdout.Write")}
	if !compatibleEffects(fns) {
		t.Error("expected two IO functions to be compatible")
	}
}

// TestQualityClassifyCallee verifies the exported classifier works.
func TestQualityClassifyCallee(t *testing.T) {
	// fmt.Println should be IO.
	if k := quality.ClassifyCallee("fmt.Println"); k != quality.IO {
		t.Errorf("expected IO for fmt.Println, got %s", k)
	}
	// strings.Join should be None (pure).
	if k := quality.ClassifyCallee("strings.Join"); k != quality.None {
		t.Errorf("expected None for strings.Join, got %s", k)
	}
}
