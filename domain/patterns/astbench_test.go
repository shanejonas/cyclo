package patterns

import (
	"testing"
	"time"
)

// TestASTFilterBenchmark compares CCGraph with and without the Stage 0
// AST pre-filter on a synthetic corpus. It reports timing and group
// counts for the PR description. This is a test, not a Go benchmark,
// so it runs with the normal test suite.
func TestASTFilterBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}
	// Build a corpus of PDGs with varying shapes. Pairs (i, i+1) for
	// even i are clones; odd pairs are distractors.
	const n = 500
	pdgs := make(map[string]*MiningGraph, n)
	names := make(map[string]string, n)
	astTypes := make(map[string]map[string]int, n)
	for i := 0; i < n; i++ {
		id := string(rune('a'+i/26)) + string(rune('a'+i%26)) + string(rune('0'+i%10))
		// Clone pairs share node count; distractors differ.
		size := 15 + (i%2)*5 + (i/10)%3
		pdgs[id] = ccTestPdg(size, i*100)
		names[id] = "func" + id
		// AST types: clone pairs get identical multisets.
		mult := map[string]int{"FuncDecl": 1, "BlockStmt": 2, "AssignStmt": size / 3}
		if i%2 == 0 {
			mult["ForStmt"] = 1
		}
		astTypes[id] = mult
	}

	// Warmup.
	CCGraphClones(pdgs, names)

	// Baseline: no AST filter (3 runs, take median).
	var plainTimes []time.Duration
	var plainGroups int
	for r := 0; r < 3; r++ {
		start := time.Now()
		plainGroups = len(CCGraphClones(pdgs, names))
		plainTimes = append(plainTimes, time.Since(start))
	}

	// With AST filter (3 runs, take median).
	var astTimes []time.Duration
	var astGroupCount int
	for r := 0; r < 3; r++ {
		start := time.Now()
		astGroupCount = len(CCGraphClonesWithAST(pdgs, names, astTypes))
		astTimes = append(astTimes, time.Since(start))
	}

	t.Logf("baseline (no AST): %d groups in %v (median of 3)", plainGroups, medianDuration(plainTimes))
	t.Logf("with AST filter:   %d groups in %v (median of 3)", astGroupCount, medianDuration(astTimes))
	if astGroupCount < plainGroups {
		t.Fatalf("AST filter found fewer groups (%d) than baseline (%d)",
			astGroupCount, plainGroups)
	}
}

// medianDuration returns the median of a duration slice.
func medianDuration(ds []time.Duration) time.Duration {
	// Simple insertion sort for tiny n.
	for i := 1; i < len(ds); i++ {
		for j := i; j > 0 && ds[j] < ds[j-1]; j-- {
			ds[j], ds[j-1] = ds[j-1], ds[j]
		}
	}
	return ds[len(ds)/2]
}
