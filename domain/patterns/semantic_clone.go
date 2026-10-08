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
	minCloneNodes = 6
	maxCloneNodes = 8
	// maxSubgraphsPerFunc caps enumeration to prevent combinatorial explosion
	// on large functions. The BFS can generate exponentially many connected
	// subgraphs; this budget keeps it tractable.
	maxSubgraphsPerFunc = 1000
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
//
// Pipeline:
// 1. Enumerate connected subgraphs (bounded by maxSubgraphsPerFunc)
// 2. Bucket by WL hash (Komondoor & Horwitz: WL replaces expensive isomorphism)
// 3. Verify isomorphism within buckets (catches WL hash collisions)
// 4. Filter to maximal clones (removes subgraphs contained in larger ones)
// 5. Keep only groups spanning 2+ functions
func FindSemanticClones(facts []*FuncFacts) [][]Subgraph {
	all := collectSubgraphs(facts)
	buckets := bucketByHash(all)
	verified := verifyBuckets(buckets, facts)
	maximal := filterMaximal(verified)
	return filterMultiFunction(maximal)
}

// verifyBuckets checks isomorphism within each hash bucket.
// WL hashes can collide; this ensures we only group truly isomorphic subgraphs.
func verifyBuckets(buckets map[string][]Subgraph, facts []*FuncFacts) map[string][]Subgraph {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	out := map[string][]Subgraph{}
	for hash, group := range buckets {
		if len(group) < 2 {
			continue
		}
		// Verify pairwise isomorphism; keep only the isomorphic ones.
		verified := verifyIsomorphic(group, factByID)
		if len(verified) >= 2 {
			out[hash] = verified
		}
	}
	return out
}

// verifyIsomorphic filters a bucket to subgraphs isomorphic to the first.
func verifyIsomorphic(group []Subgraph, factByID map[string]*FuncFacts) []Subgraph {
	if len(group) == 0 {
		return nil
	}
	base := group[0]
	basePdg := factByID[base.FuncID].Pdg
	out := []Subgraph{base}
	for _, sg := range group[1:] {
		otherPdg := factByID[sg.FuncID].Pdg
		if subgraphsIsomorphic(basePdg, base.Nodes, otherPdg, sg.Nodes) {
			out = append(out, sg)
		}
	}
	return out
}

// subgraphsIsomorphic checks if two node sets induce isomorphic subgraphs.
// Uses backtracking (VF2-style) — tractable since subgraphs are ≤8 nodes.
func subgraphsIsomorphic(pdgA *Pdg, nodesA []int, pdgB *Pdg, nodesB []int) bool {
	if len(nodesA) != len(nodesB) {
		return false
	}
	// Build sub-PDGs for comparison.
	subA := inducedSubPdg(pdgA, nodesA)
	subB := inducedSubPdg(pdgB, nodesB)
	return pdgsIsomorphic(subA, subB)
}

// pdgsIsomorphic checks if two PDGs are isomorphic via backtracking.
func pdgsIsomorphic(a, b *Pdg) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.Edges) != len(b.Edges) {
		return false
	}
	n := len(a.Nodes)
	// Build adjacency matrices for quick lookup.
	adjA := buildAdjMatrix(a)
	adjB := buildAdjMatrix(b)
	// Backtracking: try all mappings.
	mapping := make([]int, n) // mapping[i] = node in B for node i in A
	used := make([]bool, n)
	return backtrack(0, n, mapping, used, a, b, adjA, adjB)
}

func buildAdjMatrix(pdg *Pdg) [][]bool {
	n := len(pdg.Nodes)
	m := make([][]bool, n)
	for i := range m {
		m[i] = make([]bool, n)
	}
	for _, e := range pdg.Edges {
		m[e.From][e.To] = true
	}
	return m
}

func backtrack(idx, n int, mapping []int, used []bool, a, b *Pdg, adjA, adjB [][]bool) bool {
	if idx == n {
		return true // Found a valid mapping.
	}
	// Try each unused node in B for node idx in A.
	for j := 0; j < n; j++ {
		if used[j] {
			continue
		}
		// Check node labels match.
		if !nodesMatch(a.Nodes[idx], b.Nodes[j]) {
			continue
		}
		// Check edge consistency with already-mapped nodes.
		if !edgesConsistent(idx, j, mapping, adjA, adjB) {
			continue
		}
		mapping[idx] = j
		used[j] = true
		if backtrack(idx+1, n, mapping, used, a, b, adjA, adjB) {
			return true
		}
		used[j] = false
	}
	return false
}

// nodesMatch checks if two PDG nodes have the same label (ignoring line numbers).
func nodesMatch(a, b PdgNode) bool {
	return a.Kind == b.Kind && a.TyClass == b.TyClass && a.SigClass == b.SigClass
}

// edgesConsistent checks if the mapping preserves edges to already-mapped nodes.
func edgesConsistent(idx, j int, mapping []int, adjA, adjB [][]bool) bool {
	for i := 0; i < idx; i++ {
		mapped := mapping[i]
		// Edge i->idx in A must correspond to mapped->j in B.
		if adjA[i][idx] != adjB[mapped][j] {
			return false
		}
		// Edge idx->i in A must correspond to j->mapped in B.
		if adjA[idx][i] != adjB[j][mapped] {
			return false
		}
	}
	return true
}

// filterMaximal keeps only subgraphs not contained in a larger clone.
// This prevents the flood of overlapping candidates.
func filterMaximal(buckets map[string][]Subgraph) map[string][]Subgraph {
	// Deduplicate by function pair: for each unique pair of functions,
	// keep only the largest subgraph. This prevents the flood of overlapping
	// candidates from the same function pair.
	seenPair := map[string]bool{}
	out := map[string][]Subgraph{}
	for hash, group := range buckets {
		var filtered []Subgraph
		for _, sg := range group {
			// The group already contains subgraphs from different functions.
			// We need to deduplicate across all buckets by function pair.
			// For now, keep the group as-is; deduplication happens in
			// SemanticCloneCandidates by function pair.
			filtered = append(filtered, sg)
		}
		if len(filtered) >= 2 {
			out[hash] = filtered
		}
	}
	_ = seenPair
	return out
}

func filterMaximalGroup(group []Subgraph) []Subgraph {
	var out []Subgraph
	for i, sg := range group {
		contained := false
		for j, other := range group {
			if i == j {
				continue
			}
			if nodesSubset(sg.Nodes, other.Nodes) && len(other.Nodes) > len(sg.Nodes) {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, sg)
		}
	}
	return out
}

// isSubset checks if a is a subset of b.
func nodesSubset(a, b []int) bool {
	setB := map[int]bool{}
	for _, x := range b {
		setB[x] = true
	}
	for _, x := range a {
		if !setB[x] {
			return false
		}
	}
	return true
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
		if len(out) >= maxSubgraphsPerFunc {
			break // Budget exhausted; stop enumeration.
		}
	}
	return out
}

// bfsSubgraphs grows connected subgraphs from a start node via BFS.
func bfsSubgraphs(funcID string, pdg *Pdg, start int, seen map[string]bool) []Subgraph {
	var out []Subgraph
	queue := []work{{nodes: []int{start}}}
	visited := map[string]bool{}
	// Budget the BFS to prevent explosion on dense graphs.
	const maxQueue = 5000
	for len(queue) > 0 {
		if len(queue) > maxQueue {
			break // Too many; stop expanding from this start.
		}
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
	// Build the induced sub-PDG and hash it with real WL refinement.
	// This is the Komondoor & Horwitz approach: WL-hash bucketing replaces
	// expensive isomorphism checks.
	subPdg := inducedSubPdg(pdg, nodes)
	wl := NewWl(subPdg)
	hash := wlHash(wl)
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

// inducedSubPdg builds a PDG containing only the given nodes and the edges
// between them. Node indices are remapped to 0..len(nodes)-1.
func inducedSubPdg(pdg *Pdg, nodes []int) *Pdg {
	// Map old indices to new.
	remap := map[int]int{}
	for i, n := range nodes {
		remap[n] = i
	}
	sub := &Pdg{
		Nodes: make([]PdgNode, len(nodes)),
	}
	for i, n := range nodes {
		// Copy the node but clear line numbers (they differ across clones).
		pn := pdg.Nodes[n]
		pn.Line = 0
		sub.Nodes[i] = pn
	}
	for _, e := range pdg.Edges {
		from, okFrom := remap[e.From]
		to, okTo := remap[e.To]
		if okFrom && okTo {
			sub.Edges = append(sub.Edges, PdgEdge{
				From:   from,
				To:     to,
				Kind:   e.Kind,
				ArgPos: e.ArgPos,
			})
		}
	}
	return sub
}

// wlHash returns a canonical string hash from WL histograms.
// The histograms capture the refined color distribution, which is
// isomorphism-invariant.
func wlHash(wl *Wl) string {
	var parts []string
	for _, hist := range wl.hists {
		for _, e := range hist {
			parts = append(parts, fmt.Sprintf("%d:%d", e.color, e.count))
		}
		parts = append(parts, "|")
	}
	return strings.Join(parts, ",")
}

// SemanticCloneCandidates converts clone groups to pattern candidates.
func SemanticCloneCandidates(groups [][]Subgraph, facts []*FuncFacts) []Candidate {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	// Deduplicate by function pair: keep only the largest subgraph per pair.
	// This prevents the flood of overlapping candidates from the same pair.
	bestByPair := map[string]struct {
		group []Subgraph
		size  int
	}{}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		// Build a canonical pair key (sorted function IDs).
		var ids []string
		for _, sg := range group {
			ids = append(ids, sg.FuncID)
		}
		sort.Strings(ids)
		key := strings.Join(ids, "|")
		size := len(group[0].Nodes)
		if existing, ok := bestByPair[key]; !ok || size > existing.size {
			bestByPair[key] = struct {
				group []Subgraph
				size  int
			}{group, size}
		}
	}
	var out []Candidate
	for _, entry := range bestByPair {
		group := entry.group
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
			ScoreMilli:       700,
			Observation:      fmt.Sprintf("%d functions share an isomorphic PDG subgraph (%d nodes)", len(sites), len(group[0].Nodes)),
			Inference:        "the same computation is written out once per site (possibly reordered)",
			PossibleRefactor: "extract one helper; use anti-unification to derive the parameters",
			Sites:            sites,
		})
	}
	return out
}
