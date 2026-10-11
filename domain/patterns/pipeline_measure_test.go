package patterns

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/bits"
	"os"
	"runtime"
	"testing"
	"time"
)

type measuredStage struct {
	Name           string  `json:"name"`
	Milliseconds   float64 `json:"milliseconds"`
	AllocatedBytes uint64  `json:"allocated_bytes"`
	Allocations    uint64  `json:"allocations"`
}

type pipelineMeasurement struct {
	Stages                                                                   []measuredStage `json:"stages"`
	Functions, PossiblePairs, Candidates, WLGraphs, Clusters, LargestCluster int
	VerificationPairs, ClonePairs                                            int
	GroupDigest                                                              string
}

func (m *pipelineMeasurement) stage(name string, work func()) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	work()
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	m.Stages = append(m.Stages, measuredStage{name, float64(elapsed.Microseconds()) / 1000, after.TotalAlloc - before.TotalAlloc, after.Mallocs - before.Mallocs})
}

// TestMeasureMatchingPipeline uses production stage helpers on a frozen corpus.
// It keeps instrumentation out of the normal matcher and reports pair counts
// before grouping. Each measured run uses a fresh test process.
func TestMeasureMatchingPipeline(t *testing.T) {
	path := os.Getenv("CYCLO_MATCH_CORPUS")
	if path == "" {
		t.Skip("opt-in matching measurement")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var facts []*MiningFacts
	if err := json.NewDecoder(file).Decode(&facts); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	pdgs, names, asts := ccGraphInputs(facts)
	ids := ccSortableIDs(pdgs)
	m := pipelineMeasurement{Functions: len(ids), PossiblePairs: len(ids) * (len(ids) - 1) / 2}
	var shapes []astShape
	var vecs [][]float64
	var alignedNames []string
	m.stage("shared_labels", func() {
		cache := &baseLabelCache{hashes: map[baseLabelKey]uint64{}}
		for _, g := range pdgs {
			for _, node := range g.Nodes {
				cache.add(node)
			}
		}
		for _, g := range pdgs {
			cache.attach(g)
		}
	})
	m.stage("features", func() {
		shapes = ccASTShapes(ids, asts)
		vecs = make([][]float64, len(ids))
		alignedNames = make([]string, len(ids))
		for i, id := range ids {
			vecs[i] = characteristicVector(pdgs[id])
			alignedNames[i] = names[id]
		}
	})
	var candidates *ccPairs
	m.stage("pair_filters", func() { candidates = ccCandidatePairs(vecs, alignedNames, shapes) })
	for _, word := range candidates.words {
		m.Candidates += bits.OnesCount64(word)
	}
	var wls map[string]*Wl
	m.stage("wl_features", func() { wls = ccBuildWls(ids, candidates, pdgs, vecs) })
	m.WLGraphs = len(wls)
	clusters := [][]string{ids}
	m.Clusters = len(clusters)
	for _, cluster := range clusters {
		m.LargestCluster = max(m.LargestCluster, len(cluster))
	}
	var groups [][]string
	m.stage("verify", func() { groups = measureVerification(ids, candidates, wls, clusters) })
	measurePairCounts(ids, candidates, wls, clusters, &m)
	data, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	m.GroupDigest = fmt.Sprintf("%x", sha256.Sum256(data))
	if os.Getenv("CYCLO_MATCH_VERIFY") == "1" {
		actual := CCGraphClonesWithAST(pdgs, names, asts)
		encoded, err := json.Marshal(actual)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(encoded)) != m.GroupDigest {
			t.Fatal("measured helpers differ from production pipeline")
		}
	}
	output, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(output))
}

func measureVerification(ids []string, candidates *ccPairs, wls map[string]*Wl, clusters [][]string) [][]string {
	indexes := ccIDIndex(ids)
	aligned := ccAlignedWLs(ids, wls)
	parent := ccMakeParent(ids)
	// Production verification is parallel. Collect pair counts in a separate
	// untimed pass; no atomic counters enter the hot similarity loop.
	sim := func(a, b int) bool { return similarityAtLeast(aligned[a], aligned[b], ccMatchThreshold) }
	for _, cluster := range clusters {
		ccUnionSimilar(cluster, candidates, indexes, sim, parent)
	}
	return groupsOfTwoOrMore(parent)
}

func measurePairCounts(ids []string, candidates *ccPairs, wls map[string]*Wl, clusters [][]string, m *pipelineMeasurement) {
	indexes := ccIDIndex(ids)
	aligned := ccAlignedWLs(ids, wls)
	sim := func(a, b int) bool { return similarityAtLeast(aligned[a], aligned[b], ccMatchThreshold) }

	for _, cluster := range clusters {
		for i, a := range cluster {
			for _, b := range cluster[i+1:] {
				x, y := indexes[a], indexes[b]
				if !candidates.has(x, y) {
					continue
				}
				m.VerificationPairs++
				if sim(x, y) {
					m.ClonePairs++
				}
			}
		}
	}
}
