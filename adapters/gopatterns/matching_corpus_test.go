package gopatterns

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// TestWriteMatchingCorpus freezes matching inputs for before/after runs.
// It is an opt-in test artifact, not a graph export feature.
func TestWriteMatchingCorpus(t *testing.T) {
	root, output := os.Getenv("CYCLO_MATCH_ROOT"), os.Getenv("CYCLO_MATCH_CORPUS")
	if root == "" || output == "" {
		t.Skip("opt-in matching corpus")
	}
	extraction, err := Extract(context.Background(), root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	facts := make([]*patterns.MiningFacts, 0, len(extraction.Funcs))
	for _, f := range extraction.Funcs {
		facts = append(facts, &patterns.MiningFacts{ID: f.Name, Name: f.Name, Path: f.Path, Line: f.Line, EndLine: f.EndLine, Pdg: patterns.MiningView(&f.Pdg), AstTypes: f.AstTypes, SigKey: SigKeyOf(f), SelfTy: f.SelfTy, Params: f.Params})
	}
	file, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(facts); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d matching inputs", len(facts))
}
