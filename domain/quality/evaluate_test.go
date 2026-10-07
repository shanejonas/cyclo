package quality

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func fact(name string, line int) Function {
	return Function{Location: Location{Path: "sample.go", Line: line, Column: 1, Name: name}, Statements: 4, CodeLines: 6, Mutations: []Mutation{}, Calls: []Call{}, Effects: []Effect{}}
}

func evaluate(t *testing.T, f Function, c Config) Report {
	t.Helper()
	report, err := Evaluate([]Function{f}, c)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestMutationBudgetsAndProvenance(t *testing.T) {
	f := fact("churn", 1)
	for _, field := range []string{"a", "b", "c", "d"} {
		f.Mutations = append(f.Mutations, Mutation{Root: "self", FieldPath: field, Line: 2, Provenance: External})
	}
	c := DefaultConfig()
	report := evaluate(t, f, c)
	if report.Functions[0].DensityMilli != 1000 || report.Functions[0].MutatedTargets != 1 {
		t.Fatalf("result: %+v", report)
	}
	if report.Diagnostics[0].RuleID != "mutation_per_target" || len(report.Diagnostics[0].Mutations) != 4 {
		t.Fatalf("diagnostics: %+v", report.Diagnostics)
	}
	c.Granularity = "field"
	report = evaluate(t, f, c)
	if report.Diagnostics[0].RuleID != "mutated_targets" || report.Functions[0].MutatedTargets != 4 {
		t.Fatalf("field result: %+v", report)
	}
	for index := range f.Mutations {
		f.Mutations[index].Provenance = Local
	}
	report = evaluate(t, f, c)
	if report.Functions[0].DensityMilli != 0 || report.Functions[0].Mutations != 4 {
		t.Fatalf("local mutation: %+v", report)
	}
}

func TestPurePrefixesClassifyAsNone(t *testing.T) {
	pure := []string{
		"strings.Repeat", "strconv.Itoa", "unicode.IsSpace", "unicode/utf8.RuneLen",
		"cmp.Compare", "slices.Concat", "maps.Keys", "math.Max", "sort.Strings",
		"errors.Is", "path.Join", "path/filepath.Base", "path/filepath.Join",
		"charm.land/lipgloss/v2.Style.Render", "github.com/charmbracelet/lipgloss.NewStyle",
		"github.com/charmbracelet/x/ansi.StringWidth", "error.Error",
		"go/types.Object.Pkg", "go/types.Type.Underlying", "go/types.Info.ObjectOf",
		"go/types.Selection.Index",
	}
	f := fact("calls", 1)
	for index, callee := range pure {
		f.Calls = append(f.Calls, Call{Callee: callee, Line: index + 2})
	}
	report := evaluate(t, f, DefaultConfig())
	result := report.Functions[0]
	if len(result.Effects) != 0 || result.DensityMilli != 0 || !result.Complete {
		t.Fatalf("pure calls should vanish: %+v", result)
	}
	// Narrow classifications still win where packages mix pure and effectful
	// calls, and unknown callees still count.
	f = fact("calls", 1)
	f.Calls = []Call{
		{Callee: "math/rand.Int", Line: 2},
		{Callee: "fmt.Println", Line: 3},
		{Callee: "path/filepath.Abs", Line: 4},
		{Callee: "mystery", Line: 5},
	}
	report = evaluate(t, f, DefaultConfig())
	result = report.Functions[0]
	if len(result.Effects) != 4 || result.UnclassifiedCalls != 1 {
		t.Fatalf("mixed classification: %+v", result)
	}
	kinds := map[Kind]int{}
	for _, effect := range result.Effects {
		kinds[effect.Kind]++
	}
	if kinds[Random] != 1 || kinds[IO] != 2 || kinds[UnknownEffect] != 1 {
		t.Fatalf("effect kinds: %+v", result.Effects)
	}
}

func TestPureStdlibCallsScoreZero(t *testing.T) {
	f := fact("pure", 1)
	f.Statements = 10
	for index, callee := range []string{
		"strings.Repeat", "bytes.Compare", "slices.Contains", "maps.Keys",
		"strconv.Itoa", "sort.Strings", "errors.Is", "math/big.NewInt",
		"encoding/json.Marshal", "context.Background", "sync.Mutex.Lock",
		"path/filepath.Join", "net/url.Parse",
		"net/http.ResponseWriter.Header", "flag.FlagSet.Args", "flag.FlagSet.Lookup",
	} {
		f.Calls = append(f.Calls, Call{Callee: callee, Line: index + 2})
	}
	report := evaluate(t, f, DefaultConfig())
	for _, diagnostic := range report.Diagnostics {
		t.Fatalf("pure stdlib finding: %+v", diagnostic)
	}
	if report.Functions[0].DensityMilli != 0 {
		t.Fatalf("density: %+v", report.Functions[0])
	}
}

func TestRealEffectsStillFail(t *testing.T) {
	f := fact("effectful", 1)
	f.Statements = 10
	for index, callee := range []string{
		"os.Open", "net/http.Get", "log/slog.Info",
		"path/filepath.Abs", "path/filepath.WalkDir",
		"flag.FlagSet.Parse", "flag.FlagSet.StringVar",
		"net/http.ResponseWriter.WriteHeader",
	} {
		f.Calls = append(f.Calls, Call{Callee: callee, Line: index + 2})
	}
	report := evaluate(t, f, DefaultConfig())
	var density *Diagnostic
	for i, diagnostic := range report.Diagnostics {
		if diagnostic.RuleID == "side_effect_density" {
			density = &report.Diagnostics[i]
		}
	}
	if density == nil {
		t.Fatal("expected a side_effect_density finding")
	}
	// weight 3*7+2=23 over 10 statements = 2300 milli > 500.
	if density.Actual != 2300 {
		t.Fatalf("actual = %d", density.Actual)
	}
	// Every call classified: no unknown weight remains.
	if density.KindWeights["unknown"] != 0 {
		t.Fatalf("kind weights = %+v", density.KindWeights)
	}
}

func TestDensityFindingExplainsItself(t *testing.T) {
	f := fact("mixed", 1)
	f.Statements = 10
	f.Calls = []Call{
		{Callee: "os.Open", Line: 2},
		{Callee: "mystery", Line: 3},
		{Callee: "mystery", Line: 4},
	}
	f.Mutations = []Mutation{
		{Root: "x", RootID: "1:1", Line: 5, Provenance: External},
	}
	report := evaluate(t, f, DefaultConfig())
	var density *Diagnostic
	for i, diagnostic := range report.Diagnostics {
		if diagnostic.RuleID == "side_effect_density" {
			density = &report.Diagnostics[i]
		}
	}
	if density == nil {
		t.Fatal("expected a side_effect_density finding")
	}
	// weight 3 (io) + 1 + 1 (unknown) + 1 (mutation) = 6 over 10 statements.
	if density.Weight != 6 || density.Statements != 10 {
		t.Fatalf("weight/statements = %d/%d", density.Weight, density.Statements)
	}
	wantCounts := map[string]int64{"io": 3, "unknown": 2, "mutation": 1}
	if !reflect.DeepEqual(density.KindWeights, wantCounts) {
		t.Fatalf("kind weights = %v", density.KindWeights)
	}
	want := "side_effect_density 600 > 500: weight 6 over 10 statements (io 3, unknown 2, mutation 1)"
	if density.Message != want {
		t.Fatalf("message = %q, want %q", density.Message, want)
	}
}

func TestClassificationLongestPrefixAndUnknownCoverage(t *testing.T) {
	f := fact("calls", 1)
	f.Calls = []Call{{Callee: "net/http.Get", Line: 2}, {Callee: "fmt.Sprintf", Line: 3}, {Callee: "os/exec.Command", Line: 4}, {Callee: "os/exec.Cmd.Output", Line: 5}, {Callee: "project.save", Local: true, Line: 6}, {Callee: "callback", Dynamic: true, Line: 7}}
	c := DefaultConfig()
	c.Prefixes = append(c.Prefixes, Prefix{"net/http.Get", None})
	report := evaluate(t, f, c)
	result := report.Functions[0]
	if result.Complete || result.UnclassifiedCalls != 2 || result.DensityMilli != 1250 {
		t.Fatalf("result: %+v", result)
	}
	if len(result.Effects) != 3 || result.Effects[0].Kind != IO {
		t.Fatalf("effects: %+v", result.Effects)
	}
	if NormalizeCallee("Vec::<T, A>::push") != "Vec::push" || NormalizeCallee("p.Box[T].Set") != "p.Box.Set" {
		t.Fatal("generic normalization")
	}
}

func TestCalleeNormalizationPreservesPlainAndMalformedNames(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"fmt.Println", "fmt.Println"},
		{"pkg.語", "pkg.語"},
		{"Vec::::push", "Vec::push"},
		{"pkg.Box[T].Set", "pkg.Box.Set"},
		{"Vec::<T, A>::push", "Vec::push"},
		{"pkg.Call]", "pkg.Call"},
		{"pkg.Call[T", "pkg.Call"},
		{"pkg.\xffCall", "pkg.\uFFFDCall"},
		{"\xff\xfe", "\uFFFD\uFFFD"},
	} {
		if got := NormalizeCallee(tc.input); got != tc.want {
			t.Fatalf("NormalizeCallee(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestStrictLimitsAndMinimumStatements(t *testing.T) {
	f := fact("limits", 1)
	f.Params, f.CodeLines, f.Statements = 4, 50, 2
	f.Effects = []Effect{{Kind: IO, Detail: "write", Line: 2}}
	c := DefaultConfig()
	if len(evaluate(t, f, c).Diagnostics) != 0 {
		t.Fatal("limits are strict and minimum statements applies")
	}
	f.Params, f.CodeLines, f.Statements, f.HasSelf = 5, 51, 3, true
	report := evaluate(t, f, c)
	if len(report.Diagnostics) != 3 {
		t.Fatalf("diagnostics: %+v", report.Diagnostics)
	}
	c.CountSelf = true
	report = evaluate(t, f, c)
	if report.Diagnostics[1].Actual != 6 {
		t.Fatalf("receiver not counted: %+v", report.Diagnostics)
	}
	f.Effects = []Effect{{Kind: Panic, Detail: "panic", Line: 3}}
	if evaluate(t, f, c).Functions[0].DensityMilli != 0 {
		t.Fatal("panic weight defaults to zero")
	}
}

func TestSuppressionsRequireReasonAndKnownRules(t *testing.T) {
	f := fact("wide", 1)
	f.Params = 10
	for _, comment := range []string{"// cyclo-allow(fn_params)", "// cyclo-allow(fn_params): ", "// cyclo-allow(typo): legacy", "// cyclo-allow(invalid_suppression): legacy", "// cyclo-allow(): legacy"} {
		f.PrecedingLine = comment
		report := evaluate(t, f, DefaultConfig())
		if len(report.Diagnostics) != 2 || report.Diagnostics[1].RuleID != "invalid_suppression" {
			t.Fatalf("%s: %+v", comment, report.Diagnostics)
		}
	}
	f.PrecedingLine = "// cyclo-allow(fn_params, fn_length): stable public API"
	if len(evaluate(t, f, DefaultConfig()).Diagnostics) != 0 {
		t.Fatal("valid suppression rejected")
	}
}

func TestDeterminismDeduplicationAndInputImmutability(t *testing.T) {
	a, b := fact("a", 10), fact("b", 1)
	a.Params, b.Params = 8, 9
	a.Effects = []Effect{{Kind: IO, Detail: "late", Line: 12}, {Kind: Time, Detail: "early", Line: 11}}
	facts := []Function{a, b, a}
	before, _ := json.Marshal(facts)
	first, err := Evaluate(facts, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(facts)
	second, err := Evaluate(facts, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	one, _ := json.Marshal(first)
	two, _ := json.Marshal(second)
	if string(one) != string(two) || first.Summary.Functions != 2 {
		t.Fatal("non-deterministic or duplicate output")
	}
	slices.Reverse(facts)
	after, _ := json.Marshal(facts)
	if string(before) != string(after) {
		t.Fatal("evaluator mutates input")
	}
	conflict := a
	conflict.Params++
	if _, err := Evaluate([]Function{a, conflict}, DefaultConfig()); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
}

func TestShadowedRootsRemainDistinct(t *testing.T) {
	f := fact("shadow", 1)
	f.Mutations = []Mutation{{Root: "x", RootID: "2:1", Line: 3, Provenance: Local}, {Root: "x", RootID: "4:1", Line: 5, Provenance: Local}}
	if evaluate(t, f, DefaultConfig()).Functions[0].MutatedTargets != 2 {
		t.Fatal("shadowed roots merged")
	}
}

func TestInvalidBoundaries(t *testing.T) {
	f := fact("valid", 1)
	for _, path := range []string{"/tmp/a.go", "../a.go", "a/../b.go", "a\\b.go"} {
		f.Path = path
		if _, err := Evaluate([]Function{f}, DefaultConfig()); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
	f = fact("valid", 1)
	f.Mutations = []Mutation{{Root: "x", Line: 1, Provenance: "guess"}}
	if _, err := Evaluate([]Function{f}, DefaultConfig()); err == nil {
		t.Fatal("invalid provenance accepted")
	}
	c := DefaultConfig()
	c.Weights.IO = -1
	if _, err := Evaluate(nil, c); err == nil {
		t.Fatal("negative weight accepted")
	}
	c = DefaultConfig()
	c.Prefixes = append(c.Prefixes, Prefix{"p", Kind("typo")})
	if _, err := Evaluate(nil, c); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestMeanUsesExactIntegerMath(t *testing.T) {
	result := summarize([]FunctionResult{{DensityMilli: 1}, {DensityMilli: 2}, {DensityMilli: 8}})
	if result.MeanDensityMilli != 3 || result.MaxDensityMilli != 8 {
		t.Fatal(result)
	}
	large := int64(8_000_000_000_000_000_000)
	if summarize([]FunctionResult{{DensityMilli: large}, {DensityMilli: large}}).MeanDensityMilli != large {
		t.Fatal("mean overflows")
	}
	if !reflect.DeepEqual(summarize(nil).Effects[0], EffectCount{MutationEffect, 0}) {
		t.Fatal("summary lacks ordered effect counts")
	}
}
