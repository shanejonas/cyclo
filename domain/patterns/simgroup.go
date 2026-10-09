package patterns

import (
	"fmt"
	"sort"
)

// SimilarityGroup is a set of functions whose PDGs are similar per CCGraph.
// It's the shared grouping primitive for patterns that analyze "similar
// functions, then what's different": ccgraph_clone, inconsistent_clone,
// and future consumers.
//
// Grouping is separate from analysis: this file only finds the groups.
// Each pattern does its own analysis on the members. CCGraph itself is
// untouched (paper-exact); this only wraps its output in a clean type.
type SimilarityGroup struct {
	// GroupID is a stable identifier: "simgroup-<index>".
	GroupID string
	// MemberIDs are the function IDs in the group, sorted for determinism.
	MemberIDs []string
	// RepresentativeID is the group's representative member: the first
	// member by sorted ID. Deterministic; patterns that need a different
	// notion of centrality can pick their own.
	RepresentativeID string
	// Size is len(MemberIDs).
	Size int
}

// FindSimilarityGroups runs the CCGraph pipeline at the given WL similarity
// threshold (in thousandths) and returns the groups as SimilarityGroups.
//
// thresholdMilli is the Stage-4 WL similarity threshold. Pass
// ccMatchThreshold (900) for the paper-exact clone threshold; lower values
// find looser cohorts. Functions without a PDG are skipped.
func FindSimilarityGroups(facts []*FuncFacts, thresholdMilli uint32) []SimilarityGroup {
	pdgs, names := ccGraphInputs(facts)
	raw := ccGraphGroups(pdgs, names, thresholdMilli)
	return makeSimilarityGroups(raw)
}

// makeSimilarityGroups converts raw CCGraph groups to SimilarityGroups.
// Members are sorted for determinism; the representative is the first.
func makeSimilarityGroups(raw [][]string) []SimilarityGroup {
	out := make([]SimilarityGroup, 0, len(raw))
	for i, members := range raw {
		sorted := append([]string(nil), members...)
		sort.Strings(sorted)
		out = append(out, SimilarityGroup{
			GroupID:          fmt.Sprintf("simgroup-%d", i),
			MemberIDs:        sorted,
			RepresentativeID: sorted[0],
			Size:             len(sorted),
		})
	}
	return out
}
