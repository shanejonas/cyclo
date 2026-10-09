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
	// 12 similar functions: 11 call {validate, respond} plus shared
	// setup calls, 1 calls {validate} plus setup but no respond.
	// CCGraph should group the similar ones; PR-Miner should find
	// {validate} -> respond within a group. The shared setup calls keep
	// call density above the boilerplate floor.
	setup := []string{"parse", "auth", "log", "metrics"}
	var facts []*FuncFacts
	for i := 0; i < 11; i++ {
		calls := append(append([]string{}, setup...), "validate", "respond")
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("handler%d", i),
			Name: fmt.Sprintf("handler%d", i),
			Pdg:  groupTestPdg(calls, i*10),
		})
	}
	badCalls := append(append([]string{}, setup...), "validate")
	facts = append(facts, &FuncFacts{
		ID:   "handlerBad",
		Name: "handlerBad",
		Pdg:  groupTestPdg(badCalls, 110),
	})

	results := MineSimilarityGroups(facts, ccMatchThreshold)
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
	results := MineSimilarityGroups(facts, ccMatchThreshold)
	for _, r := range results {
		if len(r.GroupIDs) < minGroupMineSize {
			t.Errorf("group of %d functions should have been skipped", len(r.GroupIDs))
		}
	}
}

func TestMineSimilarityGroupsNoRules(t *testing.T) {
	// 12 similar functions with identical 2-call sets: below the call
	// density floor, so the group is skipped as boilerplate. The pipeline
	// should run without error either way.
	var facts []*FuncFacts
	for i := 0; i < 12; i++ {
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("fn%d", i),
			Name: fmt.Sprintf("fn%d", i),
			Pdg:  groupTestPdg([]string{"a", "b"}, i*10),
		})
	}
	// Should not panic; results may or may not contain groups.
	_ = MineSimilarityGroups(facts, ccMatchThreshold)
}

func TestMineSimilarityGroupsCohortThreshold(t *testing.T) {
	// 12 similar functions with varied call sets. At the clone threshold
	// (900) they may or may not group; at the cohort threshold (650) the
	// looser match should find at least as many pairs. The pipeline must
	// run without error at both thresholds.
	var facts []*FuncFacts
	for i := 0; i < 12; i++ {
		calls := []string{"setup", "work", fmt.Sprintf("step%d", i%3)}
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("task%d", i),
			Name: fmt.Sprintf("task%d", i),
			Pdg:  groupTestPdg(calls, i*10),
		})
	}
	_ = MineSimilarityGroups(facts, ccMatchThreshold)
	results := MineSimilarityGroups(facts, defaultCohortThresholdMilli)
	// Cohort results must respect the minimum group size.
	for _, r := range results {
		if len(r.GroupIDs) < minGroupMineSize {
			t.Errorf("cohort of %d functions should have been skipped", len(r.GroupIDs))
		}
	}
}

func TestMeetsCallDensity(t *testing.T) {
	// Getters: 1 call each on average -> below the density floor.
	getters := []CallSet{
		{FuncID: "g1", Calls: map[string]bool{"a": true}},
		{FuncID: "g2", Calls: map[string]bool{"b": true}},
	}
	if meetsCallDensity(getters) {
		t.Errorf("getter call sets should not meet call density")
	}
	// Real handlers: 6 calls each on average -> meets the floor.
	handlers := []CallSet{
		{FuncID: "h1", Calls: map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true, "f": true}},
		{FuncID: "h2", Calls: map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true, "g": true}},
	}
	if !meetsCallDensity(handlers) {
		t.Errorf("handler call sets should meet call density")
	}
	if meetsCallDensity(nil) {
		t.Errorf("empty call sets should not meet call density")
	}
}

func TestMineSimilarityGroupsSkipsLowDensity(t *testing.T) {
	// 12 trivial getters: structurally similar (so CCGraph groups them)
	// but only 1 call each. The call density filter should skip the group,
	// producing no results even though the group meets the size minimum.
	var facts []*FuncFacts
	for i := 0; i < 12; i++ {
		facts = append(facts, &FuncFacts{
			ID:   fmt.Sprintf("getter%d", i),
			Name: fmt.Sprintf("getter%d", i),
			Pdg:  groupTestPdg([]string{"field"}, i*10),
		})
	}
	results := MineSimilarityGroups(facts, defaultCohortThresholdMilli)
	if len(results) != 0 {
		t.Errorf("expected low-density getter group to be skipped, got %d results", len(results))
	}
}

func TestGroupNoun(t *testing.T) {
	if got := groupNoun(ccMatchThreshold); got != "similarity group" {
		t.Errorf("clone threshold should say %q, got %q", "similarity group", got)
	}
	if got := groupNoun(defaultCohortThresholdMilli); got != "cohort" {
		t.Errorf("cohort threshold should say %q, got %q", "cohort", got)
	}
}
