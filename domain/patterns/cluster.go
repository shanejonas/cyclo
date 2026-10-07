// Cluster groups similar functions around a template (medoid) instead of
// chaining pairs, so dissimilar bodies cannot be linked through a middleman
// (no transitive union-find). Ported from rstyle's cluster.rs.
package patterns

import (
	"sort"
	"strconv"
)

// Blocks larger than this are skipped: their key is too common to say anything.
const blockCap = 1000

// How many of a function's rarest call classes are used as blocking keys.
const blockKeysCount = 2

// Corpora up to this size compare every pair: blocking on the rarest call
// classes picks classes unique to each function when the corpus is tiny, so
// parallel functions never meet.
const allPairsLimit = 400

// Params tunes clustering. It mirrors rstyle's cluster::Params, including the
// two experiment fields that cluster itself does not consume: SingleCallGuard
// and MaxHoles are read downstream by the candidates stage, exactly as in
// Rust where cluster.rs only carries them.
type Params struct {
	ThresholdMilli   uint32
	MinCoverageMilli uint32
	// PairCoverageMilli is the experiment that also selects pairs whose
	// alignment coverage reaches this, besides WL similarity. Nil disables it.
	PairCoverageMilli *uint32
	// Normalize holds the canonicalization rules applied to every graph
	// before hashing and aligning (default: none).
	Normalize Rules
	// SingleCallGuard is the experiment where bodies with one call count as
	// substantial unless that call only forwards parameters.
	SingleCallGuard bool
	// MaxHoles caps how many differing parts a parameterize helper may take.
	MaxHoles int
}

// DefaultParams mirrors Rust's Default: 600/600 thresholds, no pair-coverage
// experiment, no normalization, no single-call guard, at most 3 holes.
func DefaultParams() Params {
	return Params{
		ThresholdMilli:   600,
		MinCoverageMilli: 600,
		Normalize:        Rules{},
		MaxHoles:         3,
	}
}

// Column is one hole variable across all members: Values[k] belongs to
// Cluster.Members[k].
type Column struct {
	Kind   HoleKind
	Var    string
	Values []string
}

// Cluster is a group of structurally parallel functions.
type Cluster struct {
	// Members are indices into the input slice; the template comes first,
	// the rest ascending.
	Members []int
	// CoverageMilli aligns with Members: the template scores 1000.
	CoverageMilli []uint32
	Columns       []Column
}

// classesOf returns the sorted set of call classes in a graph.
func classesOf(pdg *Pdg) []string {
	seen := map[string]bool{}
	for _, n := range pdg.Nodes {
		if n.Kind == Call {
			seen[CallClass(n)] = true
		}
	}
	classes := make([]string, 0, len(seen))
	for c := range seen {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	return classes
}

// blockKeys returns the rarest call classes (across the corpus) of every
// function: at most blockKeysCount, rarest first, ties broken by class name.
func blockKeys(pdgs []*Pdg) [][]string {
	freq := map[string]int{}
	sets := make([][]string, len(pdgs))
	for i, p := range pdgs {
		sets[i] = classesOf(p)
		for _, c := range sets[i] {
			freq[c]++
		}
	}
	keys := make([][]string, len(pdgs))
	for i, set := range sets {
		ranked := append([]string{}, set...)
		sort.Slice(ranked, func(a, b int) bool {
			if freq[ranked[a]] != freq[ranked[b]] {
				return freq[ranked[a]] < freq[ranked[b]]
			}
			return ranked[a] < ranked[b]
		})
		if len(ranked) > blockKeysCount {
			ranked = ranked[:blockKeysCount]
		}
		keys[i] = ranked
	}
	return keys
}

// indexBlocks maps each blocking key to the functions carrying it.
func indexBlocks(keys [][]string) map[string][]int {
	blocks := map[string][]int{}
	for i, list := range keys {
		for _, key := range list {
			blocks[key] = append(blocks[key], i)
		}
	}
	return blocks
}

// blockPairs emits every pair inside one block, deduplicated across blocks.
func blockPairs(members []int, seen map[[2]int]bool, pairs [][2]int) [][2]int {
	for i, a := range members {
		for _, b := range members[i+1:] {
			p := [2]int{a, b}
			if !seen[p] {
				seen[p] = true
				pairs = append(pairs, p)
			}
		}
	}
	return pairs
}

// candidatePairs returns the pairs that share a blocking key, skipping blocks
// larger than blockCap, sorted and deduplicated.
func candidatePairs(keys [][]string) [][2]int {
	seen := map[[2]int]bool{}
	var pairs [][2]int
	for _, members := range indexBlocks(keys) {
		if len(members) > blockCap {
			continue
		}
		pairs = blockPairs(members, seen, pairs)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	return pairs
}

// pairsToCompare returns every pair for small corpora, and the pairs that
// share a blocking key otherwise.
func pairsToCompare(pdgs []*Pdg) [][2]int {
	if len(pdgs) > allPairsLimit {
		return candidatePairs(blockKeys(pdgs))
	}
	var pairs [][2]int
	for a := 0; a < len(pdgs); a++ {
		for b := a + 1; b < len(pdgs); b++ {
			pairs = append(pairs, [2]int{a, b})
		}
	}
	return pairs
}

// pairSelected reports whether a comparable pair reaches the WL similarity
// threshold, or (experiment) the pair-coverage floor. CCGraph two-stage
// filtering: characteristic vector cosine similarity is checked before the
// expensive WL kernel.
func pairSelected(pdgs []*Pdg, wls []*Wl, a, b int, params Params) bool {
	if !Comparable(wls[a], wls[b]) {
		return false
	}
	// Stage 2: skip pairs with dissimilar characteristic vectors
	if !charVecSimilar(wls[a], wls[b]) {
		return false
	}
	// A pair is selected if EITHER the flat kernel OR the control-weighted
	// kernel reaches the threshold. The weighted kernel ranks "same logic,
	// different data" higher, finding more real clones; the flat kernel
	// preserves pairs the weighting might miss. Union ensures no lost pairs.
	flatSim := SimilarityMilli(wls[a], wls[b])
	weightedSim := SimilarityWeighted(wls[a], wls[b])
	if flatSim >= params.ThresholdMilli || weightedSim >= params.ThresholdMilli {
		return true
	}
	if params.PairCoverageMilli != nil {
		coverage := Align(pdgs[a], wls[a], pdgs[b], wls[b]).CoverageMilli
		return coverage >= *params.PairCoverageMilli
	}
	return false
}

// buildAdjacency links the pairs the threshold selects.
func buildAdjacency(pdgs []*Pdg, wls []*Wl, pairs [][2]int, params Params) map[int]map[int]bool {
	adjacency := map[int]map[int]bool{}
	for _, p := range pairs {
		a, b := p[0], p[1]
		if pairSelected(pdgs, wls, a, b, params) {
			if adjacency[a] == nil {
				adjacency[a] = map[int]bool{}
			}
			if adjacency[b] == nil {
				adjacency[b] = map[int]bool{}
			}
			adjacency[a][b] = true
			adjacency[b][a] = true
		}
	}
	return adjacency
}

// freeDegree counts a node's neighbours that are still unassigned.
func freeDegree(adjacency map[int]map[int]bool, free map[int]bool, v int) int {
	degree := 0
	for n := range adjacency[v] {
		if free[n] {
			degree++
		}
	}
	return degree
}

// betterSeed picks the higher degree, breaking ties toward the lower index.
func betterSeed(degree, v, best, seed int) bool {
	if degree <= 0 {
		return false
	}
	if degree != best {
		return degree > best
	}
	return v < seed
}

// nextSeed returns the unassigned function with the most unassigned
// neighbours (lowest index on ties), or false when none has any.
func nextSeed(adjacency map[int]map[int]bool, free map[int]bool) (int, bool) {
	seed, best := 0, -1
	for v := range free {
		if degree := freeDegree(adjacency, free, v); betterSeed(degree, v, best, seed) {
			seed, best = v, degree
		}
	}
	return seed, best > 0
}

// totalSimilarity sums a candidate's WL similarity to the rest of the group.
func totalSimilarity(wls []*Wl, group []int, v int) uint64 {
	var total uint64
	for _, w := range group {
		if w != v {
			total += uint64(SimilarityMilli(wls[v], wls[w]))
		}
	}
	return total
}

// medoid returns the group member with the highest total similarity to the
// others (lowest index on ties).
func medoid(wls []*Wl, group []int) int {
	best := group[0]
	bestTotal := totalSimilarity(wls, group, best)
	for _, v := range group[1:] {
		if total := totalSimilarity(wls, group, v); total > bestTotal || total == bestTotal && v < best {
			best, bestTotal = v, total
		}
	}
	return best
}

// joinedMember is a non-template member aligned against the template.
type joinedMember struct {
	index     int
	alignment Alignment
}

func joinMember(pdgs []*Pdg, wls []*Wl, template, member int) joinedMember {
	return joinedMember{
		index:     member,
		alignment: Align(pdgs[template], wls[template], pdgs[member], wls[member]),
	}
}

// columnKey identifies a substitution column: the hole kind plus the
// template-side value.
type columnKey struct {
	kind     HoleKind
	template string
}

type tabSlot struct {
	// site is the first template node index where the substitution occurs.
	site int
	// found[k] is the value joined member k has there, or nil when its
	// alignment has no such hole (falls back to the template value).
	found []*string
}

const maxInt = int(^uint(0) >> 1)

// tabulate builds the template value -> (first template site, the value each
// joined member has there) table.
func tabulate(joined []joinedMember) map[columnKey]*tabSlot {
	table := map[columnKey]*tabSlot{}
	for k, jm := range joined {
		for _, h := range jm.alignment.Holes {
			key := columnKey{kind: h.Kind, template: h.A}
			slot, ok := table[key]
			if !ok {
				slot = &tabSlot{site: maxInt, found: make([]*string, len(joined))}
				table[key] = slot
			}
			if h.Sites[0][0] < slot.site {
				slot.site = h.Sites[0][0]
			}
			if slot.found[k] == nil {
				b := h.B
				slot.found[k] = &b
			}
		}
	}
	return table
}

// holeKindRank reproduces Rust's HoleKind declaration order (Type, Method,
// FreeFn, Literal, Field, Op) for column ordering, independent of the Go
// constant values the align port chooses.
var holeKindRank = map[HoleKind]int{
	HoleType:    0,
	HoleMethod:  1,
	HoleFreeFn:  2,
	HoleLiteral: 3,
	HoleField:   4,
	HoleOp:      5,
}

// holeVarName is the variable name of the index-th hole of a kind: T0, M1...
// It mirrors Rust's Hole::var_name over HoleKind::prefix.
var holeKindPrefix = map[HoleKind]string{
	HoleType:    "T",
	HoleMethod:  "M",
	HoleFreeFn:  "F",
	HoleLiteral: "L",
	HoleField:   "D",
	HoleOp:      "O",
}

func holeVarName(kind HoleKind, index int) string {
	prefix, ok := holeKindPrefix[kind]
	if !ok {
		prefix = "X"
	}
	return prefix + strconv.Itoa(index)
}

type columnRow struct {
	kind     HoleKind
	template string
	site     int
	found    []*string
}

// sortedRows orders the substitution table by first template site, then hole
// kind (declaration order), then template value.
func sortedRows(table map[columnKey]*tabSlot) []columnRow {
	rows := make([]columnRow, 0, len(table))
	for key, slot := range table {
		rows = append(rows, columnRow{kind: key.kind, template: key.template, site: slot.site, found: slot.found})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].site != rows[j].site {
			return rows[i].site < rows[j].site
		}
		if holeKindRank[rows[i].kind] != holeKindRank[rows[j].kind] {
			return holeKindRank[rows[i].kind] < holeKindRank[rows[j].kind]
		}
		return rows[i].template < rows[j].template
	})
	return rows
}

// columnValues lists the template value first, then each joined member's,
// falling back to the template's when a member's alignment lacks the hole.
func columnValues(template string, found []*string) []string {
	values := make([]string, 0, len(found)+1)
	values = append(values, template)
	for _, f := range found {
		if f != nil {
			values = append(values, *f)
		} else {
			values = append(values, template)
		}
	}
	return values
}

// columnsOf renders the substitution table as columns, numbering each kind's
// variables (T0, T1, ...) in row order.
func columnsOf(joined []joinedMember) []Column {
	rows := sortedRows(tabulate(joined))
	counts := map[HoleKind]int{}
	columns := make([]Column, 0, len(rows))
	for _, r := range rows {
		n := counts[r.kind]
		counts[r.kind] = n + 1
		columns = append(columns, Column{
			Kind:   r.kind,
			Var:    holeVarName(r.kind, n),
			Values: columnValues(r.template, r.found),
		})
	}
	return columns
}

// buildCluster aligns every group member against the medoid template and
// keeps the ones reaching the coverage floor. It returns false when nothing
// joins, so the caller can retire just the seed.
// rankTemplates orders group members by total similarity, best first.
func rankTemplates(wls []*Wl, group []int) []int {
	ranked := append([]int{}, group...)
	sort.Slice(ranked, func(i, j int) bool {
		ti, tj := totalSimilarity(wls, group, ranked[i]), totalSimilarity(wls, group, ranked[j])
		if ti != tj {
			return ti > tj
		}
		return ranked[i] < ranked[j]
	})
	return ranked
}

// tryTemplate aligns group members against one template, keeping those
// reaching the coverage floor. It returns the joined members and their
// total coverage.
func tryTemplate(pdgs []*Pdg, wls []*Wl, group []int, template int, params Params) ([]joinedMember, uint64) {
	var joined []joinedMember
	var totalCov uint64
	for _, v := range group {
		if v == template {
			continue
		}
		jm := joinMember(pdgs, wls, template, v)
		if jm.alignment.CoverageMilli >= params.MinCoverageMilli {
			joined = append(joined, jm)
			totalCov += uint64(jm.alignment.CoverageMilli)
		}
	}
	return joined, totalCov
}

// makeCluster builds the Cluster from a template and its joined members.
func makeCluster(template int, joined []joinedMember) Cluster {
	members := make([]int, 0, len(joined)+1)
	members = append(members, template)
	coverage := make([]uint32, 0, len(joined)+1)
	coverage = append(coverage, 1000)
	for _, jm := range joined {
		members = append(members, jm.index)
		coverage = append(coverage, jm.alignment.CoverageMilli)
	}
	return Cluster{Members: members, CoverageMilli: coverage, Columns: columnsOf(joined)}
}

// bestTemplate tries each candidate template and returns the one yielding
// the most joined members (ties broken by total coverage).
func bestTemplate(pdgs []*Pdg, wls []*Wl, group []int, ranked []int, params Params) (Cluster, int) {
	var best Cluster
	bestCount := 0
	var bestCoverage uint64
	for _, template := range ranked {
		joined, totalCov := tryTemplate(pdgs, wls, group, template, params)
		if len(joined) > bestCount || len(joined) == bestCount && totalCov > bestCoverage {
			bestCount = len(joined)
			bestCoverage = totalCov
			best = makeCluster(template, joined)
		}
	}
	return best, bestCount
}

// buildCluster aligns every group member against the template and keeps the
// ones reaching the coverage floor. It tries the top candidates by total
// similarity as templates (not just the medoid), because the similarity-best
// template is not always the alignment-best: with the weighted kernel's
// coarser control projection, a central member may score high on similarity
// while a peripheral pair aligns better with each other. It returns false
// when nothing joins, so the caller can retire just the seed.
func buildCluster(pdgs []*Pdg, wls []*Wl, group []int, params Params) (Cluster, bool) {
	ranked := rankTemplates(wls, group)
	tries := 3
	if len(ranked) < tries {
		tries = len(ranked)
	}
	best, count := bestTemplate(pdgs, wls, group, ranked[:tries], params)
	if count == 0 {
		return Cluster{}, false
	}
	return best, true
}

// seedGroup is the seed plus its still-unassigned neighbours, ascending.
func seedGroup(adjacency map[int]map[int]bool, free map[int]bool, seed int) []int {
	set := map[int]bool{seed: true}
	for v := range adjacency[seed] {
		if free[v] {
			set[v] = true
		}
	}
	group := make([]int, 0, len(set))
	for v := range set {
		group = append(group, v)
	}
	sort.Ints(group)
	return group
}

// prepare canonicalizes every graph and hashes it, mirroring Rust's
// canonicalize-then-Wl::new prologue. With a non-nil cache, WL refinements
// for unchanged functions are reused instead of recomputed.
func prepare(pdgs []*Pdg, params Params, cache *WlCache) ([]*Pdg, []*Wl) {
	canonical := make([]*Pdg, len(pdgs))
	for i, p := range pdgs {
		canonical[i] = Canonicalize(p, params.Normalize)
	}
	wls := make([]*Wl, len(canonical))
	for i, p := range canonical {
		if w, ok := cache.Get(p); ok {
			wls[i] = w
			continue
		}
		wls[i] = NewWl(p)
		cache.Put(p, wls[i])
	}
	return canonical, wls
}

// ClusterPdgs finds clusters of structurally parallel functions, largest
// first. Each function lands in at most one cluster; members must align with
// the template above the coverage floor.
func ClusterPdgs(pdgs []*Pdg, params Params) []Cluster {
	return ClusterPdgsCached(pdgs, params, nil)
}

// ClusterPdgsCached is ClusterPdgs with an optional WL cache for incremental
// runs. A nil cache computes every refinement fresh.
func ClusterPdgsCached(pdgs []*Pdg, params Params, cache *WlCache) []Cluster {
	canonical, wls := prepare(pdgs, params, cache)
	adjacency := buildAdjacency(canonical, wls, pairsToCompare(canonical), params)
	free := map[int]bool{}
	for v := range adjacency {
		free[v] = true
	}
	var found []Cluster
	for {
		seed, ok := nextSeed(adjacency, free)
		if !ok {
			break
		}
		cluster, ok := buildCluster(canonical, wls, seedGroup(adjacency, free, seed), params)
		if ok {
			for _, m := range cluster.Members {
				delete(free, m)
			}
			found = append(found, cluster)
		} else {
			delete(free, seed)
		}
	}
	return found
}
