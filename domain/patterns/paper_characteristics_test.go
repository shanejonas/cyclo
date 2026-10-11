package patterns

import (
	"github.com/shanejonas/cyclo/domain/pdg"
	"math/rand"
	"reflect"
	"testing"
)

func TestPaperVectorUsesNativeCategoriesAndCounts(t *testing.T) {
	graph := &pdg.Graph{Function: pdg.Function{ReferenceCount: pdg.CountFact{Status: pdg.Known, Value: 3}}, Nodes: []pdg.Node{
		{Category: pdg.Declaration}, {Category: pdg.Assignment}, {Category: pdg.Computation}, {Category: pdg.Control}, {Category: pdg.Call}, {Category: pdg.FormalOutput},
	}, Edges: []pdg.Edge{{Kind: pdg.Data}, {Kind: pdg.ControlEdge}, {Kind: pdg.Execution}}}
	view := &MiningGraph{Paper: nativeCharacteristics(graph)}
	want := []float64{1, 1, 1, 1, 1, 1, 1, 1, 3}
	if got := characteristicVector(view); !reflect.DeepEqual(got, want) {
		t.Fatalf("vector=%v want %v", got, want)
	}
	clone := Canonicalize(view, RulesAll)
	if !reflect.DeepEqual(characteristicVector(clone), want) {
		t.Fatal("normalization drops source characteristics")
	}
	graph.Function.ReferenceCount.Status = pdg.Unknown
	view.Paper = nativeCharacteristics(graph)
	if characteristicVector(view) != nil {
		t.Fatal("unknown references become a measured zero")
	}
	if cosineSimilarity(want, nil) != 0 {
		t.Fatal("unknown vector passes numerical filter")
	}
}

func TestExecutionEdgeHasDistinctWLTagAndProfile(t *testing.T) {
	if edgeTag(Exec, 0) == edgeTag(Data, 0) || edgeTag(Exec, 0) == edgeTag(Ctrl, 0) {
		t.Fatal("execution edge loses kind")
	}
	graph := &MiningGraph{Nodes: []PdgNode{{Kind: Call}, {Kind: Call}}, Edges: []PdgEdge{{From: 0, To: 1, Kind: Exec}}}
	if len(buildGraph(graph).out[0]) != 0 || len(buildProfileGraph(graph, true).out[0]) != 1 {
		t.Fatal("execution profile not selected")
	}
}

func TestPaperWLBoundMatchesFullScoresWithExecutionEdges(t *testing.T) {
	rng := rand.New(rand.NewSource(91))
	for trial := range 60 {
		graphs := []*MiningGraph{randomDependenceGraph(rng), randomDependenceGraph(rng)}
		for _, graph := range graphs {
			for i := 1; i < len(graph.Nodes); i++ {
				graph.Edges = append(graph.Edges, PdgEdge{From: i - 1, To: i, Kind: Exec, ArgPos: -1})
			}
		}
		a, b := NewWlLight(graphs[0], nil), NewWlLight(graphs[1], nil)
		for _, threshold := range []uint32{0, 600, 899, 900, 901, 1000} {
			if got, want := similarityAtLeast(a, b, threshold), SimilarityMilli(a, b) >= threshold; got != want {
				t.Fatalf("trial %d threshold %d: bounded=%t full=%t", trial, threshold, got, want)
			}
		}
	}
}
