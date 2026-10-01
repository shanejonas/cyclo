package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shanejonas/cyclo/domain/quality"
)

// This runs real generated Go programs and the checkout's typed analyzer. It
// belongs to normal CI and requires neither Tree-sitter nor a frozen binary.
func TestGeneratedOwnershipAndSyntaxInvariants(t *testing.T) {
	cases, err := corpus()
	if err != nil {
		t.Fatal(err)
	}
	check := runner{root: t.TempDir(), timeout: 30 * time.Second}
	mismatch, err := check.assess(context.Background(), program(cases), cases)
	if err != nil {
		t.Fatal(err)
	}
	if mismatch.Rule != "" {
		t.Fatalf("generated invariant failed: %+v", mismatch)
	}
}

func TestReductionRejectsIndependentProgramChanges(t *testing.T) {
	pair := []specimen{{Name: "Base", Base: "Base", Body: "local := shared; local.Count++", Changed: true},
		{Name: "Variant", Base: "Base", Body: "local := shared; (local).Count++; ((_)) = 0", Changed: true}}
	source := program(pair)
	if err := equivalentSource(source, pair); err != nil {
		t.Fatal(err)
	}
	// Both still change caller state, but the variant now has an extra write.
	modified := strings.Replace(string(source), "(local).Count++;", "(local).Count++; (local).Count++;", 1)
	if err := equivalentSource([]byte(modified), pair); !errors.Is(err, invalidCandidate) {
		t.Fatalf("non-equivalent candidate accepted: %v", err)
	}
	// Calls in discarded expressions are effects and cannot be normalized away.
	modified = strings.Replace(string(source), "((_)) = 0", "((_)) = len(shared.Items)", 1)
	if err := equivalentSource([]byte(modified), pair); !errors.Is(err, invalidCandidate) {
		t.Fatalf("non-literal discard normalized away: %v", err)
	}
}

func TestOracleDetectsBadGeneratorExpectation(t *testing.T) {
	cases := []specimen{{Name: "Base", Base: "Base", Body: "shared.Count++", Changed: false}}
	check := runner{root: t.TempDir(), timeout: 30 * time.Second}
	_, err := check.assess(context.Background(), program(cases), cases)
	if !errors.Is(err, invalidCandidate) {
		t.Fatalf("wrong runtime expectation accepted: %v", err)
	}
}

func TestInvariantChecksDetectHiddenWritesAndPhantomMutations(t *testing.T) {
	cases := []specimen{{Name: "Base", Base: "Base", Changed: true}, {Name: "Variant", Base: "Base", Changed: true}}
	facts := []quality.Function{
		{Location: quality.Location{Path: "input.go", Line: 1, Column: 1, Name: "p.Base"}, Source: "func Base() {}", Statements: 1, CodeLines: 1, Mutations: []quality.Mutation{{Root: "shared", RootID: "1", Line: 1, Provenance: quality.External}}},
		{Location: quality.Location{Path: "input.go", Line: 2, Column: 1, Name: "p.Variant"}, Source: "func Variant() {}", Statements: 1, CodeLines: 1, Mutations: []quality.Mutation{{Root: "shared", RootID: "2", Line: 2, Provenance: quality.External}, {Root: "<temporary>", RootID: "3", Line: 2, Provenance: quality.Local}}},
	}
	observed := map[string]bool{"Base": true, "Variant": true}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	mismatch, err := compare(cases, observed, facts, report)
	if err != nil || mismatch.Rule != "equivalent-syntax-evidence" {
		t.Fatalf("phantom mutation missed: %+v, %v", mismatch, err)
	}
	facts[0].Mutations[0].Provenance = quality.Local
	report, err = quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	mismatch, err = compare(cases, observed, facts, report)
	if err != nil || mismatch.Rule != "hidden-caller-write" {
		t.Fatalf("hidden write missed: %+v, %v", mismatch, err)
	}
}

func TestSavedReducedCounterexamplePassesFixedAnalyzer(t *testing.T) {
	for _, name := range []string{"phantom-discard", "parenthesized-allocation"} {
		t.Run(name, func(t *testing.T) {
			cases, source, err := loadInput("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			check := runner{root: t.TempDir(), timeout: 30 * time.Second}
			mismatch, err := check.assess(context.Background(), source, cases)
			if err != nil || mismatch.Rule != "" {
				t.Fatalf("saved counterexample: %+v, %v", mismatch, err)
			}
		})
	}
}
