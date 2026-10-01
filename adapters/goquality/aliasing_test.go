package goquality

import (
	"context"
	"os"
	"testing"

	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/domain/quality"
)

func TestReducedAliasingMutationsRemainVisibleInQuality(t *testing.T) {
	for _, name := range []string{"indirect", "range", "allocated-wrapper"} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile("testdata/aliasing/" + name + "-reduced.go.txt")
			if err != nil {
				t.Fatal(err)
			}
			root := qualitySourceModule(t, source)
			analyzer := ReportAnalyzer{Complexity: gocyclo.NewAnalyzer(), Config: quality.DefaultConfig()}
			report, err := analyzer.Analyze([]string{root})
			if err != nil {
				t.Fatal(err)
			}
			if report.Quality == nil || report.Quality.Status != "ready" || len(report.Files) != 1 || len(report.Files[0].Functions) != 1 {
				t.Fatalf("quality unavailable: %+v", report)
			}
			q := report.Files[0].Functions[0].Quality
			if q == nil || q.Complete || q.DensityMilli == 0 || len(q.Effects) == 0 {
				t.Fatalf("caller-state mutation hidden from quality: %+v", q)
			}
			if len(q.MutationEvents) != 2 {
				t.Fatalf("missing write evidence: %+v", q.MutationEvents)
			}
		})
	}
}

func TestAliasingProvenancePreservesOwnedStorage(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       quality.Provenance
	}{
		{"local pointer", "local := new(State); local.Count++", quality.Local},
		{"local embedded value", "local := new(Value); local.Count++", quality.Local},
		{"local array copy", "local := [1]State{*shared}; local[0].Count++", quality.Local},
		{"local scalar address", "local := 0; alias := &local; *alias = 1", quality.Local},
		{"parenthesized pointer address", "local := new(State); holder := &(local); *holder = shared; local.Count++", quality.Unknown},
		{"range declaration", "for _, local := range []*State{shared} { local.Count++ }", quality.Unknown},
		{"range map key assignment", "local := new(State); for local = range map[*State]bool{shared: true} {}; local.Count++", quality.Unknown},
		{"nested allocated wrapper", "local := new(Nested); local.Pointer = Pointer{shared}; local.Count++", quality.Unknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "package repro\ntype State struct { Count int }\ntype Pointer struct { *State }\ntype Nested struct { Pointer }\ntype Value struct { State }\nfunc Mutate(shared *State) { " + test.body + " }\n"
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
