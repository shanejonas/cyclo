package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"

	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/domain"
)

type expected struct {
	name    string
	literal *ast.FuncLit
	set     *token.FileSet
}

// Invalid Go and inputs without the target initializer cannot reproduce a
// mismatch. Both conditions are checked again for every reduction candidate.
func parseTargets(source []byte) ([]expected, bool) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "candidate.go", source, 0)
	if err != nil {
		return nil, false
	}
	config := types.Config{}
	_, err = config.Check("p", set, []*ast.File{file}, nil)
	if err != nil {
		return nil, false
	}
	var targets []expected
	for _, decl := range file.Decls {
		group, ok := decl.(*ast.GenDecl)
		if ok {
			targets = append(targets, groupTargets(group, set)...)
		}
	}
	return targets, len(targets) > 0
}

func groupTargets(group *ast.GenDecl, set *token.FileSet) []expected {
	var targets []expected
	for _, item := range group.Specs {
		spec, ok := item.(*ast.ValueSpec)
		if ok {
			targets = append(targets, valueTargets(spec, set)...)
		}
	}
	return targets
}

func valueTargets(spec *ast.ValueSpec, set *token.FileSet) []expected {
	var targets []expected
	for index, value := range spec.Values {
		literal, ok := ast.Unparen(value).(*ast.FuncLit)
		if ok && index < len(spec.Names) {
			targets = append(targets, expected{name: spec.Names[index].Name, literal: literal, set: set})
		}
	}
	return targets
}

func analyze(path string, source []byte) (domain.Report, error) {
	err := os.WriteFile(path, source, 0600)
	if err != nil {
		return domain.Report{}, err
	}
	return gocyclo.NewAnalyzer().Analyze([]string{path})
}

func check(path string, source []byte) (string, error) {
	targets, valid := parseTargets(source)
	if !valid {
		return "", nil
	}
	baselineSource, err := namedBaseline(targets)
	if err != nil {
		return "", err
	}
	baseline, err := analyze(filepath.Join(filepath.Dir(path), "baseline.go"), baselineSource)
	if err != nil {
		return "", fmt.Errorf("analyze baseline: %w", err)
	}
	actual, err := analyze(path, source)
	if err != nil {
		return "", fmt.Errorf("analyze candidate: %w", err)
	}
	return compareReports(targets, source, baseline, actual), nil
}

func namedBaseline(targets []expected) ([]byte, error) {
	file := &ast.File{Name: ast.NewIdent("p")}
	for _, target := range targets {
		file.Decls = append(file.Decls, &ast.FuncDecl{Name: ast.NewIdent(target.name), Type: target.literal.Type, Body: target.literal.Body})
	}
	var output bytes.Buffer
	err := format.Node(&output, targets[0].set, file)
	return output.Bytes(), err
}

func compareReports(targets []expected, source []byte, baseline, actual domain.Report) string {
	if actual.Functions != len(targets) {
		return "function-count"
	}
	functions := functionsByName(actual)
	if len(functions) != actual.Functions {
		return "duplicate-function-name"
	}
	reference := functionsByName(baseline)
	for _, target := range targets {
		function, ok := functions[target.name]
		if !ok {
			return "function-name:" + target.name
		}
		failure := compareFunction(target, source, reference[target.name], function)
		if failure != "" {
			return failure + ":" + target.name
		}
	}
	return ""
}

func functionsByName(report domain.Report) map[string]domain.Function {
	result := map[string]domain.Function{}
	for _, file := range report.Files {
		for _, function := range file.Functions {
			result[function.Name] = function
		}
	}
	return result
}

func compareFunction(target expected, source []byte, baseline, function domain.Function) string {
	if function.Complexity != baseline.Complexity || function.CognitiveComplexity != baseline.CognitiveComplexity {
		return "equivalent-function-score"
	}
	return compareLocations(target, source, function)
}

func compareLocations(target expected, source []byte, function domain.Function) string {
	start := target.set.PositionFor(target.literal.Pos(), false)
	end := target.set.PositionFor(target.literal.End(), false)
	if function.Line != start.Line || function.Column != start.Column || function.EndLine != end.Line {
		return "physical-source-location"
	}
	if function.Source != string(source[start.Offset:end.Offset]) {
		return "function-source"
	}
	return compareDiagnosticTotals(function)
}

func compareDiagnosticTotals(function domain.Function) string {
	if len(function.CyclomaticDiagnostics) != function.Complexity {
		return "cyclomatic-diagnostic-total"
	}
	total := 0
	for _, diagnostic := range function.CognitiveDiagnostics {
		total += diagnostic.Increment
	}
	if total != function.CognitiveComplexity {
		return "cognitive-diagnostic-total"
	}
	return ""
}
