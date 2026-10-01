package goquality

import (
	"context"
	"os"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

func TestReducedParenthesizedAssignmentKeepsMutationEffect(t *testing.T) {
	source, err := os.ReadFile("testdata/parenthesized-places/reduced.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	root := qualitySourceModule(t, source)
	facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	f := findFunction(t, facts, "Mutate")
	if len(f.Mutations) != 2 || f.Mutations[1].Provenance != quality.Unknown {
		t.Fatalf("mutations = %+v", f.Mutations)
	}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Functions) != 1 || report.Functions[0].Complete || report.Functions[0].DensityMilli == 0 {
		t.Fatalf("hidden external write: %+v", report)
	}
}

func TestParenthesizedPlacesMatchUnwrappedPlaces(t *testing.T) {
	for _, test := range []struct {
		name, body string
		count      int
		want       quality.Provenance
	}{
		{"shared pointer", "local := new(State); ((local)) = shared; local.Count++", 2, quality.Unknown},
		{"shared slice", "local := make([]int, 1); ((local)) = values; local[0]++", 2, quality.Unknown},
		{"owned pointer", "local := new(State); ((local)) = new(State); local.Count++", 2, quality.Local},
		{"branch assignment", "local := new(State); if false { ((local)) = shared }; local.Count++", 2, quality.Unknown},
		{"multiple assignment", "local := new(State); value := 0; ((local)), value = shared, 1; _ = value; local.Count++", 3, quality.Unknown},
		{"blank discard", "(( _ )) = 42", 0, quality.Local},
		{"mixed blank", "value := 0; ((_)), value = 42, 1; _ = value", 1, quality.Local},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("package repro\ntype State struct { Count int }\nfunc Mutate(shared *State, values []int) { " + test.body + " }\n")
			root := qualitySourceModule(t, source)
			facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			f := findFunction(t, facts, "Mutate")
			if len(f.Mutations) != test.count {
				t.Fatalf("mutations = %+v, want %d", f.Mutations, test.count)
			}
			if test.count > 0 && f.Mutations[test.count-1].Provenance != test.want {
				t.Fatalf("final mutation = %+v, want %s", f.Mutations[test.count-1], test.want)
			}
		})
	}
}
