package gopatterns

import (
	"context"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// TestMinerSmoke runs the full patterns pipeline on the shapes fixture:
// extractor -> FuncFacts -> report.Run -> candidates.
func TestMinerSmoke(t *testing.T) {
	ctx := context.Background()
	ex, err := Extract(ctx, "testdata/shapes", []string{"."})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	pdgs := ex.Funcs
	if len(pdgs) == 0 {
		t.Fatal("no PDGs extracted")
	}
	t.Logf("extracted %d PDGs", len(pdgs))

	// Convert to FuncFacts with SigKey derived from Param TyClasses.
	var facts []*patterns.FuncFacts
	for _, fp := range pdgs {
		var params []string
		for _, n := range fp.Pdg.Nodes {
			if n.Kind == patterns.Param {
				params = append(params, n.TyClass)
			}
		}
		facts = append(facts, &patterns.FuncFacts{
			ID:     "test/shapes." + fp.Name,
			Name:   fp.Name,
			Path:   fp.Path,
			Line:   fp.Line,
			Pdg:    &fp.Pdg,
			SigKey: "fn(" + strings.Join(params, ",") + ")",
		})
	}

	report := patterns.Run(facts, patterns.Options{})
	text := patterns.Text(&report)
	t.Logf("report:\n%s", text)
	if len(report.Candidates) == 0 {
		t.Fatal("expected candidates from parallel pairs, got none")
	}
	// The CreateUser/CreateOrder pair should produce a candidate.
	found := false
	for _, c := range report.Candidates {
		for _, s := range c.Sites {
			if strings.Contains(s.Name, "CreateUser") || strings.Contains(s.Name, "CreateOrder") {
				found = true
			}
		}
		t.Logf("candidate: %s score=%d sites=%d", c.Kind, c.ScoreMilli, len(c.Sites))
	}
	if !found {
		t.Fatal("expected candidate involving CreateUser/CreateOrder")
	}
}
