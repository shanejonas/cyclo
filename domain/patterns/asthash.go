package patterns

import (
	"go/ast"
	"reflect"
)

// AST pre-filter for CCGraph (Stage 0).
//
// Recent work (MAGNET 2025, FA-AST-GMN 2025) shows that combining AST and
// PDG signals catches more clones than PDG alone: AST hashing is nearly
// free and catches syntactic clones that the PDG-based characteristic
// vector might score below its 0.9 threshold.
//
// This stage runs BEFORE the paper-exact CCGraph pipeline (Zou et al.,
// ASE 2020). It does not change any paper threshold and does not reorder
// the paper stages. It only ADDS candidate pairs (high AST similarity)
// that the characteristic-vector filter would have missed. It never
// removes pairs the paper pipeline would have kept, so coverage cannot
// regress: semantic (Type-3/4) clones with dissimilar ASTs still flow
// through the normal path.

// Thresholds for the AST pre-filter. These are not paper thresholds;
// they tune the additional Stage 0 only.
const (
	// astBypassThreshold is the minimum AST Jaccard similarity for a
	// pair to bypass the characteristic-vector filter and go straight
	// to the candidate set (then through LSH and WL as normal).
	astBypassThreshold = 0.8
	// astPruneThreshold is the maximum AST Jaccard similarity for a
	// pair to be eligible for pruning. Pruning additionally requires
	// the characteristic vector to be below its paper threshold, so
	// the prune is conservative: it only drops pairs Stage 1 would
	// already have rejected.
	astPruneThreshold = 0.3
)

// AstNodeMultiset returns the multiset of AST node type names for a
// function, ignoring identifiers and literals. This is a Deckard-style
// structural fingerprint: two functions with the same shape but
// different names produce the same multiset. Nil input yields nil.
func AstNodeMultiset(fn *ast.FuncDecl) map[string]int {
	if fn == nil {
		return nil
	}
	out := map[string]int{}
	ast.Inspect(fn, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		// Identifiers and literals carry names/values, not structure.
		switch n.(type) {
		case *ast.Ident, *ast.BasicLit:
			return true
		}
		out[astTypeName(n)]++
		return true
	})
	return out
}

// astTypeName returns a stable structural name for an AST node.
func astTypeName(n ast.Node) string {
	t := reflect.TypeOf(n)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// AstJaccard returns the Jaccard similarity of two AST node multisets
// in [0, 1]. Intersection uses min counts, union uses max counts.
// Two empty multisets score 1.0; empty vs non-empty scores 0.0.
func AstJaccard(a, b map[string]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	inter, union := astMultisetOverlap(a, b)
	if union == 0 {
		return 1.0
	}
	return float64(inter) / float64(union)
}

// astMultisetOverlap returns the intersection and union sizes of two
// multisets, using min counts for intersection and max for union.
func astMultisetOverlap(a, b map[string]int) (inter, union int) {
	seen := map[string]bool{}
	for k, ca := range a {
		seen[k] = true
		inter += min(ca, b[k])
		union += max(ca, b[k])
	}
	for k, cb := range b {
		if !seen[k] {
			union += cb
		}
	}
	return inter, union
}

// ccASTBypass returns the set of pair keys (ccPairKey over positions in
// ids) whose AST Jaccard similarity meets the bypass threshold. ids is
// the sorted function ID list; types maps ID to its AST node multiset.
// Functions without a multiset are skipped (conservative: no bypass).
// The output is a set, so iteration order cannot affect the result.
func ccASTBypass(ids []string, types map[string]map[string]int) map[int64]bool {
	out := map[int64]bool{}
	if len(types) == 0 {
		return out
	}
	for i := 0; i < len(ids); i++ {
		ccASTBypassRow(ids, i, types, out)
	}
	return out
}

// ccMergeBypass merges Stage 0 bypass pair keys into the candidate set.
func ccMergeBypass(candidates, bypass map[int64]bool) {
	for k := range bypass {
		candidates[k] = true
	}
}

// ccASTBypassRow checks pairs (ids[i], ids[j]) for j > i, adding those
// that clear the bypass threshold to out.
func ccASTBypassRow(ids []string, i int, types map[string]map[string]int, out map[int64]bool) {
	ti := types[ids[i]]
	if len(ti) == 0 {
		return
	}
	for j := i + 1; j < len(ids); j++ {
		tj := types[ids[j]]
		if len(tj) == 0 {
			continue
		}
		if AstJaccard(ti, tj) >= astBypassThreshold {
			out[ccPairKey(i, j)] = true
		}
	}
}
