package main

import (
	"io"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"
)

// TestProfilePatternCLI profiles the complete read-only patterns command.
func TestProfilePatternCLI(t *testing.T) {
	prefix := os.Getenv("CYCLO_CLI_PROFILE")
	if prefix == "" {
		t.Skip("opt-in complete CLI profile")
	}
	t.Chdir(os.Getenv("CYCLO_CLI_ROOT"))
	runtime.GC()
	writeCLIHeap(t, prefix+"-before.heap")
	file, err := os.Create(prefix + ".cpu")
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = run([]string{"patterns", "--format", "json", "."}, io.Discard)
	elapsed := time.Since(start)
	pprof.StopCPUProfile()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	writeCLIHeap(t, prefix+"-after.heap")
	t.Logf("complete CLI profile wall_ms=%.3f", float64(elapsed.Microseconds())/1000)
}

func writeCLIHeap(t *testing.T, path string) {
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
