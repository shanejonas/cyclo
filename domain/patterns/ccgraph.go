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
// skipped for that pair.
func CCGraphClones(pdgs map[string]*Pdg, names map[string]string) [][]string {
	return ccGraphGroups(pdgs, names, ccMatchThreshold)
}

// ccGraphGroups runs the shared CCGraph pipeline with a parameterized
// Stage-4 WL similarity threshold. The paper-exact path passes
// ccMatchThreshold.
func ccGraphGroups(pdgs map[string]*Pdg, names map[string]string, matchMilli uint32) [][]string {
	ids := ccSortableIDs(pdgs)
	if len(ids) < 2 {
		return nil
	}
	// Stages 1+2: cheap filters produce the candidate pair set.
	// vecs and alignedNames are indexed by position in ids, so the
	// O(n^2) pair loop uses slice indexing instead of map lookups.
	vecs := make([][]float64, len(ids))
	alignedNames := make([]string, len(ids))
	for i, id := range ids {
		vecs[i] = characteristicVector(pdgs[id])
		alignedNames[i] = names[id]
	}
	candidates := ccCandidatePairs(ids, vecs, alignedNames)
	if len(candidates) == 0 {
		return nil
	}
	// Stage 3: LSH-cluster the survivors' WL vectors.
	wls := ccBuildWls(candidates, pdgs)
	clusters := ccLSHClusters(candidates, wls)
	// Stage 4: WL similarity within LSH clusters.
	parent := ccMakeParent(ids)
	for _, cluster := range clusters {
		ccUnionSimilar(cluster, candidates, wls, parent, matchMilli)
	}
	return groupsOfTwoOrMore(parent)
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

// ccPairKey builds a canonical key for an unordered function pair.
func ccPairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "\x00" + b
}

// ccPairJob is one unordered function pair to check.
type ccPairJob struct{ a, b string }

// ccCandidatePairs returns the set of pairs passing Stages 1+2,
// keyed by ccPairKey. vecs and names are aligned with ids by position.
// Pair checks are striped across a worker pool; the output is a set,
// so worker assignment cannot affect the result.
func ccCandidatePairs(ids []string, vecs [][]float64, names []string) map[string]bool {
	workers := runtime.NumCPU()
	found := make(chan map[string]bool, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			found <- ccFilterStripes(ids, vecs, names, w, workers)
		}(w)
	}
	go func() {
		wg.Wait()
		close(found)
	}()
	out := map[string]bool{}
	for m := range found {
		for k := range m {
			out[k] = true
		}
	}
	return out
}

// ccFilterStripes applies Stages 1+2 to the pairs whose outer index i
// satisfies i%workers == worker, returning the surviving pair keys.
// Slice indexing (no map lookups) keeps the inner loop tight.
func ccFilterStripes(ids []string, vecs [][]float64, names []string, worker, workers int) map[string]bool {
	out := map[string]bool{}
	for i := worker; i < len(ids); i += workers {
		vi, ni := vecs[i], names[i]
		for j := i + 1; j < len(ids); j++ {
			// Stage 1: characteristic-vector cosine pre-filter.
			if cosineSimilarity(vi, vecs[j]) < charVecThreshold {
				continue
			}
			// Stage 2: Jaro-Winkler name filter.
			if !ccNamesSimilar(ni, names[j]) {
				continue
			}
			out[ccPairKey(ids[i], ids[j])] = true
		}
	}
	return out
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
// candidate pair.
func ccBuildWls(candidates map[string]bool, pdgs map[string]*Pdg) map[string]*Wl {
	ids := map[string]bool{}
	for key := range candidates {
		// Keys are "a\x00b"; split them back out.
		for i := 0; i < len(key); i++ {
			if key[i] == 0 {
				ids[key[:i]] = true
				ids[key[i+1:]] = true
				break
			}
		}
	}
	out := make(map[string]*Wl, len(ids))
	for id := range ids {
		out[id] = NewWl(pdgs[id])
	}
	return out
}

// ccLSHClusters vectorizes the survivors' WL instances and LSH-clusters
// them into candidate groups.
func ccLSHClusters(candidates map[string]bool, wls map[string]*Wl) [][]string {
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
func ccUnionSimilar(cluster []string, candidates map[string]bool, wls map[string]*Wl, parent map[string]string, matchMilli uint32) {
	workers := runtime.NumCPU()
	found := make(chan []ccPairJob, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			found <- ccSimilarStripes(cluster, candidates, wls, w, workers, matchMilli)
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
// worker) passing the candidate-set and WL-similarity filters.
func ccSimilarStripes(cluster []string, candidates map[string]bool, wls map[string]*Wl, worker, workers int, matchMilli uint32) []ccPairJob {
	var out []ccPairJob
	for i := worker; i < len(cluster); i += workers {
		for j := i + 1; j < len(cluster); j++ {
			a, b := cluster[i], cluster[j]
			if !candidates[ccPairKey(a, b)] {
				continue
			}
			if SimilarityMilli(wls[a], wls[b]) < matchMilli {
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
	l1, l2 := len(r1), len(r2)
	if l1 == 0 && l2 == 0 {
		return 1.0
	}
	if l1 == 0 || l2 == 0 {
		return 0.0
	}
	matches, transpositions := jaroMatches(r1, r2)
	if matches == 0 {
		return 0.0
	}
	m := float64(matches)
	t := float64(transpositions) / 2.0
	return (m/float64(l1) + m/float64(l2) + (m-t)/m) / 3.0
}

// jaroMatches counts matching runes and transpositions within the
// Jaro match window.
func jaroMatches(r1, r2 []rune) (matches, transpositions int) {
	l1, l2 := len(r1), len(r2)
	window := jaroWindow(l1, l2)
	seen1 := make([]bool, l1)
	seen2 := make([]bool, l2)
	matches = jaroFindMatches(r1, r2, window, seen1, seen2)
	if matches == 0 {
		return 0, 0
	}
	transpositions = jaroTranspositions(r1, r2, seen1, seen2)
	return matches, transpositions
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
	r1, r2 := []rune(s1), []rune(s2)
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

// ccGraphInputs builds the PDG and name maps CCGraphClones needs from facts.
// Functions without a PDG are skipped.
func ccGraphInputs(facts []*FuncFacts) (map[string]*Pdg, map[string]string) {
	pdgs := make(map[string]*Pdg, len(facts))
	names := make(map[string]string, len(facts))
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		pdgs[f.ID] = f.Pdg
		names[f.ID] = f.Name
	}
	return pdgs, names
}
