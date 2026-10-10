package patterns

import "sort"

// Neighborhood-augmented Weisfeiler-Lehman kernel.
//
// Cyclo's adaptation of the multi-scale WL idea: where standard WL (wl.go)
// runs relabeling once over the whole graph, this kernel runs one WL
// refinement round on the k-hop neighborhood of every node, for k = 1..2,
// and combines the per-level histogram kernels. The k-hop neighborhoods
// capture local structural patterns at multiple scales that whole-graph WL
// averages away.
//
// This is inspired by multi-scale WL approaches in the literature (e.g. the
// neighborhood-kernel construction in Kim & Oh's WLKS work, ICLR 2025,
// arXiv:2412.02181), but it is Cyclo's own adaptation, not that paper's
// algorithm: the paper compares target subgraphs inside a larger global graph
// with k=0 and k=diameter, while this kernel compares independent
// whole-function PDGs using per-node k=1,2 neighborhoods.
//
// Colors remain comparable across neighborhoods and graphs because the
// relabeling (digest/refine in wl.go) is deterministic: identical
// neighborhood shapes always produce identical color multisets.
//
// The representation is precomputed per graph (like Wl); pairwise similarity
// is histogram intersection, so Stage 4 pair checks stay cheap.

const (
	// neighborhoodWLMaxK is the largest k-hop neighborhood level. k = 1 captures
	// immediate call/data neighborhoods; k = 2 reaches one step further.
	// K=2 is the floor for no recall regression: K=1 misses real clones
	// (e.g. forceTypeAssertCandidates/typedNilCandidates) that standard
	// WL finds, while K=2 is a strict superset. Larger k adds linear cost
	// as neighborhoods approach the whole graph.
	neighborhoodWLMaxK = 2
	// neighborhoodWLRounds is the WL refinement depth inside each neighborhood.
	// One round keeps neighborhoods robust to small edits: deeper
	// refinement amplifies single-node deletions across every
	// neighborhood containing that node, hurting gapped-clone recall.
	neighborhoodWLRounds = 1
)

// NeighborhoodWL holds the k-hop neighborhood WL histograms of one graph,
// precomputed for fast pairwise kernel evaluation.
type NeighborhoodWL struct {
	// hists[k] is the aggregated WL color histogram over all (k+1)-hop
	// neighborhoods, sorted by color for linear intersection.
	hists [][]histEntry
	// totals[k] is the total color count in hists[k]: the sum over all
	// nodes of their (k+1)-hop neighborhood sizes. Used for normalization.
	totals []int
}

// neighborhoodWLWorkspace holds reusable buffers for neighborhood computation,
// avoiding per-neighborhood allocations. The seen array uses generation
// counters so it never needs clearing.
type neighborhoodWLWorkspace struct {
	seen  []int // generation marker per node
	queue []int // BFS queue
	nodes []int // neighborhood node list
	gen   int
}

// newNeighborhoodWLWorkspace allocates buffers sized for a graph with n nodes.
func newNeighborhoodWLWorkspace(n int) *neighborhoodWLWorkspace {
	return &neighborhoodWLWorkspace{
		seen:  make([]int, n),
		queue: make([]int, 0, 64),
		nodes: make([]int, 0, 64),
	}
}

// markKHop runs BFS from start up to k hops, marking visited nodes with the
// current generation. Returns the neighborhood node list. Reuses buffers.
func (ws *neighborhoodWLWorkspace) markKHop(g *Graph, start, k int) []int {
	ws.gen++
	gen := ws.gen
	ws.queue = ws.queue[:0]
	ws.nodes = ws.nodes[:0]
	ws.seen[start] = gen
	ws.queue = append(ws.queue, start)
	ws.nodes = append(ws.nodes, start)
	for depth := 0; depth < k && len(ws.queue) > 0; depth++ {
		levelSize := len(ws.queue)
		for i := 0; i < levelSize; i++ {
			v := ws.queue[0]
			ws.queue = ws.queue[1:]
			ws.queue = ws.markNeighbors(g.inc[v], gen, ws.queue)
			ws.queue = ws.markNeighbors(g.out[v], gen, ws.queue)
		}
		// nodes added this level are at the end of ws.nodes via markNeighbors
	}
	return ws.nodes
}

// markNeighbors appends unvisited neighbors to queue/nodes, marking them.
func (ws *neighborhoodWLWorkspace) markNeighbors(nbs []Nb, gen int, queue []int) []int {
	for _, nb := range nbs {
		if w := int(nb.node); ws.seen[w] != gen {
			ws.seen[w] = gen
			queue = append(queue, w)
			ws.nodes = append(ws.nodes, w)
		}
	}
	return queue
}

// digestFiltered is digest() restricted to neighbors in the current
// generation set. It computes the 1-round WL color without building a
// subgraph: same hash, same sort, just filtered neighbor lists.
func (ws *neighborhoodWLWorkspace) digestFiltered(own, salt uint64, neighbours []Nb, prev []uint64) uint64 {
	seen, gen := ws.seen, ws.gen
	// Count first to size the slice exactly (avoids over-allocation).
	n := 0
	for _, nb := range neighbours {
		if seen[int(nb.node)] == gen {
			n++
		}
	}
	parts := make([]uint64, 0, n)
	for _, nb := range neighbours {
		if seen[int(nb.node)] == gen {
			parts = append(parts, combine(nb.tag, prev[nb.node]))
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i] < parts[j] })
	acc := combine(own, salt)
	for _, p := range parts {
		acc = combine(acc, p)
	}
	return acc
}

// neighborhoodColorsFast computes the 1-round WL colors for the k-hop
// neighborhood of v, without building an induced subgraph. It runs BFS with
// reused buffers, then applies digestFiltered per node. The result matches
// neighborhoodColors exactly (same hashes, same neighbor sets).
func (ws *neighborhoodWLWorkspace) neighborhoodColorsFast(g *Graph, v, k int, out []uint64) []uint64 {
	nodes := ws.markKHop(g, v, k)
	for _, u := range nodes {
		inward := ws.digestFiltered(g.labels[u], inSalt, g.inc[u], g.labels)
		out = append(out, ws.digestFiltered(inward, outSalt, g.out[u], g.labels))
	}
	return out
}

// NewNeighborhoodWL builds the neighborhood-WL representation of a PDG: for each k in 1..neighborhoodWLMaxK
// and each node, the k-hop neighborhood's WL colors are aggregated into the
// level histogram. Uses reused workspace buffers; no per-neighborhood
// allocations, no subgraph construction.
func NewNeighborhoodWL(pdg *Pdg) *NeighborhoodWL {
	g := buildGraph(pdg)
	ws := newNeighborhoodWLWorkspace(len(g.labels))
	w := &NeighborhoodWL{
		hists:  make([][]histEntry, neighborhoodWLMaxK),
		totals: make([]int, neighborhoodWLMaxK),
	}
	for k := 1; k <= neighborhoodWLMaxK; k++ {
		var colors []uint64
		for v := range g.labels {
			colors = ws.neighborhoodColorsFast(&g, v, k, colors)
		}
		w.hists[k-1] = histogram(colors)
		w.totals[k-1] = len(colors)
	}
	return w
}

// neighborhoodWLWeights are (neighborhoodWLMaxK, ..., 1): tighter neighborhoods count more,
// mirroring weights() in wl.go where earlier refinement levels dominate.
func neighborhoodWLWeights() []uint64 {
	w := make([]uint64, neighborhoodWLMaxK)
	for i := range w {
		w[i] = uint64(neighborhoodWLMaxK - i)
	}
	return w
}

// SimilarityNeighborhoodWLMilli is the neighborhood-WL kernel in thousandths:
//
//	sum_k w_k |H_k(A) ∩ H_k(B)| / sum_k w_k max(total_k(A), total_k(B))
//
// Each level normalizes by its own total color count (neighborhoods hold
// many colors per node), mirroring SimilarityMilli's max(|A|,|B|)
// normalization so the same 900 (0.9) match threshold applies.
// Deterministic: histograms are sorted, intersection is exact.
func SimilarityNeighborhoodWLMilli(a, b *NeighborhoodWL) uint32 {
	w := neighborhoodWLWeights()
	num, den := weightedNeighborhoodIntersection(a, b, w)
	if den == 0 {
		return 0
	}
	return uint32(num * 1000 / den)
}

// weightedNeighborhoodIntersection accumulates the weighted histogram
// intersection over all neighborhood levels, mirroring weightedIntersection
// in wl.go. Each level normalizes by its own max total.
func weightedNeighborhoodIntersection(a, b *NeighborhoodWL, w []uint64) (uint64, uint64) {
	var num, den uint64
	for k := 0; k < neighborhoodWLMaxK; k++ {
		denom := a.totals[k]
		if b.totals[k] > denom {
			denom = b.totals[k]
		}
		if denom == 0 {
			continue
		}
		num += w[k] * uint64(intersection(a.hists[k], b.hists[k]))
		den += w[k] * uint64(denom)
	}
	return num, den
}
