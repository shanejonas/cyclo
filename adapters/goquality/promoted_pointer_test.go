package goquality

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/domain/quality"
)

func qualitySourceModule(t *testing.T, source []byte) string {
	t.Helper()
	root := t.TempDir()
	files := map[string][]byte{
		"go.mod":   []byte("module example.com/quality-repro\n\ngo 1.25.0\n"),
		"input.go": source,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReducedPromotedPointerMutationHasQualityEvidence(t *testing.T) {
	source, err := os.ReadFile("testdata/promoted-pointer/reduced.go.txt")
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
		t.Fatalf("quality report unavailable: %+v", report)
	}
	q := report.Files[0].Functions[0].Quality
	if q == nil || len(q.MutationEvents) != 1 || q.MutationEvents[0].Provenance != quality.Unknown {
		t.Fatalf("promoted pointer mutation incorrectly considered local: %+v", q)
	}
	if q.Complete || q.DensityMilli == 0 || len(q.Effects) != 1 || q.Effects[0].Kind != quality.UnknownEffect {
		t.Fatalf("caller-state mutation hidden from density: %+v", q)
	}
}

func TestSelectorMutationProvenanceRespectsPointerIndirection(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       quality.Provenance
	}{
		{"promoted pointer", "local := Pointer{shared}; local.Count++", quality.Unknown},
		{"nested promotion", "local := Nested{Pointer{shared}}; local.Count++", quality.Unknown},
		{"promoted field address", "local := Pointer{shared}; alias := &local.Count; *alias = 1", quality.Unknown},
		{"explicit pointer field", "local := Pointer{shared}; local.State.Count++", quality.Unknown},
		{"type alias", "local := Alias{shared}; local.Count++", quality.Unknown},
		{"value copy", "local := Value{*shared}; local.Count++", quality.Local},
		{"pointer parameter", "shared.Count++", quality.External},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "package repro\ntype State struct { Count int }\ntype Pointer struct { *State }\ntype Nested struct { Pointer }\ntype Value struct { State }\ntype Alias = Pointer\nfunc Mutate(shared *State) { " + test.body + " }\n"
			root := qualitySourceModule(t, []byte(source))
			facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			f := findFunction(t, facts, "Mutate")
			if len(f.Mutations) != 1 || f.Mutations[0].Provenance != test.want {
				t.Fatalf("mutations = %+v, want %s", f.Mutations, test.want)
			}
		})
	}
}
