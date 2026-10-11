package patterns

import (
	"math"
	"math/rand"
	"reflect"
	"testing"

	"github.com/shanejonas/cyclo/domain/pdg"
)

func TestCachedNormsPreserveCosineBits(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	vecs := [][]float64{make([]float64, 7), {1, 0, 0, 0, 0, 0, 0}}
	for range 100 {
		v := make([]float64, 7)
		for i := range v {
			v[i] = rng.Float64() * 10000
		}
		vecs = append(vecs, v)
	}
	f := ccPairFilters{vecs: vecs, norms: vectorNorms(vecs)}
	for i := range vecs {
		for j := range vecs {
			got, want := f.cosine(i, j), cosineSimilarity(vecs[i], vecs[j])
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("cosine changed for %d,%d: %.17g != %.17g", i, j, got, want)
			}
		}
	}
}

func TestSharedLabelHashesPreserveRefinementAfterMutation(t *testing.T) {
	pool := pdg.NewBuilder()
	refs := []pdg.Ref{
		pool.MatchLabel(pdg.MatchLabel{Kind: pool.Text(string(Param)), TypeClass: pool.Text("int")}),
		pool.MatchLabel(pdg.MatchLabel{Kind: pool.Text(string(Call)), Callee: pool.Text("fmt.Println"), SignatureClass: pool.Text("fn(int)")}),
		pool.MatchLabel(pdg.MatchLabel{Kind: pool.Text(string(Op)), Detail: pool.Text("cmp:<")}),
	}
	native := &pdg.Graph{Tables: pool.Tables}
	for _, ref := range refs {
		native.Nodes = append(native.Nodes, pdg.Node{MatchLabel: ref})
	}
	cache := sharedBaseLabels([]*FuncFacts{nil, {Pdg: native}, {Pdg: native}})
	view := MiningView(native)
	cache.attach(view)
	view.Edges = []PdgEdge{{From: 0, To: 1, Kind: Data}, {From: 1, To: 2, Kind: Data}}
	plain := &MiningGraph{Nodes: append([]PdgNode(nil), view.Nodes...), Edges: view.Edges}
	if !reflect.DeepEqual(NewWl(view).rounds, NewWl(plain).rounds) {
		t.Fatal("cached refinement differs")
	}
	rewritten := clonePdg(view)
	rewritten.Nodes[1].Kind = Lit
	rewritten.Nodes[1].LitKind = "string"
	rewritten.Nodes[2].Detail = "arith:+"
	plain.Nodes = rewritten.Nodes
	if !reflect.DeepEqual(NewWl(rewritten).rounds, NewWl(plain).rounds) {
		t.Fatal("rewrites use stale labels")
	}
	if len(cache.hashes) != 3 {
		t.Fatal("published cache changes during reads")
	}
}

func TestBaseLabelKeysMatchExistingLabels(t *testing.T) {
	rng := rand.New(rand.NewSource(73))
	cache := &baseLabelCache{hashes: map[baseLabelKey]uint64{}}
	for range 100 {
		g := randomDependenceGraph(rng)
		for _, node := range g.Nodes {
			cache.add(node)
		}
		for _, node := range g.Nodes {
			if cache.hash(node) != fnv1a([]byte(label(node))) {
				t.Fatal("base-label hash changed")
			}
		}
	}
}

func TestPaperAlgorithmOneCandidateBranches(t *testing.T) {
	cases := []struct {
		name    string
		vectors [][]float64
		names   []string
		want    bool
	}{
		{"numerical match despite names", [][]float64{{1, 0}, {1, 0}}, []string{"aaaa", "zzzz"}, true},
		{"string fallback despite vectors", [][]float64{{1, 0}, {0, 1}}, []string{"fetchData", "fetchDatum"}, true},
		{"unknown source features", [][]float64{nil, {1, 0}}, []string{"aaaa", "zzzz"}, true},
		{"neither passes", [][]float64{{1, 0}, {0, 1}}, []string{"aaaa", "zzzz"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pairs := ccCandidatePairs(tc.vectors, tc.names, make([]astShape, 2))
			if got := pairs.has(0, 1); got != tc.want {
				t.Fatalf("candidate=%t want %t", got, tc.want)
			}
		})
	}
}

func TestPairBatchTransfersDistinctBuffers(t *testing.T) {
	found := make(chan []ccPairJob, 3)
	batch := ccPairBatch{pairs: make([]ccPairJob, 0, 64), found: found}
	for i := range 129 {
		batch.add(ccPairJob{a: string(rune(i)), b: "b"})
	}
	batch.flush()
	close(found)
	var got []ccPairJob
	for pairs := range found {
		got = append(got, pairs...)
	}
	if len(got) != 129 {
		t.Fatalf("received %d pairs", len(got))
	}
	for i, pair := range got {
		if pair.a != string(rune(i)) {
			t.Fatalf("buffer overwritten at pair %d", i)
		}
	}
}
