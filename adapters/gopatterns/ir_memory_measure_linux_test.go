package gopatterns

import (
	"context"
	"encoding/json"
	"go/ast"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/shanejonas/cyclo/domain/pdg"
	"golang.org/x/tools/go/packages"
)

// TestMeasurePDGMemory is opt-in. Run the compiled test binary in a fresh
// process for each mode so peak RSS and process globals cannot cross runs.
func TestMeasurePDGMemory(t *testing.T) {
	mode := os.Getenv("CYCLO_PDG_MEMORY_MODE")
	if mode == "" {
		t.Skip("opt-in real-corpus measurement")
	}
	root, err := absRoot(os.Getenv("CYCLO_PDG_MEMORY_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	var pkgs []*packages.Package
	if mode == "graph-new" {
		pkgs, err = loadPatternPackages(context.Background(), root, []string{"./..."})
		if err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var before, built, retained runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	result, err := memoryResult(mode, root, pkgs)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	runtime.ReadMemStats(&built)
	runtime.GC()
	runtime.ReadMemStats(&retained)
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		t.Fatal(err)
	}
	functions, nodes, edges := memoryCounts(result)
	output := map[string]any{
		"mode": mode, "root": root, "go_version": runtime.Version(),
		"functions": functions, "nodes": nodes, "edges": edges,
		"retained_heap_bytes":  int64(retained.HeapAlloc) - int64(before.HeapAlloc),
		"allocated_bytes":      built.TotalAlloc - before.TotalAlloc,
		"allocations":          built.Mallocs - before.Mallocs,
		"elapsed_ms":           float64(elapsed.Microseconds()) / 1000,
		"peak_process_rss_kib": usage.Maxrss,
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	// Both inputs and output remain live through the measured GC. Graph modes
	// isolate graph construction from the shared typed-package loader.
	runtime.KeepAlive(pkgs)
	runtime.KeepAlive(result)
}

func memoryResult(mode, root string, pkgs []*packages.Package) (any, error) {
	switch mode {
	case "graph-new":
		return memoryGraphs(pkgs)
	case "api-new":
		return Extract(context.Background(), root, []string{"."})
	default:
		return nil, &memoryModeError{mode}
	}
}

type memoryModeError struct{ mode string }

func (e *memoryModeError) Error() string { return "unknown memory mode: " + e.mode }

func memoryGraphs(pkgs []*packages.Package) ([]pdg.Graph, error) {
	pool := pdg.NewBuilder()
	var graphs []pdg.Graph
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			path := pkg.Fset.PositionFor(file.Pos(), false).Filename
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				graph, err := buildGraph(pkg, fn, path, pool)
				if err != nil {
					return nil, err
				}
				graphs = append(graphs, graph)
			}
		}
	}
	return graphs, nil
}

func memoryCounts(result any) (functions, nodes, edges int) {
	switch graphs := result.(type) {
	case []pdg.Graph:
		functions = len(graphs)
		for _, g := range graphs {
			nodes += len(g.Nodes)
			edges += len(g.Edges)
		}
	case *Extraction:
		functions = len(graphs.Funcs)
		for _, g := range graphs.Funcs {
			nodes += len(g.Pdg.Nodes)
			edges += len(g.Pdg.Edges)
		}
	}
	return
}

// TestValidatePDGCorpus checks the full IR outside the timed measurements.
func TestValidatePDGCorpus(t *testing.T) {
	root := os.Getenv("CYCLO_PDG_VALIDATE_ROOT")
	if root == "" {
		t.Skip("opt-in corpus validation")
	}
	pkgs, err := loadPatternPackages(context.Background(), root, []string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	graphs, err := memoryGraphs(pkgs)
	if err != nil {
		t.Fatal(err)
	}
	for i := range graphs {
		if err := pdg.Validate(&graphs[i]); err != nil {
			t.Fatalf("%s: %v", graphs[i].Tables.Text(graphs[i].Function.ID), err)
		}
	}
	t.Logf("validated %d canonical graphs", len(graphs))
}
