package patterns

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// CCGraph-style clone detection (Zou et al., ASE 2020) with LSH scaling.
//
// The pipeline:
//  1. Characteristic vectors (cheap 7-dim) → candidate pairs.
//     CCGraph Stage 1: numerical PDG characteristics, cosine >= 0.9.
//  2. Jaro-Winkler function-name similarity >= 0.5 → still candidate.
//     CCGraph Stage 2: string similarity filter.
//  3. LSH on 512-dim WL vectors → candidate clusters.
//     Scaling layer (Gabel et al.): avoids O(n^2) pairwise WL checks.
//  4. WL graph kernel similarity >= 0.9 → clones.
//     CCGraph approximate matching. NO exact subgraph isomorphism.
//
// LSH is the optimization, CCGraph is the algorithm. Both are kept.

// Thresholds from the paper (Zou et al., ASE 2020).
const (
	// ccStage2NameThreshold is the minimum Jaro-Winkler similarity for
	// two function names to survive Stage 2 filtering.
	ccStage2NameThreshold = 0.5
	// ccMatchThreshold is the minimum WL kernel similarity (in
	// thousandths, matching SimilarityMilli) for a clone pair.
	ccMatchThreshold = 900
	// ccLSHTables and ccLSHHashes tune the LSH candidate generation.
	// More tables = higher recall; fewer hashes = higher recall.
	ccLSHTables = 16
	ccLSHHashes = 4
	ccLSHSeed   = 42
)

// CCGraphClones finds clone groups using the CCGraph pipeline with LSH
// scaling: characteristic-vector pre-filter, Jaro-Winkler name filter,
// LSH candidate clustering, then WL kernel similarity. names maps
// function ID to function name; when a name is missing, Stage 2 is
// skipped for that pair. No AST data: the Stage 0 AST pre-filter is
// skipped.
func CCGraphClones(pdgs map[string]*Pdg, names map[string]string) [][]string {
	return ccGraphGroups(pdgs, names, nil, false)
}

// CCGraphClonesWithAST is CCGraphClones with the Stage 0 AST pre-filter
// enabled. astTypes maps function ID to its AST node-type multiset (see
// AstNodeMultiset); nil or empty disables the pre-filter, matching
// CCGraphClones exactly.
func CCGraphClonesWithAST(pdgs map[string]*Pdg, names map[string]string, astTypes map[string]map[string]int) [][]string {
	return ccGraphGroups(pdgs, names, astTypes, false)
}

// CCGraphClonesWithNeighborhoodWL is CCGraphClones with the neighborhood-augmented
// WL kernel replacing the standard WL kernel in Stage 4. All paper-exact
// thresholds are unchanged; only the similarity kernel differs.
func CCGraphClonesWithNeighborhoodWL(pdgs map[string]*Pdg, names map[string]string) [][]string {
	return ccGraphGroups(pdgs, names, nil, true)
}

// ccGraphGroups runs the shared CCGraph pipeline. The Stage 4 WL
// similarity threshold is the paper-exact ccMatchThreshold.
//
// Stage 0 (AST pre-filter) runs before the paper stages when astTypes
// is non-empty. It only ADDS candidate pairs with high AST similarity;
// the paper stages (1, 2, 4) keep their exact thresholds and order.
//
// When useNeighborhoodWL is true, Stage 4 uses the neighborhood WL kernel (SimilarityNeighborhoodWLMilli)
// instead of the standard WL kernel (SimilarityMilli).
func ccGraphGroups(pdgs map[string]*Pdg, names map[string]string, astTypes map[string]map[string]int, useNeighborhoodWL bool) [][]string {
	ids := ccSortableIDs(pdgs)
	if len(ids) < 2 {
		return nil
	}
	// Stage 0: AST pre-filter. High-similarity pairs bypass the
	// characteristic-vector filter; nothing is removed that the paper
	// stages would have kept.
	shapes := ccASTShapes(ids, astTypes)
	// Stages 1+2: cheap filters produce the candidate pair set.
	// vecs and alignedNames are indexed by position in ids, so the
	// O(n^2) pair loop uses slice indexing instead of map lookups.
	vecs := make([][]float64, len(ids))
	alignedNames := make([]string, len(ids))
	for i, id := range ids {
		vecs[i] = characteristicVector(pdgs[id])
		alignedNames[i] = names[id]
	}
	candidates := ccCandidatePairs(vecs, alignedNames, shapes)
	if len(candidates.members()) == 0 {
		return nil
	}
	// Stage 3: LSH-cluster the survivors' WL vectors.
	// Stage 4: WL similarity within LSH clusters. The kernel is selected
	// once here: standard WL or neighborhood-augmented WL, wrapped in a
	// closure so the worker functions stay kernel-agnostic.
	wls := ccBuildWls(ids, candidates, pdgs, vecs)
	clusters := ccLSHClusters(wls)
	sim := func(a, b string) uint32 { return SimilarityMilli(wls[a], wls[b]) }
	if useNeighborhoodWL {
		neighborhoodWLs := ccBuildNeighborhoodWLs(ids, candidates, pdgs)
		sim = func(a, b string) uint32 {
			return SimilarityNeighborhoodWLMilli(neighborhoodWLs[a], neighborhoodWLs[b])
		}
	}
	// idToIndex maps function IDs back to ids-slice positions for
	// candidate-set lookups in Stage 4.
	idToIndex := ccIDIndex(ids)
	parent := ccMakeParent(ids)
	for _, cluster := range clusters {
		ccUnionSimilar(cluster, candidates, idToIndex, sim, parent)
	}
	return groupsOfTwoOrMore(parent)
}

// ccIDIndex maps function IDs to positions in the sorted ids slice,
// for candidate-set lookups in Stage 4.
func ccIDIndex(ids []string) map[string]int {
	out := make(map[string]int, len(ids))
	for i, id := range ids {
		out[id] = i
	}
	return out
}

// ccSortableIDs returns function IDs with usable PDGs, sorted.
func ccSortableIDs(pdgs map[string]*Pdg) []string {
	var ids []string
	for id, pdg := range pdgs {
		if pdg != nil && len(pdg.Nodes) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// ccPairJob is one unordered function pair to check.
type ccPairJob struct{ a, b string }

// ccPairFilters contains immutable pair inputs shared by striped workers.
type ccPairFilters struct {
	vecs   [][]float64
	names  [][]rune
	shapes []astShape
}

func ccCandidatePairs(vecs [][]float64, names []string, shapes []astShape) *ccPairs {
	out := newCCPairs(len(vecs))
	filters := ccPairFilters{vecs, ccNameRunes(names), shapes}
	workers := min(runtime.NumCPU(), len(vecs))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			filters.stripes(out, w, workers)
		}(w)
	}
	wg.Wait()
	return out
}

func (f ccPairFilters) stripes(out *ccPairs, worker, workers int) {
	matcher := newNameMatcher(f.names)
	for i := worker; i < len(f.vecs); i += workers {
		for j := i + 1; j < len(f.vecs); j++ {
			if f.accepts(i, j, matcher) {
				out.add(i, j)
			}
		}
	}
}

func (f ccPairFilters) accepts(i, j int, matcher nameMatcher) bool {
	if f.shapes[i].bypass(f.shapes[j]) {
		return true
	}
	if cosineSimilarity(f.vecs[i], f.vecs[j]) < charVecThreshold {
		return false
	}
	return matcher.similar(f.names[i], f.names[j])
}

// ccNameRunes converts each name once, outside the candidate-pair loop.
func ccNameRunes(names []string) [][]rune {
	out := make([][]rune, len(names))
	for i, name := range names {
		out[i] = []rune(name)
	}
	return out
}

// nameMatcher owns scratch buffers for one worker; workers never share them.
type nameMatcher struct{ seen1, seen2 []bool }

func newNameMatcher(names [][]rune) nameMatcher {
	size := 0
	for _, name := range names {
		size = max(size, len(name))
	}
	return nameMatcher{make([]bool, size), make([]bool, size)}
}

func (m nameMatcher) similar(a, b []rune) bool {
	if len(a) == 0 || len(b) == 0 {
		return true
	}
	seen1, seen2 := m.seen1[:len(a)], m.seen2[:len(b)]
	clear(seen1)
	clear(seen2)
	j := jaroRunes(a, b, seen1, seen2)
	prefix := commonRunePrefix(a, b, 4)
	return j+float64(prefix)*0.1*(1-j) >= ccStage2NameThreshold
}

// ccNamesSimilar reports whether two function names are similar enough.
// Missing names skip the filter (conservative: don't filter out).
func ccNamesSimilar(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	return jaroWinkler(a, b) >= ccStage2NameThreshold
}

// ccBuildWls builds WL instances for every function appearing in any
// candidate pair. ids maps candidate-pair indices back to function IDs;
// vecs holds the Stage 1 characteristic vectors (aligned with ids),
// reused here instead of recomputed. Uses NewWlLight: the CCGraph
// pipeline only needs histograms and node counts, so the refinement
// history and graph adjacency are not retained.
func ccBuildWls(ids []string, candidates *ccPairs, pdgs map[string]*Pdg, vecs [][]float64) map[string]*Wl {
	seen := candidates.members()
	out := make(map[string]*Wl, len(seen))
	for _, i := range seen {
		id := ids[i]
		out[id] = NewWlLight(pdgs[id], vecs[i])
	}
	return out
}

// ccBuildNeighborhoodWLs builds NeighborhoodWL instances for every function appearing in any
// candidate pair. Only called when the neighborhood WL kernel is selected.
// ids maps candidate-pair indices back to function IDs.
func ccBuildNeighborhoodWLs(ids []string, candidates *ccPairs, pdgs map[string]*Pdg) map[string]*NeighborhoodWL {
	seen := candidates.members()
	out := make(map[string]*NeighborhoodWL, len(seen))
	for _, i := range seen {
		id := ids[i]
		out[id] = NewNeighborhoodWL(pdgs[id])
	}
	return out
}

// ccLSHClusters vectorizes the survivors' WL instances and LSH-clusters
// them into candidate groups.
func ccLSHClusters(wls map[string]*Wl) [][]string {
	vectors := make(map[string][]float64, len(wls))
	for id, w := range wls {
		vectors[id] = Vectorize(w)
	}
	lsh := NewLSH(lshDim, ccLSHTables, ccLSHHashes, ccLSHSeed)
	return ClusterClones(vectors, lsh)
}

// ccUnionSimilar unions pairs within each LSH cluster that pass all
// filters including WL similarity. The WL kernel checks are striped
// across a worker pool; unions are applied sequentially afterwards.
// Union order cannot affect the final groups (groupsOfTwoOrMore
// normalizes via find), so the output matches the sequential version.
// idToIndex maps function IDs to positions in the sorted ids slice,
// for candidate-set lookups.
func ccUnionSimilar(cluster []string, candidates *ccPairs, idToIndex map[string]int, sim func(a, b string) uint32, parent map[string]string) {
	workers := runtime.NumCPU()
	found := make(chan []ccPairJob, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			found <- ccSimilarStripes(cluster, candidates, idToIndex, sim, w, workers)
		}(w)
	}
	go func() {
		wg.Wait()
		close(found)
	}()
	for pairs := range found {
		for _, p := range pairs {
			union(parent, p.a, p.b)
		}
	}
}

// ccSimilarStripes returns the cluster pairs (outer index striped by
// worker) passing the candidate-set and WL-similarity filters. The sim
// closure is the Stage 4 kernel: standard WL or neighborhood-augmented WL,
// selected by the caller. idToIndex maps function IDs to positions in the
// sorted ids slice, for candidate-set lookups.
func ccSimilarStripes(cluster []string, candidates *ccPairs, idToIndex map[string]int, sim func(a, b string) uint32, worker, workers int) []ccPairJob {
	var out []ccPairJob
	for i := worker; i < len(cluster); i += workers {
		for j := i + 1; j < len(cluster); j++ {
			a, b := cluster[i], cluster[j]
			if !candidates.has(idToIndex[a], idToIndex[b]) {
				continue
			}
			if sim(a, b) < ccMatchThreshold {
				continue
			}
			out = append(out, ccPairJob{a: a, b: b})
		}
	}
	return out
}

// ccMakeParent initializes union-find parent pointers.
func ccMakeParent(ids []string) map[string]string {
	parent := make(map[string]string, len(ids))
	for _, id := range ids {
		parent[id] = id
	}
	return parent
}

// jaroWinkler returns the Jaro-Winkler string similarity in [0, 1].
// Identical strings score 1.0; completely different strings score 0.0.
func jaroWinkler(s1, s2 string) float64 {
	j := jaro(s1, s2)
	prefix := commonPrefixLen(s1, s2, 4)
	return j + float64(prefix)*0.1*(1.0-j)
}

// jaro returns the Jaro similarity in [0, 1].
func jaro(s1, s2 string) float64 {
	r1, r2 := []rune(s1), []rune(s2)
	return jaroRunes(r1, r2, make([]bool, len(r1)), make([]bool, len(r2)))
}

func jaroRunes(r1, r2 []rune, seen1, seen2 []bool) float64 {
	l1, l2 := len(r1), len(r2)
	if l1 == 0 && l2 == 0 {
		return 1
	}
	if l1 == 0 || l2 == 0 {
		return 0
	}
	matches := jaroFindMatches(r1, r2, jaroWindow(l1, l2), seen1, seen2)
	if matches == 0 {
		return 0
	}
	t := float64(jaroTranspositions(r1, r2, seen1, seen2)) / 2
	m := float64(matches)
	return (m/float64(l1) + m/float64(l2) + (m-t)/m) / 3
}

// jaroFindMatches marks matched runes in seen1/seen2 and returns the count.
func jaroFindMatches(r1, r2 []rune, window int, seen1, seen2 []bool) int {
	matches := 0
	for i := 0; i < len(r1); i++ {
		lo, hi := jaroBounds(i, window, len(r2))
		for j := lo; j < hi; j++ {
			if !seen2[j] && r1[i] == r2[j] {
				seen1[i] = true
				seen2[j] = true
				matches++
				break
			}
		}
	}
	return matches
}

// jaroTranspositions counts position mismatches among matched runes.
func jaroTranspositions(r1, r2 []rune, seen1, seen2 []bool) int {
	out := 0
	k := 0
	for i := 0; i < len(r1); i++ {
		if !seen1[i] {
			continue
		}
		for !seen2[k] {
			k++
		}
		if r1[i] != r2[k] {
			out++
		}
		k++
	}
	return out
}

// jaroWindow returns the match window: floor(max(l1,l2)/2) - 1, min 0.
func jaroWindow(l1, l2 int) int {
	m := l1
	if l2 > m {
		m = l2
	}
	w := m/2 - 1
	if w < 0 {
		w = 0
	}
	return w
}

// jaroBounds returns the [lo, hi) search range in r2 for r1[i].
func jaroBounds(i, window, l2 int) (int, int) {
	lo := i - window
	if lo < 0 {
		lo = 0
	}
	hi := i + window + 1
	if hi > l2 {
		hi = l2
	}
	return lo, hi
}

// commonPrefixLen returns the length of the common prefix, capped at max.
func commonPrefixLen(s1, s2 string, max int) int {
	return commonRunePrefix([]rune(s1), []rune(s2), max)
}

func commonRunePrefix(r1, r2 []rune, max int) int {
	n := len(r1)
	if len(r2) < n {
		n = len(r2)
	}
	if n > max {
		n = max
	}
	out := 0
	for i := 0; i < n; i++ {
		if r1[i] != r2[i] {
			break
		}
		out++
	}
	return out
}

// buildFactMap indexes facts by ID for candidate construction.
func buildFactMap(facts []*FuncFacts) map[string]*FuncFacts {
	out := map[string]*FuncFacts{}
	for _, f := range facts {
		out[f.ID] = f
	}
	return out
}

// CCGraphCloneCandidates converts CCGraph clone groups (function ID lists)
// into pattern candidates for the report pipeline.
func CCGraphCloneCandidates(groups [][]string, facts []*FuncFacts) []Candidate {
	factByID := buildFactMap(facts)
	var out []Candidate
	for _, group := range groups {
		if c, ok := makeCCGraphCandidate(group, factByID); ok {
			out = append(out, c)
		}
	}
	return out
}

// makeCCGraphCandidate builds one candidate from a clone group. Groups with
// fewer than two resolvable sites are dropped.
func makeCCGraphCandidate(group []string, factByID map[string]*FuncFacts) (Candidate, bool) {
	var sites []Site
	for _, id := range group {
		f := factByID[id]
		if f == nil {
			continue
		}
		sites = append(sites, Site{
			Path: f.Path,
			Line: f.Line,
			Name: f.Name,
		})
	}
	if len(sites) < 2 {
		return Candidate{}, false
	}
	return Candidate{
		Kind:             CCGraphClone,
		ScoreMilli:       750,
		Observation:      ccObservation(sites),
		Inference:        "these functions have similar PDGs and similar names: they likely implement the same logic",
		PossibleRefactor: "review the group; extract a shared helper if the logic is truly duplicated",
		Sites:            sites,
	}, true
}

// ccObservation describes a CCGraph clone group for the report.
func ccObservation(sites []Site) string {
	names := make([]string, 0, len(sites))
	for _, s := range sites {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return fmt.Sprintf("CCGraph found %d similar functions: %s", len(sites), strings.Join(names, ", "))
}

// ccGraphInputs builds the PDG, name, and AST-type maps CCGraphClones
// needs from facts. Functions without a PDG are skipped.
func ccGraphInputs(facts []*FuncFacts) (map[string]*Pdg, map[string]string, map[string]map[string]int) {
	pdgs := make(map[string]*Pdg, len(facts))
	names := make(map[string]string, len(facts))
	astTypes := make(map[string]map[string]int, len(facts))
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		pdgs[f.ID] = f.Pdg
		names[f.ID] = f.Name
		if len(f.AstTypes) > 0 {
			astTypes[f.ID] = f.AstTypes
		}
	}
	return pdgs, names, astTypes
}
