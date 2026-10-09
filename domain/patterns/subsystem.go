package patterns

import (
	"fmt"
	"path"
)

// Subsystem detection for scoped PR-Miner mining.
//
// Whole-repo mining dilutes patterns: idioms live at the subsystem level
// (a package or a tightly-coupled group of functions), not across a diverse
// codebase. These helpers find subsystems via connected components of the
// call graph, so mining runs per-subsystem where patterns are dense.

// minSubsystemSize is the minimum functions for meaningful mining. Below
// this, 10% support is too few functions to be statistically significant.
const minSubsystemSize = 50

// maxSubsystemSize caps subsystem size. Larger components are split by
// package directory, since giant components dilute mining patterns.
const maxSubsystemSize = 1000

// BuildCallGraph extracts caller->callee edges from PDG Call nodes.
// Keys are FuncFacts IDs. Every fact appears as a key (possibly with no
// outgoing edges); callees not in facts also appear as keys so the graph
// is complete for component detection.
func BuildCallGraph(facts []*FuncFacts) map[string][]string {
	graph := make(map[string][]string, len(facts))
	for _, f := range facts {
		graph[f.ID] = nil
	}
	for _, f := range facts {
		graph[f.ID] = callTargets(f)
	}
	return graph
}

// callTargets returns the deduplicated CalleeIDs from a function's PDG.
func callTargets(f *FuncFacts) []string {
	if f.Pdg == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, n := range f.Pdg.Nodes {
		if n.Kind != Call || n.CalleeID == "" || seen[n.CalleeID] {
			continue
		}
		seen[n.CalleeID] = true
		out = append(out, n.CalleeID)
	}
	return out
}

// FindSubsystems groups functions into subsystems via connected components
// of the undirected call graph. Components smaller than minSubsystemSize
// are dropped; components larger than maxSubsystemSize are split by package
// directory. Returns function ID slices.
func FindSubsystems(facts []*FuncFacts) [][]string {
	graph := BuildCallGraph(facts)
	adj := undirectedAdj(graph)
	var out [][]string
	for _, comp := range connectedComponents(adj) {
		switch {
		case len(comp) < minSubsystemSize:
			continue
		case len(comp) > maxSubsystemSize:
			out = append(out, splitByPackage(facts, comp)...)
		default:
			out = append(out, comp)
		}
	}
	return out
}

// undirectedAdj symmetrizes a directed call graph for component detection.
// If A calls B, they belong to the same subsystem regardless of direction.
func undirectedAdj(graph map[string][]string) map[string]map[string]bool {
	adj := make(map[string]map[string]bool, len(graph))
	for id := range graph {
		adj[id] = map[string]bool{}
	}
	for caller, callees := range graph {
		for _, callee := range callees {
			adj[caller][callee] = true
			if _, ok := adj[callee]; !ok {
				adj[callee] = map[string]bool{}
			}
			adj[callee][caller] = true
		}
	}
	return adj
}

// connectedComponents finds connected components via breadth-first search.
// Runs in O(V+E).
func connectedComponents(adj map[string]map[string]bool) [][]string {
	visited := map[string]bool{}
	var out [][]string
	for id := range adj {
		if visited[id] {
			continue
		}
		out = append(out, bfsComponent(adj, id, visited))
	}
	return out
}

// bfsComponent collects one connected component starting from start.
func bfsComponent(adj map[string]map[string]bool, start string, visited map[string]bool) []string {
	var comp []string
	queue := []string{start}
	visited[start] = true
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		comp = append(comp, id)
		for nb := range adj[id] {
			if !visited[nb] {
				visited[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	return comp
}

// splitByPackage divides a large component by source directory, dropping
// groups below minSubsystemSize. Package directories are the natural
// fallback boundary when call-graph components grow too large.
func splitByPackage(facts []*FuncFacts, comp []string) [][]string {
	byID := make(map[string]*FuncFacts, len(facts))
	for _, f := range facts {
		byID[f.ID] = f
	}
	groups := map[string][]string{}
	for _, id := range comp {
		pkg := ""
		if f := byID[id]; f != nil {
			pkg = path.Dir(f.Path)
		}
		groups[pkg] = append(groups[pkg], id)
	}
	var out [][]string
	for _, g := range groups {
		if len(g) >= minSubsystemSize {
			out = append(out, g)
		}
	}
	return out
}

// minedRuleSubsystemCandidates mines PR-Miner rules per call-graph
// subsystem instead of across the whole repo. Each subsystem's candidates
// carry subsystem context in the inference text.
func minedRuleSubsystemCandidates(prepared []*FuncFacts) []Candidate {
	byID := make(map[string]*FuncFacts, len(prepared))
	for _, f := range prepared {
		byID[f.ID] = f
	}
	var out []Candidate
	for _, sub := range FindSubsystems(prepared) {
		facts := make([]*FuncFacts, 0, len(sub))
		for _, id := range sub {
			if f := byID[id]; f != nil {
				facts = append(facts, f)
			}
		}
		sets := BuildCallSets(facts)
		rules := MineRules(sets)
		violations := FindViolations(sets, rules)
		out = append(out, subsystemCandidates(violations, facts, len(sub))...)
	}
	return out
}

// subsystemCandidates converts violations to candidates with subsystem
// context: the inference names the subsystem size so the reader knows the
// pattern's scope.
func subsystemCandidates(violations []RuleViolation, facts []*FuncFacts, subsystemSize int) []Candidate {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, v := range violations {
		f := factByID[v.FuncID]
		if f == nil {
			continue
		}
		ant := ""
		for i, a := range v.Rule.Antecedent {
			if i > 0 {
				ant += ", "
			}
			ant += "`" + a + "`"
		}
		out = append(out, Candidate{
			Kind:       MinedRule,
			ScoreMilli: 750, // High: mined rules are repo-specific.
			Observation: fmt.Sprintf("calls %s but not `%s`",
				ant, v.Rule.Consequent),
			Inference: fmt.Sprintf(
				"in subsystem (%d functions): %.0f%% of functions that call %s also call `%s`",
				subsystemSize, v.Rule.Confidence*100, ant, v.Rule.Consequent,
			),
			PossibleRefactor: fmt.Sprintf("add the missing `%s` call", v.Rule.Consequent),
			Sites: []Site{
				{Path: f.Path, Line: f.Line, Name: f.Name},
			},
		})
	}
	return out
}
