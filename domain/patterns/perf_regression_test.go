package patterns

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

func TestIndexedDisjunctionMatchesScan(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 80; trial++ {
		sets := make([]CallSet, 20)
		for i := range sets {
			sets[i].Calls = map[string]bool{}
			for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
				if rng.Intn(5) != 0 {
					sets[i].Calls[name] = true
				}
			}
		}
		got := ruleStrings(MineDisjunctiveRules(sets))
		want := ruleStrings(scanDisjunctiveRules(sets))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d: indexed=%v scan=%v", trial, got, want)
		}
	}
}

func scanDisjunctiveRules(sets []CallSet) []DisjunctiveRule {
	var rules []DisjunctiveRule
	for a := range countSingles(sets) {
		var co []string
		for b := range countSingles(sets) {
			if a != b && countCoOccur(sets, []string{a, b}) > 0 {
				co = append(co, b)
			}
		}
		sort.Strings(co)
		cntA := countCoOccur(sets, []string{a})
		for i, b := range co {
			for _, c := range co[i+1:] {
				_, rb := makeRuleFromCounts(len(sets), cntA, countCoOccur(sets, []string{a, b}), []string{a}, b)
				_, rc := makeRuleFromCounts(len(sets), cntA, countCoOccur(sets, []string{a, c}), []string{a}, c)
				if rb || rc {
					continue
				}
				if r, ok := disjunctiveRuleFromCounts(len(sets), cntA, countDisjunct(sets, a, []string{b, c}), a, b, c); ok {
					rules = append(rules, r)
				}
			}
		}
	}
	return rules
}

func ruleStrings(rules []DisjunctiveRule) []string {
	var out []string
	for _, r := range rules {
		b, _ := json.Marshal(r)
		out = append(out, string(b))
	}
	sort.Strings(out)
	return out
}

func TestNameMatcherReusesBuffers(t *testing.T) {
	names := []string{"", "GetValue", "SetValue", "GetValues", "读取值", "读取项", "éclair", "eclair"}
	runes := ccNameRunes(names)
	m := newNameMatcher(runes)
	for repeat := 0; repeat < 3; repeat++ {
		for i, a := range names {
			for j, b := range names {
				if got, want := m.similar(runes[i], runes[j]), ccNamesSimilar(a, b); got != want {
					t.Fatalf("%q %q: got %v want %v", a, b, got, want)
				}
			}
		}
	}
}

func TestBucketClusteringMatchesQueries(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	for trial := 0; trial < 30; trial++ {
		vectors := map[string][]float64{}
		parent := map[string]string{}
		indexed := NewLSH(3, 4, 3, 42)
		for i := 0; i < 40; i++ {
			id := string(rune('a' + i))
			vec := []float64{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}
			vectors[id], parent[id] = vec, id
			indexed.Add(id, vec)
		}
		for id, vec := range vectors {
			for _, candidate := range indexed.Query(vec) {
				union(parent, id, candidate)
			}
		}
		got := ClusterClones(vectors, NewLSH(3, 4, 3, 42))
		if want := groupsOfTwoOrMore(parent); !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d: buckets=%v queries=%v", trial, got, want)
		}
	}
}

func TestCompactPairsAcrossWordBoundaries(t *testing.T) {
	p := newCCPairs(130)
	p.add(0, 64)
	p.add(63, 129)
	p.add(64, 65)
	for i := 0; i < 130; i++ {
		for j := i + 1; j < 130; j++ {
			want := i == 0 && j == 64 || i == 63 && j == 129 || i == 64 && j == 65
			if p.has(i, j) != want || p.has(j, i) != want {
				t.Fatalf("pair (%d,%d) differs", i, j)
			}
		}
	}
	if got, want := p.members(), []int{0, 63, 64, 65, 129}; !reflect.DeepEqual(got, want) {
		t.Fatalf("members=%v want %v", got, want)
	}
}

func TestPairFiltersMatchOriginalPipeline(t *testing.T) {
	rng := rand.New(rand.NewSource(53))
	ids := []string{"a", "b", "c", "d", "e", "f"}
	names := []string{"GetValue", "SetValue", "Other", "读取值", "", "ReadValue"}
	for trial := 0; trial < 100; trial++ {
		types := map[string]map[string]int{}
		vecs := make([][]float64, len(ids))
		for i, id := range ids {
			vecs[i] = []float64{rng.Float64(), rng.Float64(), rng.Float64()}
			if rng.Intn(4) == 0 {
				continue
			}
			types[id] = map[string]int{}
			for _, kind := range []string{"Call", "If", "Return", "Lit"} {
				if count := rng.Intn(10); count > 0 {
					types[id][kind] = count
				}
			}
		}
		got := ccCandidatePairs(vecs, names, ccASTShapes(ids, types))
		for i, a := range ids {
			for j := i + 1; j < len(ids); j++ {
				bypass := len(types[a]) > 0 && len(types[ids[j]]) > 0 && AstJaccard(types[a], types[ids[j]]) >= astBypassThreshold
				want := bypass || cosineSimilarity(vecs[i], vecs[j]) >= charVecThreshold && ccNamesSimilar(names[i], names[j])
				if got.has(i, j) != want {
					t.Fatalf("trial %d pair %d,%d differs", trial, i, j)
				}
			}
		}
	}
}

func TestDiameterMatchesShortestPaths(t *testing.T) {
	rng := rand.New(rand.NewSource(61))
	for trial := 0; trial < 30; trial++ {
		g := Graph{labels: make([]uint64, 15), inc: make([][]Nb, 15), out: make([][]Nb, 15)}
		dist := make([][]int, 15)
		for i := range dist {
			dist[i] = make([]int, 15)
			for j := range dist[i] {
				if i != j {
					dist[i][j] = 1000
				}
			}
		}
		for i := 0; i < 15; i++ {
			for j := i + 1; j < 15; j++ {
				if rng.Intn(6) != 0 {
					continue
				}
				g.out[i] = append(g.out[i], Nb{node: uint64(j)})
				g.inc[j] = append(g.inc[j], Nb{node: uint64(i)})
				dist[i][j], dist[j][i] = 1, 1
			}
		}
		for k := 0; k < 15; k++ {
			for i := 0; i < 15; i++ {
				for j := 0; j < 15; j++ {
					dist[i][j] = min(dist[i][j], dist[i][k]+dist[k][j])
				}
			}
		}
		want := 0
		for _, row := range dist {
			for _, d := range row {
				if d < 1000 {
					want = max(want, d)
				}
			}
		}
		if got := diameter(&g); got != want {
			t.Fatalf("trial %d: diameter=%d want %d", trial, got, want)
		}
	}
}
