package patterns

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"
)

// Weisfeiler-Lehman similarity kernel, ported from rstyle's wl.rs (via the
// Phase 0 spike, where FNV-1a was verified byte-identical against rstyle's
// golden test values). Labels deliberately exclude names — only kinds and
// signature/type classes — so Dog::bark ≡ Cat::meow.

const (
	fnvOffset64   uint64 = 14695981039346656037
	fnvPrime64    uint64 = 1099511628211
	inSalt        uint64 = 0x1f
	outSalt       uint64 = 0x2e
	maxLevels            = 4
	minNodes             = 4
	minCalls             = 1
	maxScale             = 2
	diameterLimit        = 400
)

func fnv1a(data []byte) uint64 {
	h := fnvOffset64
	for _, b := range data {
		h ^= uint64(b)
		h *= fnvPrime64
	}
	return h
}

// combine hashes the 16 LE bytes of (acc, value); order-sensitive.
func combine(acc, value uint64) uint64 {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], acc)
	binary.LittleEndian.PutUint64(buf[8:16], value)
	return fnv1a(buf[:])
}

// label is the node label text. Names (callee ids, fields, literal values)
// are deliberately absent — only kinds and signature/type classes — so
// Dog::bark ≡ Cat::meow. CCGraph-style refinement: calls carry their package
// scope (std vs local), so same-shape code calling stdlib vs local helpers
// no longer collides. Data type classes stay erased: type differences must
// surface as alignment holes (generic_fn), not as label mismatches.
func label(n PdgNode) string {
	switch n.Kind {
	case Call:
		return "call:" + callScope(n.CalleeID) + ":" + n.SigClass
	case Lit:
		return "lit:" + n.LitKind
	case Op:
		class, _, _ := strings.Cut(n.Detail, ":")
		return "op:" + class
	case Param:
		return "param:" + n.TyClass
	default:
		return string(n.Kind)
	}
}

// callScope classifies a call's package scope from its FuncID callee id
// ("pkgpath.Name" or "pkgpath.Type.Method", "builtin.Name"): stdlib package
// paths contain no dot. The trailing name segment is stripped; if what
// remains ends in an (exported, uppercase) type name it was the
// pkgpath.Type.Method form, so that segment goes too. Only the class is
// labeled, never the callee name.
func callScope(calleeID string) string {
	if strings.HasPrefix(calleeID, "builtin.") {
		return "builtin"
	}
	rest := stripCalleeName(calleeID)
	if rest == "" {
		return "unknown"
	}
	if strings.Contains(rest, ".") {
		return "local"
	}
	return "std"
}

// stripCalleeName removes the trailing function/method name from a FuncID,
// plus a trailing type qualifier when the Method form is used.
func stripCalleeName(calleeID string) string {
	rest := calleeID
	if i := strings.LastIndex(rest, "."); i >= 0 {
		rest = rest[:i]
	} else {
		return ""
	}
	return stripTypeQualifier(rest)
}

// stripTypeQualifier removes a trailing ".Type" when Type looks like an
// exported Go type name (uppercase first letter): the pkgpath.Type.Method
// form as opposed to pkgpath.Name.
func stripTypeQualifier(rest string) string {
	i := strings.LastIndex(rest, ".")
	if i < 0 || i+1 >= len(rest) {
		return rest
	}
	if c := rest[i+1]; c >= 'A' && c <= 'Z' {
		return rest[:i]
	}
	return rest
}

func edgeTag(kind EdgeKind, argPos int) uint64 {
	var isCtrl uint64
	if kind == Ctrl {
		isCtrl = 1
	}
	if kind == Exec {
		isCtrl = 2
	}
	return combine(isCtrl, uint64(argPos))
}

// Nb is a neighbour reached over a tagged edge.
type Nb struct {
	node uint64
	tag  uint64
}

// Graph is the WL working graph: hashed labels plus tagged adjacency.
type Graph struct {
	labels []uint64
	inc    [][]Nb
	out    [][]Nb
}

func adjacency(n int, pdg *MiningGraph, forward, execution bool) [][]Nb {
	lists := make([][]Nb, n)
	for _, e := range pdg.Edges {
		if !wlEdgeAllowed(e, n, execution) {
			continue
		}
		at, node := e.From, uint64(e.To)
		if !forward {
			at, node = e.To, uint64(e.From)
		}
		lists[at] = append(lists[at], Nb{node: node, tag: edgeTag(e.Kind, e.ArgPos)})
	}
	return lists
}

func buildGraph(pdg *MiningGraph) Graph { return buildProfileGraph(pdg, false) }

func wlEdgeAllowed(e PdgEdge, n int, execution bool) bool {
	return e.From < n && e.To < n && (execution || e.Kind != Exec)
}

func buildProfileGraph(pdg *MiningGraph, execution bool) Graph {
	n := len(pdg.Nodes)
	labels := make([]uint64, n)
	for i, node := range pdg.Nodes {
		labels[i] = pdg.baseLabels.hash(node)
	}
	return Graph{labels: labels, inc: adjacency(n, pdg, false, execution), out: adjacency(n, pdg, true, execution)}
}

func digest(own, salt uint64, neighbours []Nb, prev []uint64) uint64 {
	parts := make([]uint64, len(neighbours))
	for i, nb := range neighbours {
		parts[i] = combine(nb.tag, prev[nb.node])
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i] < parts[j] })
	acc := combine(own, salt)
	for _, p := range parts {
		acc = combine(acc, p)
	}
	return acc
}

func (g *Graph) refine(prev []uint64) []uint64 {
	next := make([]uint64, len(prev))
	for v := range prev {
		inward := digest(prev[v], inSalt, g.inc[v], prev)
		next[v] = digest(inward, outSalt, g.out[v], prev)
	}
	return next
}

type histEntry struct {
	color uint64
	count int
}

func histogram(colors []uint64) []histEntry {
	counts := map[uint64]int{}
	for _, c := range colors {
		counts[c]++
	}
	h := make([]histEntry, 0, len(counts))
	for color, count := range counts {
		h = append(h, histEntry{color, count})
	}
	sort.Slice(h, func(i, j int) bool { return h[i].color < h[j].color })
	return h
}

// eccentricityWith reuses BFS storage across diameter's starting nodes.
func (g *Graph) eccentricityWith(start int, seen []bool, queue []int) int {
	clear(seen)
	seen[start] = true
	queue = append(queue[:0], start)
	depth, head := 0, 0
	for head < len(queue) {
		end := len(queue)
		for _, v := range queue[head:end] {
			queue = appendNeighbours(queue, g.inc[v], seen)
			queue = appendNeighbours(queue, g.out[v], seen)
		}
		if len(queue) == end {
			return depth
		}
		head = end
		depth++
	}
	return depth
}

func appendNeighbours(queue []int, nbs []Nb, seen []bool) []int {
	for _, nb := range nbs {
		if !seen[nb.node] {
			seen[nb.node] = true
			queue = append(queue, int(nb.node))
		}
	}
	return queue
}

func diameter(g *Graph) int {
	if len(g.labels) > diameterLimit {
		return maxLevels
	}
	seen := make([]bool, len(g.labels))
	queue := make([]int, 0, len(g.labels))
	max := 0
	for v := range g.labels {
		if e := g.eccentricityWith(v, seen, queue); e > max {
			max = e
		}
	}
	return max
}

// Wl holds refined colors and histograms of one graph.
type Wl struct {
	graph    Graph
	rounds   [][]uint64
	hists    [][]histEntry
	calls    int
	diameter int
	charVec  []float64
	pdg      *MiningGraph
	// nodeCount caches len(graph.labels). The CCGraph pipeline
	// (SimilarityMilli) only needs hists and the node
	// count, so NewWlLight omits graph/rounds/pdg and relies on
	// this field. Populated by every constructor.
	nodeCount int
	// projCache lazily holds WL histograms for single-edge-kind projections,
	// keyed by edge kind. Used by SimilarityWeighted; computed on demand so
	// functions that never reach pair comparison pay nothing.
	projCache map[EdgeKind][][]histEntry
}

// characteristicVector counts node kinds and edge kinds for CCGraph-style
// pre-filtering. Native views use all nine canonical source characteristics.
func characteristicVector(pdg *MiningGraph) []float64 {
	if pdg.Paper != nil {
		if !pdg.Paper.ReferenceKnown {
			return nil
		}
		return pdg.Paper.Counts[:]
	}
	return miningCharacteristics(pdg, false)
}

func miningCharacteristics(pdg *MiningGraph, execution bool) []float64 {
	vec := make([]float64, 9)
	countNodes(pdg, vec)
	countEdges(pdg, vec)
	if !execution {
		return vec[:7]
	}
	return vec
}

// countNodes tallies node kinds into the characteristic vector.
func countNodes(pdg *MiningGraph, vec []float64) {
	for _, n := range pdg.Nodes {
		vec[nodeKindIndex(n.Kind)]++
	}
}

// nodeKindIndex maps a node kind to its characteristic vector position.
func nodeKindIndex(kind NodeKind) int {
	switch kind {
	case Let:
		return 0
	case Op:
		return 1
	case Branch, Match, Loop, Iterate:
		return 2
	case Call:
		return 3
	default:
		return 4
	}
}

// countEdges tallies edge kinds into the characteristic vector.
func countEdges(pdg *MiningGraph, vec []float64) {
	for _, e := range pdg.Edges {
		if e.Kind == Ctrl {
			vec[5]++
		} else if e.Kind == Data {
			vec[6]++
		} else if e.Kind == Exec {
			vec[7]++
		}
	}
}

// cosineSimilarity returns the cosine of the angle between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// charVecSimilar reports whether two graphs are characteristically similar
// enough to warrant the full WL kernel. CCGraph Stage 2 filtering.
func charVecSimilar(a, b *Wl) bool {
	return cosineSimilarity(a.charVec, b.charVec) >= charVecThreshold
}

const charVecThreshold = 0.9

// NewWl builds the WL refinement of a PDG.
func NewWl(pdg *MiningGraph) *Wl {
	g := buildGraph(pdg)
	d := diameter(&g)
	// CCGraph optimization: only compute rounds up to diameter+1 (capped),
	// not the full maxLevels. Small-diameter graphs save refinement work.
	rounds := (&g).refineRoundsDiameter(d)
	return &Wl{
		graph:     g,
		rounds:    rounds,
		hists:     histograms(rounds),
		calls:     countCalls(pdg),
		diameter:  d,
		charVec:   miningCharacteristics(pdg, false),
		pdg:       pdg,
		nodeCount: len(g.labels),
	}
}

// NewWlLight builds a Wl with only the fields needed for CCGraph
// similarity (SimilarityMilli):
// hists, diameter, nodeCount, charVec, and calls. It omits graph,
// rounds, and pdg, which are only needed by PDG alignment (align.go),
// the WL disk cache (wlcache.go), and weighted similarity
// (SimilarityWeighted). The caller supplies the characteristic vector
// (already computed for Stage 1) instead of recomputing it.
//
// Significantly reduces retained memory when building WL instances for
// thousands of candidate functions: the refinement history (rounds)
// and adjacency (graph) dominate per-instance memory, and neither is
// read by the CCGraph pipeline.
func NewWlLight(pdg *MiningGraph, charVec []float64) *Wl {
	g := buildProfileGraph(pdg, true)
	d := diameter(&g)
	rounds := (&g).refineRoundsDiameter(d)
	return &Wl{
		hists:     histograms(rounds),
		calls:     countCalls(pdg),
		diameter:  d,
		charVec:   charVec,
		nodeCount: len(g.labels),
	}
}

func refineRounds(g *Graph) [][]uint64 {
	return g.refineRoundsDiameter(maxLevels - 1)
}

func (g *Graph) refineRoundsDiameter(d int) [][]uint64 {
	h := d + 1
	if h < 1 {
		h = 1
	}
	if h > maxLevels {
		h = maxLevels
	}
	rounds := [][]uint64{g.labels}
	for i := 1; i < h; i++ {
		rounds = append(rounds, g.refine(rounds[len(rounds)-1]))
	}
	return rounds
}

func histograms(rounds [][]uint64) [][]histEntry {
	hists := make([][]histEntry, len(rounds))
	for i, colors := range rounds {
		hists[i] = histogram(colors)
	}
	return hists
}

func countCalls(pdg *MiningGraph) int {
	calls := 0
	for _, n := range pdg.Nodes {
		if n.Kind == Call {
			calls++
		}
	}
	return calls
}

func (w *Wl) nodes() int { return w.nodeCount }

// levels caps refinement depth by the smaller graph's diameter.
func levels(a, b *Wl) int {
	h := a.diameter
	if b.diameter < h {
		h = b.diameter
	}
	h++
	if h < 1 {
		h = 1
	}
	if h > maxLevels {
		h = maxLevels
	}
	return h
}

// weights are (h, h-1, ..., 1): earlier levels count more.
func weights(h int) []uint64 {
	w := make([]uint64, h)
	for i := 1; i <= h; i++ {
		w[i-1] = uint64(h - i + 1)
	}
	return w
}

func intersection(a, b []histEntry) int {
	i, j, total := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i].color < b[j].color:
			i++
		case a[i].color > b[j].color:
			j++
		default:
			if a[i].count < b[j].count {
				total += a[i].count
			} else {
				total += b[j].count
			}
			i++
			j++
		}
	}
	return total
}

// SimilarityMilli is sum_i w_i |hist_i(A) ∩ hist_i(B)| / sum_i w_i max(|A|,|B|),
// in thousandths. rstyle clusters at >= 600.
func SimilarityMilli(a, b *Wl) uint32 {
	h := levels(a, b)
	denom := maxNodes(a, b)
	w := weights(h)
	num, den := weightedIntersection(a, b, w, denom, h)
	if den == 0 {
		return 0
	}
	return uint32(num * 1000 / den)
}

// similarityAtLeast answers only the threshold question. Each unvisited
// level contributes at most the smaller graph's node count, so an upper
// bound below the threshold safely stops the remaining intersections.
func similarityAtLeast(a, b *Wl, limit uint32) bool {
	h := levels(a, b)
	remaining := uint64(h * (h + 1) / 2)
	den := remaining * maxNodes(a, b)
	if den == 0 {
		return limit == 0
	}
	bound := uint64(min(a.nodes(), b.nodes()))
	var num uint64
	for i := 0; i < h; i++ {
		if (num+remaining*bound)*1000/den < uint64(limit) {
			return false
		}
		weight := uint64(h - i)
		num += weight * uint64(intersection(a.hists[i], b.hists[i]))
		remaining -= weight
	}
	return num*1000/den >= uint64(limit)
}

func maxNodes(a, b *Wl) uint64 {
	denom := uint64(a.nodes())
	if uint64(b.nodes()) > denom {
		denom = uint64(b.nodes())
	}
	return denom
}

func weightedIntersection(a, b *Wl, w []uint64, denom uint64, h int) (uint64, uint64) {
	var num, den uint64
	for i := 0; i < h; i++ {
		num += w[i] * uint64(intersection(a.hists[i], b.hists[i]))
		den += w[i] * denom
	}
	return num, den
}

// Projection weights for SimilarityWeighted: control edges count 3.0,
// data edges 1.0, so "same logic, different data" outranks "same data,
// different logic". The 3:1 ratio lets control dominate for parameterize
// candidates while data still breaks ties.
const (
	ctrlSimWeight = 12
	dataSimWeight = 4
)

// buildProjection builds the WL working graph over a single edge kind.
// For the control projection, labels are coarsened to node kinds only: the
// control similarity must capture the skeleton (branching/looping shape),
// not data distinctions like which function is called. The data projection
// keeps full labels.
func buildProjection(pdg *MiningGraph, kind EdgeKind) Graph {
	n := len(pdg.Nodes)
	labels := make([]uint64, n)
	for i, node := range pdg.Nodes {
		l := label(node)
		if kind == Ctrl {
			l = string(node.Kind)
		}
		labels[i] = fnv1a([]byte(l))
	}
	inc, out := projectionAdjacency(pdg, n, kind)
	return Graph{labels: labels, inc: inc, out: out}
}

// projectionAdjacency builds the in/out adjacency lists over one edge kind.
func projectionAdjacency(pdg *MiningGraph, n int, kind EdgeKind) (inc, out [][]Nb) {
	inc = make([][]Nb, n)
	out = make([][]Nb, n)
	for _, e := range pdg.Edges {
		if e.Kind != kind || e.From >= n || e.To >= n {
			continue
		}
		tag := edgeTag(e.Kind, e.ArgPos)
		out[e.From] = append(out[e.From], Nb{node: uint64(e.To), tag: tag})
		inc[e.To] = append(inc[e.To], Nb{node: uint64(e.From), tag: tag})
	}
	return inc, out
}

// projHists returns the WL histograms for one edge-kind projection, computed
// lazily on first use and cached. Full maxLevels rounds are kept so any pair
// level h can slice them.
func (w *Wl) projHists(kind EdgeKind) [][]histEntry {
	if w.projCache == nil {
		w.projCache = map[EdgeKind][][]histEntry{}
	}
	if h, ok := w.projCache[kind]; ok {
		return h
	}
	g := buildProjection(w.pdg, kind)
	rounds := refineRounds(&g)
	hists := histograms(rounds)
	w.projCache[kind] = hists
	return hists
}

// projSimilarity is the histogram-kernel similarity over one edge-kind
// projection, in thousandths.
func projSimilarity(a, b *Wl, kind EdgeKind) uint32 {
	h := levels(a, b)
	denom := maxNodes(a, b)
	w := weights(h)
	ah, bh := a.projHists(kind), b.projHists(kind)
	var num, den uint64
	for i := 0; i < h && i < len(ah) && i < len(bh); i++ {
		num += w[i] * uint64(intersection(ah[i], bh[i]))
		den += w[i] * denom
	}
	if den == 0 {
		return 0
	}
	return uint32(num * 1000 / den)
}

// SimilarityWeighted combines control-projection and data-projection WL
// similarities with control edges weighing 1.0 and data edges 0.6. Same
// 0-1000 scale as SimilarityMilli, so the 600 cluster threshold applies.
// When either graph lacks control edges, the control projection is
// degenerate (isolated nodes), so it falls back to the flat kernel.
// The data projection must reach a floor (500): the coarse control
// projection over-groups skeletons, so data similarity gates the pair.
func SimilarityWeighted(a, b *Wl) uint32 {
	if !hasControlEdges(a.pdg) || !hasControlEdges(b.pdg) {
		return SimilarityMilli(a, b)
	}
	ctrl := uint64(projSimilarity(a, b, Ctrl))
	data := uint64(projSimilarity(a, b, Data))
	return uint32((ctrlSimWeight*ctrl + dataSimWeight*data) / uint64(ctrlSimWeight+dataSimWeight))
}

// hasControlEdges reports whether the PDG has any control-flow edges.
func hasControlEdges(pdg *MiningGraph) bool {
	for _, e := range pdg.Edges {
		if e.Kind == Ctrl {
			return true
		}
	}
	return false
}

// Mineable is rstyle's size floor: worth comparing.
func Mineable(w *Wl) bool { return w.nodes() >= minNodes && w.calls >= minCalls }

// Comparable is rstyle's scale-ratio pre-filter.
func Comparable(a, b *Wl) bool {
	small, large := a.nodes(), b.nodes()
	if small > large {
		small, large = large, small
	}
	return Mineable(a) && Mineable(b) && large <= small*maxScale
}

// CallClass returns the signature class of a call for blocking keys, porting
// rstyle's wl::call_class: the normalized callee signature, or the callee id
// without one. Unlike label(), the callee id is kept (names are fine for
// blocking; they are excluded from WL labels, not from block keys).
func CallClass(n PdgNode) string {
	if n.SigClass != "" {
		return n.SigClass
	}
	if n.CalleeID != "" {
		return n.CalleeID
	}
	return "?"
}
