// LSH-vectorized semantic clone discovery at repository scale, after
// Gabel, Jiang, Su, "Scalable detection of semantic clones" (ICSE 2008).
//
// The per-file pipeline (FindSemanticClones) buckets subgraphs by exact WL
// hash, which is O(subgraphs) but still enumerates and hashes every pair
// independently. At repository scale the win is different: LSH is the
// coarse filter that finds candidate pairs with no O(n^2) comparison at
// all, and backtracking isomorphism is the fine filter that removes LSH's
// near-duplicate false positives.
//
// Pipeline:
//  1. Enumerate bounded connected PDG subgraphs per function (10-12 nodes).
//  2. Vectorize each subgraph's WL histograms (512-dim, isomorphism-
//     invariant, so identical subgraphs always collide in every LSH table).
//  3. LSH-cluster the vectors: similar vectors share buckets with no
//     pairwise comparison.
//  4. Partition each cluster into isomorphic classes with the existing
//     backtracking check; keep classes spanning 2+ functions.
//
// Feed the result to SemanticCloneCandidates for dedup, ranking, and
// capping. (Anti-unification over the verified pairs is the natural next
// step: it already drives the parameterize fixer for per-file clones.)
package patterns

import (
	"sort"
	"strconv"
)

// repoLSHTables and repoLSHHashes trade recall for precision in the coarse
// filter; repoLSHSeed keeps clustering deterministic across runs.
const (
	repoLSHTables = 8
	repoLSHHashes = 6
	repoLSHSeed   = 42
)

// MineRepoClones finds semantic clone groups across a repository's PDGs,
// keyed by function ID. It returns verified groups of isomorphic subgraphs
// (each spanning 2+ functions); callers typically pass the result to
// SemanticCloneCandidates for deduplication, ranking, and capping.
func MineRepoClones(pdgs map[string]*Pdg) [][]Subgraph {
	sgs := collectRepoSubgraphs(pdgs)
	if len(sgs) == 0 {
		return nil
	}
	vectors := vectorizeRepoSubgraphs(sgs, pdgs)
	lsh := NewLSH(lshDim, repoLSHTables, repoLSHHashes, repoLSHSeed)
	clusters := ClusterClones(vectors, lsh)
	return verifyRepoClusters(clusters, sgs, pdgs)
}

// collectRepoSubgraphs enumerates clone-sized subgraphs for every function,
// sorted by (FuncID, Hash) so downstream ids are deterministic.
func collectRepoSubgraphs(pdgs map[string]*Pdg) []Subgraph {
	var out []Subgraph
	for funcID, pdg := range pdgs {
		if pdg == nil {
			continue
		}
		out = append(out, enumerateSubgraphs(funcID, pdg)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FuncID != out[j].FuncID {
			return out[i].FuncID < out[j].FuncID
		}
		return out[i].Hash < out[j].Hash
	})
	return out
}

// vectorizeRepoSubgraphs runs WL on each subgraph's induced sub-PDG and
// flattens the histograms to a characteristic vector. Ids are decimal
// indices into sgs, matching the order collectRepoSubgraphs produced.
//
// Note: buildSubgraph already ran WL once to compute the hash; this runs it
// again for the vector. The subgraphs are 10-12 nodes, so the extra pass is
// cheap, and it keeps this pipeline from touching the per-file one.
func vectorizeRepoSubgraphs(sgs []Subgraph, pdgs map[string]*Pdg) map[string][]float64 {
	out := make(map[string][]float64, len(sgs))
	for i, sg := range sgs {
		sub := inducedSubPdg(pdgs[sg.FuncID], sg.Nodes)
		out[strconv.Itoa(i)] = Vectorize(NewWl(sub))
	}
	return out
}

// verifyRepoClusters partitions each LSH cluster into isomorphic classes,
// keeping only classes that span 2+ functions. This is the fine filter:
// LSH is approximate, so near-duplicate vectors can share a bucket without
// being truly isomorphic.
func verifyRepoClusters(clusters [][]string, sgs []Subgraph, pdgs map[string]*Pdg) [][]Subgraph {
	var out [][]Subgraph
	for _, cluster := range clusters {
		members := resolveRepoMembers(cluster, sgs)
		out = append(out, partitionIsomorphic(members, pdgs)...)
	}
	return out
}

// resolveRepoMembers maps cluster ids back to their subgraphs, skipping
// any id that does not parse (defensive; ids are always ours).
func resolveRepoMembers(cluster []string, sgs []Subgraph) []Subgraph {
	out := make([]Subgraph, 0, len(cluster))
	for _, id := range cluster {
		i, err := strconv.Atoi(id)
		if err != nil || i < 0 || i >= len(sgs) {
			continue
		}
		out = append(out, sgs[i])
	}
	return out
}

// partitionIsomorphic splits members into classes where every member is
// isomorphic to the class's first element, keeping classes that span 2+
// functions. Because isomorphism is an equivalence relation, comparing
// each member to the base (rather than all pairs) is sufficient.
func partitionIsomorphic(members []Subgraph, pdgs map[string]*Pdg) [][]Subgraph {
	var out [][]Subgraph
	rest := members
	for len(rest) > 0 {
		class, remaining := growIsomorphicClass(rest, pdgs)
		if len(class) >= 2 && countFuncs(class) >= 2 {
			out = append(out, class)
		}
		rest = remaining
	}
	return out
}

// growIsomorphicClass takes the first member as base and splits the rest
// into members isomorphic to base (class) and the rest (remaining).
func growIsomorphicClass(members []Subgraph, pdgs map[string]*Pdg) ([]Subgraph, []Subgraph) {
	base := members[0]
	class := []Subgraph{base}
	var remaining []Subgraph
	for _, sg := range members[1:] {
		if subgraphsIsomorphic(pdgs[base.FuncID], base.Nodes, pdgs[sg.FuncID], sg.Nodes) {
			class = append(class, sg)
		} else {
			remaining = append(remaining, sg)
		}
	}
	return class, remaining
}
