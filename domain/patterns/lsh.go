// LSH-vectorized semantic clone discovery, after Gabel, Jiang, Su,
// "Scalable detection of semantic clones" (ICSE 2008).
//
// Subgraph isomorphism is too slow at repository scale, so each PDG
// subgraph is mapped to a characteristic vector (the WL histogram,
// flattened into fixed-length form) and vectors are clustered with
// locality-sensitive hashing. Similar vectors land in the same buckets
// without O(n^2) pairwise comparison; only bucket-mates need the
// expensive verification step.
package patterns

import (
	"math/rand"
	"sort"
)

// lshBucketsPerRound is the number of hash buckets each WL refinement
// round contributes to the characteristic vector. Colors are folded by
// color % lshBucketsPerRound; collisions are acceptable because LSH is
// approximate by design.
const lshBucketsPerRound = 128

// lshDim is the fixed length of every characteristic vector:
// maxLevels WL rounds times the per-round bucket count.
const lshDim = maxLevels * lshBucketsPerRound

// Vectorize flattens a WL refinement into a fixed-length characteristic
// vector. Round r contributes lshBucketsPerRound entries; each histogram
// entry (color, count) adds count * weight(r) to bucket color %
// lshBucketsPerRound. Earlier rounds weigh more, matching the WL kernel's
// level weighting. Rounds the graph never reached stay zero, so the
// vector length is always lshDim.
//
// The vector is isomorphism-invariant: WL colors are canonical, and the
// folding is deterministic, so permuted node orders give identical vectors.
func Vectorize(w *Wl) []float64 {
	vec := make([]float64, lshDim)
	for r := 0; r < maxLevels && r < len(w.hists); r++ {
		weight := float64(maxLevels - r)
		base := r * lshBucketsPerRound
		for _, e := range w.hists[r] {
			bucket := e.color % lshBucketsPerRound
			vec[base+int(bucket)] += float64(e.count) * weight
		}
	}
	return vec
}

// lshTable is one random-projection hash table: numHashes random
// hyperplanes, with ids bucketed by which side of each plane they fall on.
type lshTable struct {
	planes  [][]float64
	buckets map[uint64][]string
}

// LSH is a random-projection locality-sensitive hash index over
// characteristic vectors. Similar vectors collide in at least one table
// with high probability; dissimilar vectors rarely do.
type LSH struct {
	dim       int
	numHashes int
	tables    []lshTable
}

// NewLSH builds an LSH index. dim must match the vector length (use
// lshDim for Vectorize output). numTables trades recall for memory;
// numHashes trades precision for recall and must be <= 64 (signatures
// are uint64 bit strings). seed makes the random hyperplanes
// deterministic, which the tests rely on.
func NewLSH(dim, numTables, numHashes int, seed int64) *LSH {
	rng := rand.New(rand.NewSource(seed))
	tables := make([]lshTable, numTables)
	for ti := range tables {
		planes := make([][]float64, numHashes)
		for hi := range planes {
			plane := make([]float64, dim)
			for d := range plane {
				plane[d] = rng.NormFloat64()
			}
			planes[hi] = plane
		}
		tables[ti] = lshTable{planes: planes, buckets: map[uint64][]string{}}
	}
	return &LSH{dim: dim, numHashes: numHashes, tables: tables}
}

// dot is the inner product of two vectors.
func dot(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// signature hashes a vector to one table's bucket: bit i is 1 when the
// vector lies on the non-negative side of hyperplane i.
func (t *lshTable) signature(vec []float64) uint64 {
	var sig uint64
	for i, plane := range t.planes {
		if dot(vec, plane) >= 0 {
			sig |= 1 << uint(i)
		}
	}
	return sig
}

// Add inserts id under vec's bucket in every table.
func (l *LSH) Add(id string, vec []float64) {
	for i := range l.tables {
		sig := l.tables[i].signature(vec)
		l.tables[i].buckets[sig] = append(l.tables[i].buckets[sig], id)
	}
}

// Query returns the ids that share a bucket with vec in any table:
// the candidate near-duplicates, with no pairwise comparison.
func (l *LSH) Query(vec []float64) []string {
	seen := map[string]bool{}
	var out []string
	for i := range l.tables {
		sig := l.tables[i].signature(vec)
		for _, id := range l.tables[i].buckets[sig] {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// find is union-find lookup with path compression.
func find(parent map[string]string, x string) string {
	for parent[x] != x {
		parent[x] = parent[parent[x]]
		x = parent[x]
	}
	return x
}

// union merges the sets containing a and b.
func union(parent map[string]string, a, b string) {
	ra, rb := find(parent, a), find(parent, b)
	if ra != rb {
		parent[ra] = rb
	}
}

// groupsOfTwoOrMore collects the union-find sets of size >= 2, sorted
// deterministically: members sorted within each group, groups sorted by
// first member.
func groupsOfTwoOrMore(parent map[string]string) [][]string {
	byRoot := map[string][]string{}
	for id := range parent {
		root := find(parent, id)
		byRoot[root] = append(byRoot[root], id)
	}
	var out [][]string
	for _, group := range byRoot {
		if len(group) >= 2 {
			sort.Strings(group)
			out = append(out, group)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// ClusterClones groups vector ids into clone clusters: every id is added
// to lsh, then ids that co-occur in any bucket are unioned. Only groups
// of 2+ are returned. Output is deterministic (sorted) regardless of
// map iteration order.
func ClusterClones(vectors map[string][]float64, lsh *LSH) [][]string {
	for id, vec := range vectors {
		lsh.Add(id, vec)
	}
	parent := make(map[string]string, len(vectors))
	for id := range vectors {
		parent[id] = id
	}
	for id, vec := range vectors {
		for _, cand := range lsh.Query(vec) {
			union(parent, id, cand)
		}
	}
	return groupsOfTwoOrMore(parent)
}
