package patterns

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

type referencePair struct{ A, B uint32 }
type exhaustiveDataset struct {
	SchemaVersion int             `json:"schema_version"`
	Threshold     uint32          `json:"threshold_milli"`
	IDs           []string        `json:"function_ids"`
	Pairs         []referencePair `json:"clone_pairs"`
}

// TestMeasureExhaustiveClones creates a reference for the fixed WL predicate.
// It bypasses AST, vector and name exclusions and is not a semantic oracle.
func TestMeasureExhaustiveClones(t *testing.T) {
	path := os.Getenv("CYCLO_MATCH_CORPUS")
	if path == "" || os.Getenv("CYCLO_MATCH_EXHAUSTIVE") != "1" {
		t.Skip("opt-in exhaustive reference")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var facts []*MiningFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		t.Fatal(err)
	}
	pdgs, names, asts := ccGraphInputs(facts)
	ids := ccSortableIDs(pdgs)
	wls := make([]*Wl, len(ids))
	vecs := make([][]float64, len(ids))
	alignedNames := make([]string, len(ids))
	for i, id := range ids {
		vecs[i] = characteristicVector(pdgs[id])
		wls[i] = NewWlLight(pdgs[id], vecs[i])
		alignedNames[i] = names[id]
	}
	started := time.Now()
	bounded := referencePairs(wls, func(a, b int) bool { return similarityAtLeast(wls[a], wls[b], ccMatchThreshold) })
	boundedElapsed := time.Since(started)
	started = time.Now()
	pairs := exhaustiveReference(wls)
	elapsed := time.Since(started)
	if !reflect.DeepEqual(pairs, bounded) {
		t.Fatal("bounded threshold predicate differs from exhaustive scores")
	}
	candidates := ccCandidatePairs(vecs, alignedNames, ccASTShapes(ids, asts))
	retained, numericalFailures, stringFailures := referenceSurvival(pairs, vecs, alignedNames, ccASTShapes(ids, asts), candidates)
	// Independently apply Algorithm 1's two alternative scores to full-score matches.
	parent := ccMakeParent(ids)
	shapes := ccASTShapes(ids, asts)
	for _, pair := range pairs {
		a, b := int(pair.A), int(pair.B)
		admitted := len(vecs[a]) == 0 || len(vecs[b]) == 0 || shapes[a].bypass(shapes[b]) || cosineSimilarity(vecs[a], vecs[b]) >= charVecThreshold || ccNamesSimilar(alignedNames[a], alignedNames[b])
		if admitted {
			union(parent, ids[a], ids[b])
		}
	}
	if !reflect.DeepEqual(CCGraphClonesWithAST(pdgs, names, asts), groupsOfTwoOrMore(parent)) {
		t.Fatal("production groups differ from independently filtered full scores")
	}
	if output := os.Getenv("CYCLO_MATCH_DATASET"); output != "" {
		encoded, err := json.Marshal(exhaustiveDataset{1, ccMatchThreshold, ids, pairs})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(output, encoded, 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("functions=%d pairs=%d reference_clones=%d retained=%d numerical_failures=%d string_failures=%d exhaustive_ms=%.3f bounded_ms=%.3f", len(ids), len(ids)*(len(ids)-1)/2, len(pairs), retained, numericalFailures, stringFailures, float64(elapsed.Microseconds())/1000, float64(boundedElapsed.Microseconds())/1000)
}

func exhaustiveReference(wls []*Wl) []referencePair {
	return referencePairs(wls, func(a, b int) bool { return SimilarityMilli(wls[a], wls[b]) >= ccMatchThreshold })
}

func referencePairs(wls []*Wl, accept func(a, b int) bool) []referencePair {
	workers := min(runtime.NumCPU(), len(wls))
	stripes := make([][]referencePair, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := worker; i < len(wls); i += workers {
				for j := i + 1; j < len(wls); j++ {
					if accept(i, j) {
						stripes[worker] = append(stripes[worker], referencePair{uint32(i), uint32(j)})
					}
				}
			}
		}(worker)
	}
	wg.Wait()
	var pairs []referencePair
	for _, stripe := range stripes {
		pairs = append(pairs, stripe...)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].A != pairs[j].A {
			return pairs[i].A < pairs[j].A
		}
		return pairs[i].B < pairs[j].B
	})
	return pairs
}

func referenceSurvival(pairs []referencePair, vecs [][]float64, names []string, shapes []astShape, candidates *ccPairs) (retained, vectorMisses, nameMisses int) {
	runes := ccNameRunes(names)
	matcher := newNameMatcher(runes)
	for _, pair := range pairs {
		a, b := int(pair.A), int(pair.B)
		bypass := shapes[a].bypass(shapes[b]) || len(vecs[a]) == 0 || len(vecs[b]) == 0
		numeric := cosineSimilarity(vecs[a], vecs[b]) >= charVecThreshold
		name := matcher.similar(runes[a], runes[b])
		if !bypass && !numeric {
			vectorMisses++
		}
		if !bypass && !name {
			nameMisses++
		}
		if !candidates.has(a, b) {
			continue
		}
		retained++
	}
	return
}
