package patterns

import (
	"math/rand"
	"reflect"
	"testing"
)

// Every pair is admitted when source features and names are unknown. Compare
// production routing with independent full scores, including duplicate graphs.
func TestCCGraphVerifiesAllAdmittedPairs(t *testing.T) {
	rng := rand.New(rand.NewSource(101))
	graphs := map[string]*MiningGraph{}
	for i := range 20 {
		id := string(rune('a' + i))
		graphs[id] = randomDependenceGraph(rng)
		graphs[id+"copy"] = graphs[id]
	}
	ids := ccSortableIDs(graphs)
	parent := ccMakeParent(ids)
	wls := make([]*Wl, len(ids))
	for i, id := range ids {
		wls[i] = NewWl(graphs[id])
	}
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if SimilarityMilli(wls[i], wls[j]) >= ccMatchThreshold {
				union(parent, ids[i], ids[j])
			}
		}
	}
	want := groupsOfTwoOrMore(parent)
	if got := CCGraphClones(graphs, nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("admitted pairs lost: got %v want %v", got, want)
	}
}
