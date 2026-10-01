package goquality

import (
	"context"
	"path/filepath"

	"github.com/shanejonas/cyclo/domain"
	"github.com/shanejonas/cyclo/domain/quality"
)

type ComplexityAnalyzer interface {
	Analyze([]string) (domain.Report, error)
}

// ReportAnalyzer augments the existing syntax-only scan. Typed failures are
// report data, so code that is being edited can still be inspected in the TUI.
type ReportAnalyzer struct {
	Complexity ComplexityAnalyzer
	Config     quality.Config
	Tests      bool
	BuildFlags []string
}

func (a ReportAnalyzer) Analyze(paths []string) (domain.Report, error) {
	report, err := a.Complexity.Analyze(paths)
	if err != nil {
		return report, err
	}
	facts, result, err := a.qualityReport(report, paths)
	if err != nil {
		report.Quality = &domain.QualityAnalysis{Status: "error", Error: err.Error()}
		return report, nil
	}
	report.Quality = &domain.QualityAnalysis{Status: "ready", Report: &result}
	attachQuality(&report, facts, result)
	return report, nil
}

func (a ReportAnalyzer) qualityReport(report domain.Report, paths []string) ([]quality.Function, quality.Report, error) {
	inputs := make([]string, len(paths))
	for index, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, quality.Report{}, err
		}
		inputs[index] = absolute
	}
	extractor := Analyzer{Root: report.Root, Tests: a.Tests, BuildFlags: a.BuildFlags}
	facts, err := extractor.Extract(context.Background(), inputs)
	if err != nil {
		return nil, quality.Report{}, err
	}
	result, err := quality.Evaluate(facts, a.Config)
	return facts, result, err
}

type functionPosition struct {
	path         string
	line, column int
}

func attachQuality(report *domain.Report, facts []quality.Function, result quality.Report) {
	byPosition := qualityByPosition(facts, result)
	for fileIndex := range report.Files {
		file := &report.Files[fileIndex]
		path, err := relativePath(report.Root, file.Path)
		if err != nil {
			continue
		}
		for index := range file.Functions {
			function := &file.Functions[index]
			function.Quality = byPosition[functionPosition{path, function.Line, function.Column}]
		}
	}
}

func qualityByPosition(facts []quality.Function, result quality.Report) map[functionPosition]*domain.FunctionQuality {
	positions := map[functionPosition]*domain.FunctionQuality{}
	for index, function := range result.Functions {
		key := functionPosition{function.Path, function.Line, function.Column}
		positions[key] = &domain.FunctionQuality{FunctionResult: function, Diagnostics: []quality.Diagnostic{}, MutationEvents: facts[index].Mutations}
	}
	for _, diagnostic := range result.Diagnostics {
		key := functionPosition{diagnostic.Path, diagnostic.Line, diagnostic.Column}
		function := positions[key]
		function.Diagnostics = append(function.Diagnostics, diagnostic)
	}
	return positions
}
