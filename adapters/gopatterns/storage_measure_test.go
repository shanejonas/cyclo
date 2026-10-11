package gopatterns

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
	"unsafe"

	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/domain/pdg"
)

type storageStage struct {
	Name                        string
	Milliseconds                float64
	AllocatedBytes, Allocations uint64
	RetainedHeapBytes           int64
}

func storageMeasure(name string, work func()) storageStage {
	runtime.GC()
	var before, built, retained runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	work()
	elapsed := time.Since(start)
	runtime.ReadMemStats(&built)
	runtime.GC()
	runtime.ReadMemStats(&retained)
	return storageStage{name, float64(elapsed.Microseconds()) / 1000, built.TotalAlloc - before.TotalAlloc, built.Mallocs - before.Mallocs, int64(retained.HeapAlloc) - int64(before.HeapAlloc)}
}

// TestMeasureGraphStorage keeps typed inputs, native graphs and matching views
// live through separate GCs. Stages measure their marginal retained heap.
func TestMeasureGraphStorage(t *testing.T) {
	root := os.Getenv("CYCLO_STORAGE_ROOT")
	if root == "" {
		t.Skip("opt-in graph storage measurement")
	}
	started := time.Now()
	pkgs, err := loadPatternPackages(context.Background(), root, []string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	loadMS := float64(time.Since(started).Microseconds()) / 1000
	prefix := os.Getenv("CYCLO_STORAGE_PROFILE")
	var profileFile *os.File
	if prefix != "" {
		writeStorageHeap(t, prefix+"-before.heap")
		profileFile, err = os.Create(prefix + ".cpu")
		if err != nil {
			t.Fatal(err)
		}
	}
	var graphs []pdg.Graph
	native := storageMeasure("native_graphs", func() {
		if profileFile != nil {
			if err := pprof.StartCPUProfile(profileFile); err != nil {
				t.Fatal(err)
			}
		}
		graphs, err = memoryGraphs(pkgs)
		if profileFile != nil {
			pprof.StopCPUProfile()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if profileFile != nil {
		if err := profileFile.Close(); err != nil {
			t.Fatal(err)
		}
		writeStorageHeap(t, prefix+"-after.heap")
	}
	var views []*patterns.MiningGraph
	projection := storageMeasure("mining_views", func() {
		views = make([]*patterns.MiningGraph, len(graphs))
		for i := range graphs {
			views[i] = patterns.MiningView(&graphs[i])
		}
	})
	var cloned []*patterns.MiningGraph
	cloning := storageMeasure("canonicalization_no_rules", func() {
		cloned = make([]*patterns.MiningGraph, len(views))
		for i, v := range views {
			cloned[i] = patterns.Canonicalize(v, patterns.Rules{})
		}
	})
	var normalized []*patterns.MiningGraph
	normalization := storageMeasure("canonicalization_all_rules", func() {
		normalized = make([]*patterns.MiningGraph, len(views))
		for i, v := range views {
			normalized[i] = patterns.Canonicalize(v, patterns.RulesAll)
		}
	})
	functions, nodes, edges := memoryCounts(graphs)
	report := map[string]any{"functions": functions, "nodes": nodes, "edges": edges, "loader_ms": loadMS, "stages": []storageStage{native, projection, cloning, normalization}, "node_bytes": unsafe.Sizeof(pdg.Node{}), "edge_bytes": unsafe.Sizeof(pdg.Edge{}), "mining_node_bytes": unsafe.Sizeof(patterns.PdgNode{}), "mining_edge_bytes": unsafe.Sizeof(patterns.PdgEdge{})}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	runtime.KeepAlive(pkgs)
	runtime.KeepAlive(graphs)
	runtime.KeepAlive(views)
	runtime.KeepAlive(cloned)
	runtime.KeepAlive(normalized)
}

func writeStorageHeap(t *testing.T, path string) {
	t.Helper()
	runtime.GC()
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
