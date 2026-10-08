package patterns

import (
	"fmt"
	"sort"
	"strings"
)

// SemanticClone detection following Komondoor & Horwitz (SAS 2001):
// find isomorphic PDG subgraphs instead of matching text. This catches
// non-contiguous, reordered, and intertwined clones that token-based
// detectors miss. The fragments are meaningful computations — ideal
// extract-function candidates.
//
// Full subgraph isomorphism is NP-hard; we use WL-hash bucketing:
// enumerate bounded connected subgraphs, hash canonically, bucket by hash.
// Subgraphs with the same hash are isomorphic (with high probability).

// minCloneNodes and maxCloneNodes bound the subgraph enumeration.
// Smaller = more candidates but noisier; larger = fewer but more meaningful.
const (
	minCloneNodes = 3
	maxCloneNodes = 8
)

// Subgraph is a connected induced subgraph of a PDG.
type Subgraph struct {
	// FuncID identifies the source function.
	FuncID string
	// Nodes are the PDG node indices in this subgraph.
	Nodes []int
	// Hash is the canonical WL-style hash of the subgraph.
	Hash string
	// Lines are the source lines covered (for display).
	Lines []int
}

// FindSemanticClones enumerates connected PDG subgraphs across functions
// and groups isomorphic ones. Returns groups of 2+ subgraphs from different
// functions.
func FindSemanticClones(facts []*FuncFacts) [][]Subgraph {
	all := collectSubgraphs(facts)
	buckets := bucketByHash(all)
	return filterMultiFunction(buckets)
}

// collectSubgraphs enumerates connected subgraphs for all functions.
func collectSubgraphs(facts []*FuncFacts) []Subgraph {
	var all []Subgraph
	for _, f := range facts {
		if f.Pdg == nil {
			continue
		}
		all = append(all, enumerateSubgraphs(f.ID, f.Pdg)...)
	}
	return all
}

// bucketByHash groups subgraphs by their canonical hash.
func bucketByHash(all []Subgraph) map[string][]Subgraph {
	buckets := map[string][]Subgraph{}
	for _, s := range all {
		buckets[s.Hash] = append(buckets[s.Hash], s)
	}
	return buckets
}

// filterMultiFunction keeps buckets with 2+ subgraphs from different functions.
func filterMultiFunction(buckets map[string][]Subgraph) [][]Subgraph {
	var clones [][]Subgraph
	for _, bucket := range buckets {
		if len(bucket) < 2 {
			continue
		}
		if countFuncs(bucket) >= 2 {
			clones = append(clones, bucket)
		}
	}
	return clones
}

// countFuncs counts distinct function IDs in a bucket.
func countFuncs(bucket []Subgraph) int {
	funcs := map[string]bool{}
	for _, s := range bucket {
		funcs[s.FuncID] = true
	}
	return len(funcs)
}

// enumerateSubgraphs finds all connected induced subgraphs of size
// [minCloneNodes, maxCloneNodes] via BFS from each node.
func enumerateSubgraphs(funcID string, pdg *Pdg) []Subgraph {
	var out []Subgraph
	seen := map[string]bool{} // dedupe by hash
	n := len(pdg.Nodes)
	for start := 0; start < n; start++ {
		out = append(out, bfsSubgraphs(funcID, pdg, start, seen)...)
	}
	return out
}

// bfsSubgraphs grows connected subgraphs from a start node via BFS.
func bfsSubgraphs(funcID string, pdg *Pdg, start int, seen map[string]bool) []Subgraph {
	var out []Subgraph
	queue := []work{{nodes: []int{start}}}
	visited := map[string]bool{}
	for len(queue) > 0 {
		w := queue[0]
		queue = queue[1:]
		if sg, ok := maybeBuildSubgraph(funcID, pdg, w.nodes, seen); ok {
			out = append(out, sg)
		}
		if len(w.nodes) >= maxCloneNodes {
			continue
		}
		queue = append(queue, expandWork(pdg, w.nodes, visited)...)
	}
	return out
}

type work struct {
	nodes []int
}

// maybeBuildSubgraph builds the subgraph if its size is in range and its
// hash hasn't been seen. Returns false if skipped.
func maybeBuildSubgraph(funcID string, pdg *Pdg, nodes []int, seen map[string]bool) (Subgraph, bool) {
	if len(nodes) < minCloneNodes || len(nodes) > maxCloneNodes {
		return Subgraph{}, false
	}
	sg := buildSubgraph(funcID, pdg, nodes)
	if seen[sg.Hash] {
		return Subgraph{}, false
	}
	seen[sg.Hash] = true
	return sg, true
}

// expandWork returns new work items by adding one frontier node each.
func expandWork(pdg *Pdg, nodes []int, visited map[string]bool) []work {
	var out []work
	for _, nb := range frontierNodes(pdg, nodes) {
		newNodes := append(append([]int{}, nodes...), nb)
		key := nodesKey(newNodes)
		if visited[key] {
			continue
		}
		visited[key] = true
		out = append(out, work{nodes: newNodes})
	}
	return out
}

// frontierNodes returns PDG node indices adjacent to the set but not in it.
func frontierNodes(pdg *Pdg, nodes []int) []int {
	inSet := map[int]bool{}
	for _, n := range nodes {
		inSet[n] = true
	}
	var out []int
	seen := map[int]bool{}
	for _, e := range pdg.Edges {
		out = appendFrontier(out, seen, inSet, e.From, e.To)
		out = appendFrontier(out, seen, inSet, e.To, e.From)
	}
	return out
}

// appendFrontier adds `to` to out if `from` is in the set and `to` is not.
// Returns the updated slice.
func appendFrontier(out []int, seen, inSet map[int]bool, from, to int) []int {
	if inSet[from] && !inSet[to] && !seen[to] {
		seen[to] = true
		return append(out, to)
	}
	return out
}

// nodesKey is a canonical string for a node set (sorted).
func nodesKey(nodes []int) string {
	sorted := append([]int{}, nodes...)
	sort.Ints(sorted)
	var sb strings.Builder
	for _, n := range sorted {
		fmt.Fprintf(&sb, "%d,", n)
	}
	return sb.String()
}

// buildSubgraph constructs the Subgraph with its canonical hash.
func buildSubgraph(funcID string, pdg *Pdg, nodes []int) Subgraph {
	// Canonical hash: sorted node labels + sorted edge descriptors.
	// Node label: Kind + TyClass + SigClass (not line numbers — those differ
	// across clones).
	var nodeLabels []string
	nodeSet := map[int]bool{}
	for _, n := range nodes {
		nodeSet[n] = true
		pn := pdg.Nodes[n]
		nodeLabels = append(nodeLabels, fmt.Sprintf("%s|%s|%s", pn.Kind, pn.TyClass, pn.SigClass))
	}
	sort.Strings(nodeLabels)
	var edgeDescs []string
	for _, e := range pdg.Edges {
		if nodeSet[e.From] && nodeSet[e.To] {
			// Use positions within the sorted node list for canonical form.
			// For simplicity, use the labels of the endpoints.
			from := pdg.Nodes[e.From]
			to := pdg.Nodes[e.To]
			edgeDescs = append(edgeDescs, fmt.Sprintf("%s-%s:%s", from.Kind, to.Kind, e.Kind))
		}
	}
	sort.Strings(edgeDescs)
	hash := strings.Join(nodeLabels, ";") + "#" + strings.Join(edgeDescs, ";")
	// Collect lines for display.
	var lines []int
	for _, n := range nodes {
		lines = append(lines, pdg.Nodes[n].Line)
	}
	sort.Ints(lines)
	return Subgraph{
		FuncID: funcID,
		Nodes:  append([]int{}, nodes...),
		Hash:   hash,
		Lines:  lines,
	}
}

// SemanticCloneCandidates converts clone groups to pattern candidates.
func SemanticCloneCandidates(groups [][]Subgraph, facts []*FuncFacts) []Candidate {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, group := range groups {
		var sites []Site
		for _, sg := range group {
			f := factByID[sg.FuncID]
			if f == nil {
				continue
			}
			sites = append(sites, Site{
				Path: f.Path,
				Line: sg.Lines[0],
				Name: f.Name,
			})
		}
		if len(sites) < 2 {
			continue
		}
		out = append(out, Candidate{
			Kind:             SemanticClone,
			ScoreMilli:       700, // Fixed: semantic clones are high-value.
			Observation:      fmt.Sprintf("%d functions share an isomorphic PDG subgraph (%d nodes)", len(sites), len(group[0].Nodes)),
			Inference:        "the same computation is written out once per site (possibly reordered)",
			PossibleRefactor: "extract one helper; use anti-unification to derive the parameters",
			Sites:            sites,
		})
	}
	return out
}
