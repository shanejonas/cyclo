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
