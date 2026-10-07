package patterns

import (
	"fmt"
	"sort"
	"testing"
)

// chainGraph builds: Param -> Call(sig[0]) -> ... -> Return, data-chained.
// Labels carry only kinds and signature classes, so two chains with the same
// sig multiset are structurally identical.
func chainGraph(sigs ...string) *Pdg {
	nodes := []PdgNode{{Kind: Param, TyClass: "_"}}
	for _, s := range sigs {
		nodes = append(nodes, PdgNode{Kind: Call, SigClass: s, CalleeID: s})
	}
	nodes = append(nodes, PdgNode{Kind: Return})
	var edges []PdgEdge
	for i := 0; i+1 < len(nodes); i++ {
		edges = append(edges, PdgEdge{From: i, To: i + 1, Kind: Data, ArgPos: 0})
	}
	return &Pdg{Nodes: nodes, Edges: edges}
}

const (
	sigF = "fn() -> _"
	sigG = "fn(_) -> _"
)

// Sliding window over one differing call: adjacent pairs score 675-725,
// distance-2 pairs 550, distance-3 pairs 425 (probed against the WL port).
func windowGraphs() []*Pdg {
	return []*Pdg{
		chainGraph(sigF, sigF, sigF, sigF, sigF, sigF),
		chainGraph(sigF, sigF, sigF, sigF, sigF, sigG),
		chainGraph(sigF, sigF, sigF, sigF, sigG, sigG),
		chainGraph(sigF, sigF, sigF, sigG, sigG, sigG),
	}
}

func memberSet(c Cluster) []int {
	sorted := append([]int{}, c.Members...)
	sort.Ints(sorted)
	return sorted
}

func TestDefaultParams(t *testing.T) {
	p := DefaultParams()
	if p.ThresholdMilli != 600 || p.MinCoverageMilli != 600 {
		t.Fatalf("thresholds = %d/%d, want 600/600", p.ThresholdMilli, p.MinCoverageMilli)
	}
	if p.PairCoverageMilli != nil {
		t.Fatal("PairCoverageMilli should default to nil")
	}
	if p.Normalize != (Rules{}) {
		t.Fatalf("Normalize should default to all-off, got %+v", p.Normalize)
	}
	if p.SingleCallGuard {
		t.Fatal("SingleCallGuard should default to false")
	}
	if p.MaxHoles != 3 {
		t.Fatalf("MaxHoles = %d, want 3", p.MaxHoles)
	}
}

func clusterParams() Params {
	// MinCoverageMilli 0 because the Align stub reports zero coverage until
	// align.go lands; the WL threshold does the real selecting here.
	return Params{ThresholdMilli: 600, MinCoverageMilli: 0}
}

// Medoid clustering must not chain: 0~1, 1~2, 2~3 are above threshold but the
// non-adjacent pairs are below it, so 3 must not ride in through middlemen.
func TestMedoidNotChained(t *testing.T) {
	clusters := ClusterPdgs(windowGraphs(), clusterParams())
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	c := clusters[0]
	if got := memberSet(c); !equalInts(got, []int{0, 1, 2}) {
		t.Fatalf("members = %v, want {0 1 2}: 3 chained in through a middleman", got)
	}
	if c.Members[0] != 1 {
		t.Fatalf("template = %d, want 1 (the medoid)", c.Members[0])
	}
	if c.CoverageMilli[0] != 1000 {
		t.Fatalf("template coverage = %d, want 1000", c.CoverageMilli[0])
	}
}

// The window pair (0,2) scores exactly 550: the threshold is inclusive.
func TestThresholdBoundary(t *testing.T) {
	graphs := windowGraphs()
	pair := []*Pdg{graphs[0], graphs[2]}

	at := clusterParams()
	at.ThresholdMilli = 550
	clusters := ClusterPdgs(pair, at)
	if len(clusters) != 1 {
		t.Fatalf("threshold 550: got %d clusters, want 1", len(clusters))
	}
	if clusters[0].Members[0] != 0 {
		t.Fatalf("threshold 550: template = %d, want 0 (similarity tie -> lowest index)",
			clusters[0].Members[0])
	}

	above := clusterParams()
	above.ThresholdMilli = 551
	if clusters := ClusterPdgs(pair, above); len(clusters) != 0 {
		t.Fatalf("threshold 551: got %d clusters, want 0", len(clusters))
	}
}

func TestUnrelatedFunctionsDoNotCluster(t *testing.T) {
	graphs := windowGraphs()
	if clusters := ClusterPdgs([]*Pdg{graphs[0], graphs[3]}, clusterParams()); len(clusters) != 0 {
		t.Fatalf("got %d clusters for dissimilar functions, want 0", len(clusters))
	}
}

func TestClusterEmptyInput(t *testing.T) {
	if clusters := ClusterPdgs(nil, clusterParams()); len(clusters) != 0 {
		t.Fatalf("got %d clusters for empty input, want 0", len(clusters))
	}
}

// The coverage floor rejects members: with the Align stub every alignment
// reports coverage 0, so a floor of 1 dissolves the cluster. Once align.go
// lands this becomes a real alignment-coverage check.
func TestCoverageFloor(t *testing.T) {
	graphs := windowGraphs()
	pair := []*Pdg{graphs[0], graphs[1]}

	loose := clusterParams()
	if clusters := ClusterPdgs(pair, loose); len(clusters) != 1 {
		t.Fatalf("floor 0: got %d clusters, want 1", len(clusters))
	}
	strict := clusterParams()
	strict.MinCoverageMilli = 1
	if clusters := ClusterPdgs(pair, strict); len(clusters) != 0 {
		t.Fatalf("floor 1: got %d clusters, want 0", len(clusters))
	}
}

// The pair-coverage experiment selects pairs the WL threshold rejects.
// (Still stub-bound: Align reports 0, so only a floor of 0 selects.)
func TestPairCoverageExperiment(t *testing.T) {
	graphs := windowGraphs()
	pair := []*Pdg{graphs[0], graphs[3]} // sim 425, below any WL threshold here

	wlOff := Params{ThresholdMilli: 1001, MinCoverageMilli: 0}
	if clusters := ClusterPdgs(pair, wlOff); len(clusters) != 0 {
		t.Fatalf("WL-only: got %d clusters, want 0", len(clusters))
	}

	floor := uint32(0)
	byCoverage := Params{ThresholdMilli: 1001, MinCoverageMilli: 0, PairCoverageMilli: &floor}
	if clusters := ClusterPdgs(pair, byCoverage); len(clusters) != 1 {
		t.Fatalf("pair coverage 0: got %d clusters, want 1", len(clusters))
	}
}

// withPrivateCalls pads a graph with two calls whose signature class is
// unique to tag, so its two rarest call classes are shared with nobody.
func withPrivateCalls(pdg *Pdg, tag int) *Pdg {
	out := &Pdg{Nodes: append([]PdgNode{}, pdg.Nodes...), Edges: pdg.Edges}
	for k := 0; k < 2; k++ {
		sig := fmt.Sprintf("priv:%d:%d", tag, k)
		out.Nodes = append(out.Nodes, PdgNode{Kind: Call, SigClass: sig, CalleeID: sig})
	}
	return out
}

// A tiny corpus compares every pair even when no rare call class is shared.
func TestSmallCorpusComparesAllPairs(t *testing.T) {
	base := windowGraphs()
	graphs := []*Pdg{withPrivateCalls(base[0], 0), withPrivateCalls(base[1], 1)}
	if pairs := candidatePairs(blockKeys(graphs)); len(pairs) != 0 {
		t.Fatalf("blocking found %d pairs, want 0 (private keys are unshared)", len(pairs))
	}
	if pairs := pairsToCompare(graphs); len(pairs) != 1 {
		t.Fatalf("small corpus compares %d pairs, want 1 (all pairs)", len(pairs))
	}
}

// A corpus past allPairsLimit falls back to blocking: with per-function
// private keys, every block is a singleton and no pair is compared.
func TestLargeCorpusFallsBackToBlocking(t *testing.T) {
	var graphs []*Pdg
	for i := 0; i <= allPairsLimit; i++ {
		graphs = append(graphs, withPrivateCalls(windowGraphs()[0], i))
	}
	if pairs := pairsToCompare(graphs); len(pairs) != 0 {
		t.Fatalf("large corpus compares %d pairs, want 0", len(pairs))
	}
}

func TestNextSeed(t *testing.T) {
	adjacency := map[int]map[int]bool{
		0: {1: true},
		1: {0: true, 2: true},
		2: {1: true, 3: true},
		3: {2: true},
	}
	free := map[int]bool{0: true, 1: true, 2: true, 3: true}
	seed, ok := nextSeed(adjacency, free)
	if !ok || seed != 1 {
		t.Fatalf("nextSeed = (%d, %v), want (1, true): degree tie -> lowest index", seed, ok)
	}
	if _, ok := nextSeed(adjacency, map[int]bool{}); ok {
		t.Fatal("nextSeed on empty free should report none")
	}
}

// Columns: one hole variable across members, values template-first, missing
// member values falling back to the template's, ordered by (site, kind, value).
func TestColumnsOf(t *testing.T) {
	joined := []joinedMember{
		{index: 1, alignment: Alignment{Holes: []Hole{
			{Kind: HoleType, A: "k::Dog", B: "k::Cat", Sites: [][2]int{{3, 5}}},
			{Kind: HoleMethod, A: "k::Dog::bark", B: "k::Cat::meow", Sites: [][2]int{{7, 9}}},
		}}},
		{index: 2, alignment: Alignment{Holes: []Hole{
			{Kind: HoleType, A: "k::Dog", B: "k::Cow", Sites: [][2]int{{3, 6}}},
			// No method hole here: the value falls back to the template's.
		}}},
	}
	cols := columnsOf(joined)
	if len(cols) != 2 {
		t.Fatalf("got %d columns, want 2", len(cols))
	}
	if cols[0].Var != "T0" || !equalStrings(cols[0].Values, []string{"k::Dog", "k::Cat", "k::Cow"}) {
		t.Fatalf("column 0 = %+v, want T0 [k::Dog k::Cat k::Cow]", cols[0])
	}
	if cols[1].Var != "M0" || !equalStrings(cols[1].Values, []string{"k::Dog::bark", "k::Cat::meow", "k::Dog::bark"}) {
		t.Fatalf("column 1 = %+v, want M0 with template fallback", cols[1])
	}
}

// Same-kind holes number per kind in site order; kinds order by declaration
// rank (Type before Op) when sites tie, not by constant string value.
func TestColumnsOfOrdering(t *testing.T) {
	joined := []joinedMember{
		{index: 1, alignment: Alignment{Holes: []Hole{
			{Kind: HoleType, A: "b", B: "b1", Sites: [][2]int{{9, 1}}},
			{Kind: HoleType, A: "a", B: "a1", Sites: [][2]int{{2, 1}}},
			{Kind: HoleOp, A: "==", B: "!=", Sites: [][2]int{{2, 3}}},
		}}},
	}
	cols := columnsOf(joined)
	if len(cols) != 3 {
		t.Fatalf("got %d columns, want 3", len(cols))
	}
	// Site 2 first: Type (rank 0) before Op (rank 5); then site 9.
	wantVars := []string{"T0", "O0", "T1"}
	for i, w := range wantVars {
		if cols[i].Var != w {
			t.Fatalf("column %d var = %s, want %s", i, cols[i].Var, w)
		}
	}
	if !equalStrings(cols[0].Values, []string{"a", "a1"}) {
		t.Fatalf("T0 values = %v, want [a a1]", cols[0].Values)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
