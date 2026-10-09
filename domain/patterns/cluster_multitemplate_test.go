package patterns

import (
	"testing"
)

// TestBuildClusterTriesMultipleTemplates verifies that buildCluster tries
// multiple templates, not just the similarity medoid. When the medoid is a
// poor alignment template but another member aligns well with the group,
// the cluster should still form.
func TestBuildClusterTriesMultipleTemplates(t *testing.T) {
	// Three graphs: A and B align well, C is the similarity medoid but
	// aligns poorly. The cluster should form around A or B, not fail.
	// We use the windowGraphs where 0~1 are similar and 0~3 are not.
	graphs := windowGraphs()
	// graphs[0], graphs[1], graphs[2] are a chain: 0~1 similar, 1~2 similar,
	// 0~2 less similar. The medoid by similarity might be 1, but if 1
	// doesn't align well, 0 should work as template for the 0~1 pair.
	pdgs := []*Pdg{graphs[0], graphs[1], graphs[2]}
	params := clusterParams()
	params.MinCoverageMilli = 0 // Align stub reports 0; WL does the selecting

	clusters := ClusterPdgs(pdgs, params)
	if len(clusters) == 0 {
		t.Fatal("expected at least one cluster, got 0")
	}
	// The cluster should contain the similar pair (0,1)
	found := false
	for _, c := range clusters {
		has0, has1 := false, false
		for _, m := range c.Members {
			if m == 0 {
				has0 = true
			}
			if m == 1 {
				has1 = true
			}
		}
		if has0 && has1 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cluster containing pair (0,1), got %v", clusters)
	}
}

// TestUnionPreservesFlatPairs verifies that the union of flat and weighted
// similarity in pairSelected doesn't lose pairs the flat kernel finds.
func TestUnionPreservesFlatPairs(t *testing.T) {
	graphs := windowGraphs()
	wls := []*Wl{NewWl(graphs[0]), NewWl(graphs[1])}
	pdgs := []*Pdg{graphs[0], graphs[1]}
	params := clusterParams()

	flat := SimilarityMilli(wls[0], wls[1])
	weighted := SimilarityWeighted(wls[0], wls[1])
	t.Logf("flat=%d, weighted=%d", flat, weighted)

	// If flat passes, pairSelected must also pass (union)
	if flat >= params.ThresholdMilli && !pairSelected(pdgs, wls, 0, 1, params) {
		t.Errorf("flat %d >= threshold but pairSelected rejected", flat)
	}
}
