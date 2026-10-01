package goquality

import (
	"context"
	"os"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

func TestGenericArrayStorageUsesValueOwnership(t *testing.T) {
	for _, test := range []struct {
		name, constraint, body string
		want                   quality.Provenance
	}{
		{"array value", "~[1]int", "local := shared; local[0]++", quality.Local},
		{"direct array parameter", "~[1]int", "shared[0]++", quality.Local},
		{"slice of array copy", "~[1]int", "local := shared[:]; local[0]++", quality.Local},
		{"array union", "~[1]int | ~[2]int", "local := shared; local[0]++", quality.Local},
		{"named constraint", "ArraySet", "local := shared; local[0]++", quality.Local},
		{"constraint intersection", "interface { ArraySet; ~[1]int }", "local := shared; local[0]++", quality.Local},
		{"pointer to array", "~*[1]int", "local := shared; local[0]++", quality.External},
		{"slice", "~[]int", "local := shared; local[0]++", quality.External},
		{"mixed array and slice", "~[1]int | ~[]int", "local := shared; local[0]++", quality.External},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("package repro\ntype ArraySet interface { ~[1]int | ~[2]int }\nfunc Copy[T " + test.constraint + "](shared T) { " + test.body + " }\n")
			root := qualitySourceModule(t, source)
			facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			f := findFunction(t, facts, "Copy")
			if len(f.Mutations) != 1 || f.Mutations[0].Provenance != test.want {
				t.Fatalf("mutations = %+v, want %s", f.Mutations, test.want)
			}
		})
	}
}

func TestReducedGenericArrayCopyIsEffectFree(t *testing.T) {
	source, err := os.ReadFile("testdata/generic-array-copy/reduced.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	root := qualitySourceModule(t, source)
	facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Functions) != 1 || !report.Functions[0].Complete || len(report.Functions[0].Effects) != 0 {
		t.Fatalf("array copy incorrectly effectful: %+v", report.Functions)
	}
}

func TestParenthesizedAllocationsKeepOwnedStorage(t *testing.T) {
	for _, body := range []string{
		"local := (new)(int); *local = 1",
		"local := ((new))(int); *local = 1",
		"local := (make)([]int, 1); local[0]++",
		"local := ((make))(map[int]int); local[0]++",
	} {
		t.Run(body, func(t *testing.T) {
			root := qualitySourceModule(t, []byte("package repro\nfunc Owned() { "+body+" }\n"))
			facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			f := findFunction(t, facts, "Owned")
			if len(f.Mutations) != 1 || f.Mutations[0].Provenance != quality.Local {
				t.Fatalf("owned allocation lost: %+v", f.Mutations)
			}
		})
	}
}
