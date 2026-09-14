package main

import (
	"path/filepath"
	"testing"

	"github.com/shanejonas/cyclo/domain"
)

func TestCheckerRejectsInvalidOrMissingTarget(t *testing.T) {
	for _, source := range []string{"package p", "package p; var handle = func() { if missing {} }", "package p; var handle = func() {"} {
		_, valid := parseTargets([]byte(source))
		if valid {
			t.Fatalf("accepted %q", source)
		}
	}
}

func TestOracleDetectsHistoricalFailures(t *testing.T) {
	source := []byte("package p\n//line imaginary.go:100\nvar handle = (func() {})\n")
	targets, valid := parseTargets(source)
	if !valid {
		t.Fatal("invalid fixture")
	}
	target := targets[0]
	start := target.set.PositionFor(target.literal.Pos(), false)
	end := target.set.PositionFor(target.literal.End(), false)
	baseline := domain.Function{Name: "handle", Complexity: 1}
	correct := domain.Function{Name: "handle", Complexity: 1, Line: start.Line, Column: start.Column, EndLine: end.Line, Source: string(source[start.Offset:end.Offset]), CyclomaticDiagnostics: []domain.CyclomaticDiagnostic{{Kind: "function", Line: start.Line, Column: start.Column}}}
	for _, test := range []struct {
		name   string
		change func(*domain.Report)
	}{
		{"function-count", func(r *domain.Report) { r.Functions = 0 }},
		{"function-name:handle", func(r *domain.Report) { r.Files[0].Functions[0].Name = "number" }},
		{"physical-source-location:handle", func(r *domain.Report) { r.Files[0].Functions[0].Line = 100 }},
		{"equivalent-function-score:handle", func(r *domain.Report) { r.Files[0].Functions[0].CognitiveComplexity = 1 }},
	} {
		report := domain.Report{Functions: 1, Files: []domain.File{{Functions: []domain.Function{correct}}}}
		if got := compareReports(targets, source, domain.Report{Files: []domain.File{{Functions: []domain.Function{baseline}}}}, report); got != "" {
			t.Fatalf("correct report failed: %s", got)
		}
		test.change(&report)
		if got := compareReports(targets, source, domain.Report{Files: []domain.File{{Functions: []domain.Function{baseline}}}}, report); got != test.name {
			t.Fatalf("got %s, want %s", got, test.name)
		}
	}
}

func TestMultipleFunctionOracleMatchesNamesNotReportOrder(t *testing.T) {
	source := []byte("package p\nvar idle, branch = func() {}, func() { if true {} }\n")
	targets, valid := parseTargets(source)
	if !valid {
		t.Fatal("invalid source")
	}
	baselineSource, err := namedBaseline(targets)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	baseline, err := analyze(filepath.Join(directory, "baseline.go"), baselineSource)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := analyze(filepath.Join(directory, "candidate.go"), source)
	if err != nil {
		t.Fatal(err)
	}
	functions := actual.Files[0].Functions
	functions[0], functions[1] = functions[1], functions[0]
	if got := compareReports(targets, source, baseline, actual); got != "" {
		t.Fatalf("report order caused failure: %s", got)
	}
	functions[0].Name, functions[1].Name = functions[1].Name, functions[0].Name
	if got := compareReports(targets, source, baseline, actual); got != "equivalent-function-score:idle" {
		t.Fatalf("swapped names not detected: %s", got)
	}
	functions[0].Name, functions[1].Name = "idle", "idle"
	if got := compareReports(targets, source, baseline, actual); got != "duplicate-function-name" {
		t.Fatalf("duplicate names not detected: %s", got)
	}
}
