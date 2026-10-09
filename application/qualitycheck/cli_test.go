package qualitycheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

func writeTemp(t *testing.T, name, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func savedFacts(t *testing.T) string {
	t.Helper()
	facts := FactsFile{1, []quality.Function{{Location: quality.Location{Path: "input.go", Line: 1, Column: 1, Name: "Write"}, Statements: 3, CodeLines: 4, Params: 5, Effects: []quality.Effect{{Kind: quality.IO, Detail: "write", Line: 2}}}}}
	encoded, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	return writeTemp(t, "facts.json", string(encoded))
}

func TestSavedFactsCanBeReevaluatedWithoutCompiler(t *testing.T) {
	path := savedFacts(t)
	var first, second bytes.Buffer
	args := []string{"--facts-in", path, "--format", "json"}
	for _, output := range []*bytes.Buffer{&first, &second} {
		if err := Run(context.Background(), args, output); !errors.Is(err, ErrFindings) {
			t.Fatalf("check error: %v", err)
		}
	}
	if first.String() != second.String() || strings.Contains(first.String(), t.TempDir()) {
		t.Fatal("non-deterministic output")
	}
	config := writeTemp(t, "cyclo.toml", "[fn_params]\nmax = 5\n[side_effect_density]\nmax = 1000\n")
	var output bytes.Buffer
	if err := Run(context.Background(), append(args, "--config", config), &output); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDefaultsOverridesAndValidation(t *testing.T) {
	path := writeTemp(t, "cyclo.toml", "granularity = 'field'\n[weights]\nio = 0\n[[prefixes]]\npath = 'net/http.Get'\nkind = 'none'\n")
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Granularity != "field" || config.Weights.IO != 0 || config.Weights.Network != 3 || config.FnLength.Max != 200 {
		t.Fatalf("config: %+v", config)
	}
	for _, source := range []string{"typo = 1", "[fn_params]\nmax = -1", "[[prefixes]]\npath = 'foo'\nkind = 'typo'", "granularity = 'typo'"} {
		if _, err := LoadConfig(writeTemp(t, "bad.toml", source)); err == nil {
			t.Fatalf("invalid config accepted: %s", source)
		}
	}
}

func TestCLIHelpAndInvalidInput(t *testing.T) {
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"--help"}, &output); err != nil || !strings.Contains(output.String(), "Usage: cyclo check") {
		t.Fatalf("help: %v %s", err, output.String())
	}
	for _, args := range [][]string{{"--format", "bad"}, {"--facts-in", "missing", "."}, {"--facts-in", "missing", "--tests"}} {
		if err := Run(context.Background(), args, &output); err == nil {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
	for _, source := range []string{"{}", "{\"schema_version\":3,\"functions\":[]}", "{\"schema_version\":1,\"functions\":[],\"typo\":true}", "{\"schema_version\":1,\"functions\":[]} {}"} {
		if _, err := readFacts(writeTemp(t, "bad.json", source)); err == nil {
			t.Fatalf("invalid facts accepted: %s", source)
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestOutputErrorsAreOperationalFailures(t *testing.T) {
	err := Run(context.Background(), []string{"--facts-in", savedFacts(t)}, brokenWriter{})
	if err == nil || errors.Is(err, ErrFindings) {
		t.Fatalf("output error: %v", err)
	}
}

func TestMissingSavedFactsIsAnOperationalFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	var output bytes.Buffer
	err := Run(context.Background(), []string{"--facts-in", path, "--format", "json"}, &output)
	if !errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrFindings) {
		t.Fatalf("missing facts error: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("missing facts emits a report: %s", output.String())
	}
}

func TestFactsVersionsKeepHelperEvidenceAndLegacyUncertainty(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			function := quality.Function{Location: quality.Location{Path: "input.go", Line: 1, Column: 1, Name: "project.caller"}, Statements: 1, CodeLines: 1,
				Calls: []quality.Call{{Callee: "project.helper", Line: 1, Local: true}}}
			if version == 2 {
				function.Helpers = []quality.Helper{{Name: "project.helper"}}
			}
			encoded, err := json.Marshal(FactsFile{version, []quality.Function{function}})
			if err != nil {
				t.Fatal(err)
			}
			path := writeTemp(t, "facts.json", string(encoded))
			var output bytes.Buffer
			if err := Run(context.Background(), []string{"--facts-in", path, "--format", "json"}, &output); err != nil {
				t.Fatal(err)
			}
			var report quality.Report
			if err := json.Unmarshal(output.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Functions[0].Complete != (version == 2) {
				t.Fatalf("legacy uncertainty or helper evidence lost: %+v", report)
			}
			restored, err := readFacts(path)
			if err != nil || len(restored[0].Helpers) != len(function.Helpers) {
				t.Fatalf("helper evidence lost: %+v, %v", restored, err)
			}

		})
	}
}

func TestFactsExportIncludesSelectedFunctionHelperBodies(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"go.mod":    "module example.com/helpers\n\ngo 1.25.0\n",
		"caller.go": "package helpers\nfunc Caller() int { return helper(1) }\n",
		"helper.go": "package helpers\nfunc helper(value int) int { local := value; local++; return local }\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	var output bytes.Buffer
	if err := Run(context.Background(), []string{"--format", "facts", "caller.go"}, &output); err != nil {
		t.Fatal(err)
	}
	var exported FactsFile
	if err := json.Unmarshal(output.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.SchemaVersion != 2 || len(exported.Functions) != 1 || len(exported.Functions[0].Helpers) != 1 {
		t.Fatalf("invalid selected export: %+v", exported)
	}
	path := writeTemp(t, "roundtrip.json", output.String())
	output.Reset()
	if err := Run(context.Background(), []string{"--facts-in", path, "--format", "json"}, &output); err != nil {
		t.Fatal(err)
	}
	var report quality.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Functions[0].Complete || len(report.Functions[0].Effects) != 0 {
		t.Fatalf("exported pure helper not resolved: %+v", report)
	}
}

func TestWriteTextRendersFixGroups(t *testing.T) {
	report := quality.Report{
		Summary: quality.Summary{Functions: 3},
		FixGroups: []quality.FixGroup{
			{ID: 1, Stacked: true, Functions: []quality.GroupFunction{
				{Location: quality.Location{Path: "b.go", Line: 10, Column: 1, Name: "mod.callee"}, Rules: []string{"fn_params"}},
				{Location: quality.Location{Path: "a.go", Line: 1, Column: 1, Name: "mod.caller"}, Rules: []string{"fn_length"}},
			}},
			{ID: 2, Functions: []quality.GroupFunction{
				{Location: quality.Location{Path: "c.go", Line: 5, Column: 1, Name: "mod.solo"}, Rules: []string{"side_effect_density"}},
			}},
		},
	}
	var output strings.Builder
	if err := writeText(&output, report); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"fix groups: 2 (1 stacked, 1 independent)",
		"group 1 (2 functions; land together or stack in this order):",
		"b.go:10:1 mod.callee [fn_params]",
		"a.go:1:1 mod.caller [fn_length]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text output missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "group 2 (") {
		t.Errorf("independent group should be counted, not listed:\n%s", text)
	}
}

func TestGroupingConfigTOML(t *testing.T) {
	config, err := LoadConfig(writeTemp(t, "cyclo.toml", "[grouping]\ncaller_rules = ['fn_params', 'fn_length']\ncaller_hops = 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Grouping.CallerRules) != 2 || config.Grouping.CallerHops != 2 {
		t.Fatalf("grouping config: %+v", config.Grouping)
	}
	defaults, err := LoadConfig(writeTemp(t, "empty.toml", ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.Grouping.CallerRules) != 1 || defaults.Grouping.CallerRules[0] != "fn_params" || defaults.Grouping.CallerHops != 1 {
		t.Fatalf("grouping defaults: %+v", defaults.Grouping)
	}
	for _, source := range []string{"[grouping]\ncaller_rules = ['fn_param']", "[grouping]\ncaller_hops = -1"} {
		if _, err := LoadConfig(writeTemp(t, "bad.toml", source)); err == nil {
			t.Fatalf("invalid grouping config accepted: %s", source)
		}
	}
}
