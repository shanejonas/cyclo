package patterns

import (
	"encoding/binary"
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
// are deliberately absent — only kinds and signature/type classes.
func label(n PdgNode) string {
	switch n.Kind {
	case Call:
		return "call:" + n.SigClass
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

func edgeTag(kind EdgeKind, argPos int) uint64 {
	var isCtrl uint64
	if kind == Ctrl {
		isCtrl = 1
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

func adjacency(n int, pdg *Pdg, forward bool) [][]Nb {
	lists := make([][]Nb, n)
	for _, e := range pdg.Edges {
		if e.From >= n || e.To >= n {
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

func buildGraph(pdg *Pdg) Graph {
	n := len(pdg.Nodes)
	labels := make([]uint64, n)
	for i, node := range pdg.Nodes {
		labels[i] = fnv1a([]byte(label(node)))
	}
	return Graph{labels: labels, inc: adjacency(n, pdg, false), out: adjacency(n, pdg, true)}
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

func refine(g *Graph, prev []uint64) []uint64 {
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

// eccentricity is the longest shortest path from start (BFS levels).
func eccentricity(g *Graph, start int) int {
	seen := make([]bool, len(g.labels))
	seen[start] = true
	frontier := []int{start}
	depth := 0
	for len(frontier) > 0 {
		frontier = expandFrontier(g, frontier, seen)
		if len(frontier) == 0 {
			return depth
		}
		depth++
	}
	return depth
}

// expandFrontier returns the unvisited neighbours of frontier, marking them.
func expandFrontier(g *Graph, frontier []int, seen []bool) []int {
	fresh := map[int]bool{}
	for _, v := range frontier {
		collectNeighbours(g.inc[v], seen, fresh)
		collectNeighbours(g.out[v], seen, fresh)
	}
	next := make([]int, 0, len(fresh))
	for node := range fresh {
		seen[node] = true
		next = append(next, node)
	}
	return next
}

func collectNeighbours(nbs []Nb, seen []bool, fresh map[int]bool) {
	for _, nb := range nbs {
		if !seen[nb.node] {
			fresh[int(nb.node)] = true
		}
	}
}

func diameter(g *Graph) int {
	if len(g.labels) > diameterLimit {
		return maxLevels
	}
	max := 0
	for v := range g.labels {
		if e := eccentricity(g, v); e > max {
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
}

// NewWl builds the WL refinement of a PDG.
func NewWl(pdg *Pdg) *Wl {
	g := buildGraph(pdg)
	rounds := refineRounds(&g)
	return &Wl{
		graph:    g,
		rounds:   rounds,
		hists:    histograms(rounds),
		calls:    countCalls(pdg),
		diameter: diameter(&g),
	}
}

func refineRounds(g *Graph) [][]uint64 {
	rounds := [][]uint64{g.labels}
	for i := 1; i < maxLevels; i++ {
		rounds = append(rounds, refine(g, rounds[len(rounds)-1]))
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

func countCalls(pdg *Pdg) int {
	calls := 0
	for _, n := range pdg.Nodes {
		if n.Kind == Call {
			calls++
		}
	}
	return calls
}

func (w *Wl) nodes() int { return len(w.graph.labels) }

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
