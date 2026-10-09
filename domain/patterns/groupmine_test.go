package patterns

import (
	"fmt"
	"testing"
)

// groupTestPdg builds a PDG with a linear op chain plus call nodes.
// All PDGs share the same op structure (so CCGraph groups them); the
// call sets differ (so PR-Miner finds rules within the group).
func groupTestPdg(calls []string, lineBase int) *Pdg {
	var nodes []PdgNode
	// Shared structural backbone: 5 op nodes.
	for i := 0; i < 5; i++ {
		nodes = append(nodes, PdgNode{Kind: Op, Line: lineBase + i, Detail: "add:int"})
	}
	// Call nodes appended after the backbone.
	for i, c := range calls {
		nodes = append(nodes, PdgNode{Kind: Call, Line: lineBase + 5 + i, CalleeID: c})
	}
	var edges []PdgEdge
	for i := 0; i+1 < len(nodes); i++ {
		edges = append(edges, PdgEdge{From: i, To: i + 1, Kind: Data})
	}
	return &Pdg{Nodes: nodes, Edges: edges}
}

func TestMineSimilarityGroupsFindsRule(t *testing.T) {
	// 12 similar functions: 11 call {validate, respond}, 1 calls
	// {validate} only. CCGraph should group the similar ones; PR-Miner
	// should find {validate} -> respond within a group.
	var facts []*FuncFacts
	for i := 0; i < 11; i++ {
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("handler%d", i),
			Name: fmt.Sprintf("handler%d", i),
			Pdg:  groupTestPdg([]string{"validate", "respond"}, i*10),
		})
	}
	facts = append(facts, &FuncFacts{
		ID:   "handlerBad",
		Name: "handlerBad",
		Pdg:  groupTestPdg([]string{"validate"}, 110),
	})

	results := MineSimilarityGroups(facts)
	if len(results) == 0 {
		t.Fatalf("expected at least one similarity group, got none")
	}

	// Find a result with the validate->respond rule.
	found := false
	for _, r := range results {
		for _, rule := range r.Rules {
			if len(rule.Antecedent) == 1 && rule.Antecedent[0] == "validate" &&
				rule.Consequent == "respond" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected rule {validate} -> respond in group mining results")
	}
}

func TestMineSimilarityGroupsSkipsSmall(t *testing.T) {
	// 5 functions: below the 10-function minimum, should be skipped.
	var facts []*FuncFacts
	for i := 0; i < 5; i++ {
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("fn%d", i),
			Name: fmt.Sprintf("fn%d", i),
			Pdg:  groupTestPdg([]string{"a", "b"}, i*10),
		})
	}
	results := MineSimilarityGroups(facts)
	for _, r := range results {
		if len(r.GroupIDs) < minGroupMineSize {
			t.Errorf("group of %d functions should have been skipped", len(r.GroupIDs))
		}
	}
}

func TestMineSimilarityGroupsNoRules(t *testing.T) {
	// 12 similar functions with identical call sets: PR-Miner will find
	// trivial 100% rules ({a}->b, {b}->a) but with zero violations.
	// The pipeline should run without error.
	var facts []*FuncFacts
	for i := 0; i < 12; i++ {
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("fn%d", i),
			Name: fmt.Sprintf("fn%d", i),
			Pdg:  groupTestPdg([]string{"a", "b"}, i*10),
		})
	}
	// Should not panic; results may or may not contain groups.
	_ = MineSimilarityGroups(facts)
}
