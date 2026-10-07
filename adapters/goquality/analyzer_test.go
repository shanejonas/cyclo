package goquality

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

var updateGolden = flag.Bool("update", false, "update quality report golden")

func sampleFacts(t *testing.T, tests bool) []quality.Function {
	t.Helper()
	facts, err := (Analyzer{Root: "testdata/sample", Tests: tests}).Extract(context.Background(), []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func findFunction(t *testing.T, facts []quality.Function, name string) quality.Function {
	t.Helper()
	for _, fact := range facts {
		if strings.HasSuffix(fact.Name, "."+name) {
			return fact
		}
	}
	t.Fatalf("function %s missing", name)
	return quality.Function{}
}

func TestTypedWritesAndAliasProvenance(t *testing.T) {
	facts := sampleFacts(t, false)
	writes := findFunction(t, facts, "Writes")
	want := []struct {
		root       string
		provenance quality.Provenance
	}{
		{"local", quality.Local}, {"local", quality.Local}, {"b", quality.External},
		{"alias", quality.External}, {"owned", quality.Local}, {"data", quality.External},
		{"m", quality.External}, {"a", quality.Local}, {"m", quality.External},
		{"local", quality.Local}, {"local", quality.Local},
	}
	if len(writes.Mutations) != len(want) {
		t.Fatalf("mutations: %+v", writes.Mutations)
	}
	for index, expected := range want {
		actual := writes.Mutations[index]
		if actual.Root != expected.root || actual.Provenance != expected.provenance {
			t.Fatalf("mutation %d: %+v, want %+v", index, actual, expected)
		}
	}
	if writes.Mutations[0].RootID == writes.Mutations[9].RootID {
		t.Fatal("shadowed binding merged")
	}
	if writes.Mutations[0].RootID != writes.Mutations[10].RootID {
		t.Fatal("closure loses captured root")
	}
	if writes.Mutations[2].FieldPath != "Count" {
		t.Fatal("field path missing")
	}
	if len(writes.Effects) != 1 || writes.Effects[0].Kind != quality.Global {
		t.Fatalf("global effects: %+v", writes.Effects)
	}
	aliases := findFunction(t, facts, "Aliases")
	if aliases.Mutations[1].Provenance != quality.Unknown {
		t.Fatalf("branch alias should be unknown: %+v", aliases.Mutations)
	}
	if aliases.Mutations[2].Provenance != quality.Local || aliases.Mutations[3].Provenance != quality.External {
		t.Fatalf("buffer/copy provenance: %+v", aliases.Mutations)
	}
}

func TestCallsUseTypesRatherThanMutatorNames(t *testing.T) {
	facts := sampleFacts(t, false)
	calls := findFunction(t, facts, "Calls")
	if len(calls.Mutations) != 0 || len(calls.Calls) != 2 {
		t.Fatalf("calls: %+v", calls)
	}
	if calls.Calls[0].Callee != "example.com/sample.Box.Read" || calls.Calls[0].Dynamic {
		t.Fatalf("resolved method: %+v", calls.Calls)
	}
	dynamic := findFunction(t, facts, "Dynamic")
	if !dynamic.Calls[0].Dynamic || dynamic.Calls[0].Callee != "io.Reader.Read" || !dynamic.Calls[1].Dynamic {
		t.Fatalf("dynamic calls: %+v", dynamic.Calls)
	}
	writes := findFunction(t, facts, "Writes")
	if writes.Calls[0].Callee != "fmt.Println" || writes.Calls[0].Line != 36 {
		t.Fatalf("alias/call site: %+v", writes.Calls[0])
	}
}

func TestFixtureGoldenAndDoubleRun(t *testing.T) {
	first := sampleFacts(t, true)
	second := sampleFacts(t, true)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("extractor is non-deterministic")
	}
	report, err := quality.Evaluate(first, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := "testdata/sample/report.golden.json"
	if *updateGolden {
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(golden) != string(encoded) {
		t.Fatalf("quality report differs from %s; inspect then use -update", path)
	}
	for _, fact := range first {
		if strings.HasSuffix(fact.Name, ".Generated") || strings.HasSuffix(fact.Name, ".Tagged") {
			t.Fatalf("excluded function included: %s", fact.Name)
		}
	}
	if len(first) != len(sampleFacts(t, false))+1 {
		t.Fatal("test variants are not deduplicated")
	}
}

func TestSelectionTagsAndIncompletePackages(t *testing.T) {
	facts, err := (Analyzer{Root: "testdata/sample", Tests: true, BuildFlags: []string{"-tags=cyclo_fixture"}}).Extract(context.Background(), []string{"ignored.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || !strings.HasSuffix(facts[0].Name, ".Tagged") {
		t.Fatalf("file selection: %+v", facts)
	}
	root := t.TempDir()
	for name, source := range map[string]string{"go.mod": "module broken\n\ngo 1.25.0\n", "broken.go": "package broken\nfunc Broken() { missing() }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (Analyzer{Root: root}).Extract(context.Background(), nil); err == nil {
		t.Fatal("type error produces successful analysis")
	}
}

func TestCodeLinesIgnoreCommentsAndPreserveStrings(t *testing.T) {
	source := "func f() {\n// comment\n/* block\ncomment */\n\ntext := `// value\n/* value */`\n_ = text // comment\n}\n"
	if got := codeLines(source); got != 5 {
		t.Fatalf("code lines = %d, want 5", got)
	}
}

func TestTupleAliasesAndTemporaryTargetsStayVisible(t *testing.T) {
	f := findFunction(t, sampleFacts(t, false), "TupleAndTemporary")
	if len(f.Mutations) != 3 {
		t.Fatalf("mutations: %+v", f.Mutations)
	}
	if f.Mutations[0].Provenance != quality.Unknown {
		t.Fatal("tuple return incorrectly considered local")
	}
	if f.Mutations[1].Root != "<temporary>" || f.Mutations[1].Provenance != quality.Unknown {
		t.Fatal("temporary write missing")
	}
	if f.Mutations[2].Root != "values" || f.Mutations[2].Provenance != quality.External {
		t.Fatal("slice projection loses root/provenance")
	}
}

func TestUnsafeUsesTypesAndExcludesSizeof(t *testing.T) {
	f := findFunction(t, sampleFacts(t, false), "Unsafe")
	if len(f.Effects) != 2 {
		t.Fatalf("unsafe effects: %+v", f.Effects)
	}
	for _, effect := range f.Effects {
		if effect.Kind != quality.Unsafe {
			t.Fatalf("effect: %+v", effect)
		}
	}
}

func TestSelfCheckProducesValidReport(t *testing.T) {
	if testing.Short() {
		t.Skip("package-wide self-check")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	facts, err := (Analyzer{Root: root}).Extract(context.Background(), []string{"domain/quality", "adapters/goquality", "application/qualitycheck"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Functions < 20 || report.Summary.IncompleteFunctions == 0 {
		t.Fatalf("self-check report: %+v", report.Summary)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Actual <= diagnostic.Limit {
			t.Fatalf("invalid self-check diagnostic: %+v", diagnostic)
		}
	}
}
