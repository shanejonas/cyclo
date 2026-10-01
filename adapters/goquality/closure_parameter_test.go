package goquality

import (
	"context"
	"os"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

func TestReducedClosureParameterMutationIsExternal(t *testing.T) {
	source, err := os.ReadFile("testdata/closure-parameters/reduced.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	root := qualitySourceModule(t, source)
	facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	f := findFunction(t, facts, "Mutate")
	if len(f.Mutations) != 2 || f.Mutations[0].Provenance != quality.Local || f.Mutations[1].Provenance != quality.External {
		t.Fatalf("parameter reassignment is local, but its field mutation is external: %+v", f.Mutations)
	}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range report.Functions[0].Effects {
		if effect.Kind == quality.MutationEffect && effect.Detail == "value.Count" {
			return
		}
	}
	t.Fatalf("closure's caller-state mutation missing from density: %+v", report.Functions[0])
}

func TestClosureParametersAndOwnedLocalsKeepTheirProvenance(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       quality.Provenance
	}{
		{"pointer parameter", "closure := func(value *State) { value.Count++ }; closure(shared)", quality.External},
		{"nested parameter", "closure := func() { inner := func(value *State) { if false { value = new(State) }; value.Count++ }; inner(shared) }; closure()", quality.External},
		{"slice parameter", "closure := func(values []int) { if false { values = make([]int, 1) }; values[0]++ }; closure([]int{0})", quality.External},
		{"value parameter", "closure := func(value State) { value.Count++ }; closure(*shared)", quality.Local},
		{"owned pointer", "closure := func() { owned := new(State); owned.Count++ }; closure()", quality.Local},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "package repro\ntype State struct { Count int }\nfunc Mutate(shared *State) { " + test.body + " }\n"
			root := qualitySourceModule(t, []byte(source))
			facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			f := findFunction(t, facts, "Mutate")
			if len(f.Mutations) == 0 || f.Mutations[len(f.Mutations)-1].Provenance != test.want {
				t.Fatalf("mutations = %+v, want final provenance %s", f.Mutations, test.want)
			}
		})
	}
}
