package patterns

import (
	"encoding/json"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

// TestProfileMatchingRoute excludes corpus decoding from CPU and allocation
// snapshots. Profile runs are separate from the unprofiled timing runs.
func TestProfileMatchingRoute(t *testing.T) {
	prefix := os.Getenv("CYCLO_PROFILE_PREFIX")
	if prefix == "" {
		t.Skip("opt-in matching profile")
	}
	data, err := os.ReadFile(os.Getenv("CYCLO_MATCH_CORPUS"))
	if err != nil {
		t.Fatal(err)
	}
	var facts []*MiningFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		t.Fatal(err)
	}
	cache := &baseLabelCache{hashes: map[baseLabelKey]uint64{}}
	for _, f := range facts {
		for _, n := range f.Pdg.Nodes {
			cache.add(n)
		}
		cache.attach(f.Pdg)
	}
	graphs, names, asts := ccGraphInputs(facts)
	runtime.GC()
	writeMatchingHeap(t, prefix+"-before.heap")
	file, err := os.Create(prefix + ".cpu")
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	passes := 0
	var groups [][]string
	for time.Since(start) < 3*time.Second {
		groups = CCGraphClonesWithAST(graphs, names, asts)
		passes++
	}
	pprof.StopCPUProfile()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	writeMatchingHeap(t, prefix+"-after.heap")
	t.Logf("profile passes=%d groups=%d", passes, len(groups))
	runtime.KeepAlive(facts)
	runtime.KeepAlive(cache)
	runtime.KeepAlive(groups)
}

func writeMatchingHeap(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.WriteHeapProfile(file); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestMeasureWlStorage compares identical execution-aware WL computations.
// Full mode retains temporary adjacency and rounds; light mode discards them.
func TestMeasureWlStorage(t *testing.T) {
	mode := os.Getenv("CYCLO_WL_STORAGE")
	if mode == "" {
		t.Skip("opt-in WL storage")
	}
	if mode != "light" && mode != "full" {
		t.Fatal("unknown WL storage mode")
	}
	data, err := os.ReadFile(os.Getenv("CYCLO_MATCH_CORPUS"))
	if err != nil {
		t.Fatal(err)
	}
	var facts []*MiningFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		t.Fatal(err)
	}
	graphs, _, _ := ccGraphInputs(facts)
	ids := ccSortableIDs(graphs)
	cache := &baseLabelCache{hashes: map[baseLabelKey]uint64{}}
	for _, id := range ids {
		for _, n := range graphs[id].Nodes {
			cache.add(n)
		}
		cache.attach(graphs[id])
	}
	runtime.GC()
	var before, built, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	wls := make([]*Wl, len(ids))
	for i, id := range ids {
		graph := buildProfileGraph(graphs[id], true)
		d := diameter(&graph)
		rounds := graph.refineRoundsDiameter(d)
		wls[i] = &Wl{hists: histograms(rounds), diameter: d, nodeCount: len(graph.labels), calls: countCalls(graphs[id])}
		if mode == "full" {
			wls[i].graph = graph
			wls[i].rounds = rounds
			wls[i].pdg = graphs[id]
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&built)
	runtime.GC()
	runtime.ReadMemStats(&after)
	report := map[string]any{"mode": mode, "functions": len(ids), "milliseconds": float64(elapsed.Microseconds()) / 1000, "allocated_bytes": built.TotalAlloc - before.TotalAlloc, "allocations": built.Mallocs - before.Mallocs, "retained_heap_bytes": int64(after.HeapAlloc) - int64(before.HeapAlloc)}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
	runtime.KeepAlive(facts)
	runtime.KeepAlive(graphs)
	runtime.KeepAlive(cache)
	runtime.KeepAlive(wls)
}
