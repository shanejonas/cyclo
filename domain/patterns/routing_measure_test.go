package patterns

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"
)

// Opt-in full CCGraph route comparison. An exhaustive build uses a temporary
// Go overlay; this test adds no production routing or CLI flags.
func TestMeasureMatchingRoute(t *testing.T) {
	path, method := os.Getenv("CYCLO_MATCH_CORPUS"), os.Getenv("CYCLO_MATCH_METHOD")
	if path == "" || method == "" {
		t.Skip("opt-in routing measurement")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var facts []*MiningFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		t.Fatal(err)
	}
	labels := &baseLabelCache{hashes: map[baseLabelKey]uint64{}}
	for _, fact := range facts {
		for _, node := range fact.Pdg.Nodes {
			labels.add(node)
		}
		labels.attach(fact.Pdg)
	}
	pdgs, names, asts := ccGraphInputs(facts)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	m := pipelineMeasurement{}
	var groups [][]string
	m.stage("complete_ccgraph", func() { groups = CCGraphClonesWithAST(pdgs, names, asts) })
	runtime.GC()
	runtime.ReadMemStats(&after)
	encoded, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"method": method, "stages": m.Stages, "group_count": len(groups), "group_digest": fmt.Sprintf("%x", sha256.Sum256(encoded)), "retained_result_heap_bytes": int64(after.HeapAlloc) - int64(before.HeapAlloc)}
	if method == "exhaustive" {
		ids := ccSortableIDs(pdgs)
		wls := make([]*Wl, len(ids))
		for i, id := range ids {
			wls[i] = NewWlLight(pdgs[id], nil)
		}
		parent := ccMakeParent(ids)
		pairs := exhaustiveReference(wls)
		for _, pair := range pairs {
			union(parent, ids[pair.A], ids[pair.B])
		}
		if !reflect.DeepEqual(groups, groupsOfTwoOrMore(parent)) {
			t.Fatal("route differs from exhaustive full-score groups")
		}
		report["reference_pairs"] = len(pairs)
	}
	output, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(output))
	runtime.KeepAlive(pdgs)
	runtime.KeepAlive(names)
	runtime.KeepAlive(asts)
	runtime.KeepAlive(facts)
	runtime.KeepAlive(groups)
}
