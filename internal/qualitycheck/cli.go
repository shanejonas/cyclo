// Package qualitycheck is the IO boundary for Cyclo's headless quality checks.
package qualitycheck

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/shanejonas/cyclo/adapters/goquality"
	"github.com/shanejonas/cyclo/domain/quality"
)

var ErrFindings = errors.New("quality guardrails exceeded")

const usage = `Usage: cyclo check [OPTIONS] [DIRECTORIES OR GO FILES...]

Extract typed Go facts and evaluate code quality guardrails. Defaults to .
  --config PATH     TOML policy; unspecified values retain defaults
  --format FORMAT   text (default), json, or facts (save extraction only)
  --facts-in PATH   evaluate saved facts without loading Go packages
  --tests           include tests (default false)
  --tags TAGS       Go build tags

Exit 0: no findings (or facts exported); 1: findings; 2: analysis/config/IO failure.
Effects include syntactic operations and conservative same-package helper summaries.
Unclassified calls are unknown, not pure. Facts export version 2; version 1 remains readable.
`

type options struct {
	config, format, factsIn, tags string
	tests                         bool
	paths                         []string
}

type FactsFile struct {
	SchemaVersion int                `json:"schema_version"`
	Functions     []quality.Function `json:"functions"`
}

func Run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		_, err = io.WriteString(output, usage)
		return err
	}
	if err != nil {
		return err
	}
	return execute(ctx, opts, output)
}

func execute(ctx context.Context, opts options, output io.Writer) error {
	config, err := LoadConfig(opts.config)
	if err != nil {
		return err
	}
	facts, err := inputFacts(ctx, opts)
	if err != nil {
		return err
	}
	if opts.format == "facts" {
		return writeJSON(output, FactsFile{2, facts})
	}
	return evaluateFacts(output, facts, config, opts.format)
}

func evaluateFacts(output io.Writer, facts []quality.Function, config quality.Config, format string) error {
	report, err := quality.Evaluate(facts, config)
	if err != nil {
		return err
	}
	if format == "json" {
		err = writeJSON(output, report)
	} else {
		err = writeText(output, report)
	}
	if err != nil {
		return err
	}
	if len(report.Diagnostics) > 0 {
		return ErrFindings
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	result := options{}
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&result.config, "config", "", "TOML config")
	flags.StringVar(&result.format, "format", "text", "output format")
	flags.StringVar(&result.factsIn, "facts-in", "", "saved facts")
	flags.StringVar(&result.tags, "tags", "", "Go build tags")
	flags.BoolVar(&result.tests, "tests", false, "include tests")
	if err := flags.Parse(args); err != nil {
		return result, err
	}
	result.paths = flags.Args()
	return result, result.validate()
}

func (opts options) validate() error {
	if !slices.Contains([]string{"text", "json", "facts"}, opts.format) {
		return fmt.Errorf("format must be text, json, or facts")
	}
	if opts.factsIn != "" && opts.hasSourceOptions() {
		return fmt.Errorf("facts-in cannot be combined with source paths, tests, tags, or facts export")
	}
	return nil
}

func (opts options) hasSourceOptions() bool {
	return len(opts.paths) > 0 || opts.tests || opts.tags != "" || opts.format == "facts"
}

func inputFacts(ctx context.Context, opts options) ([]quality.Function, error) {
	if opts.factsIn != "" {
		return readFacts(opts.factsIn)
	}
	analyzer := goquality.Analyzer{Tests: opts.tests}
	if opts.tags != "" {
		analyzer.BuildFlags = []string{"-tags=" + opts.tags}
	}
	return analyzer.Extract(ctx, opts.paths)
}

func readFacts(path string) ([]quality.Function, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open facts: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var facts FactsFile
	if err := decoder.Decode(&facts); err != nil {
		return nil, fmt.Errorf("decode facts: %w", err)
	}
	if !supportedFactsVersion(facts.SchemaVersion) || facts.Functions == nil {
		return nil, fmt.Errorf("facts require schema_version 1 or 2 and a functions array")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("facts must contain exactly one JSON document")
	}
	return facts.Functions, nil
}

func writeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeText(output io.Writer, report quality.Report) error {
	var text strings.Builder
	for _, diagnostic := range report.Diagnostics {
		fmt.Fprintf(&text, "%s:%d:%d: %s: %s\n", diagnostic.Path, diagnostic.Line, diagnostic.Column, diagnostic.Name, diagnostic.Message)
		for _, effect := range diagnostic.Effects {
			fmt.Fprintf(&text, "  line %d: %s: %s\n", effect.Line, effect.Kind, effect.Detail)
		}
		for _, mutation := range diagnostic.Mutations {
			fmt.Fprintf(&text, "  line %d: mutation: %s.%s (%s)\n", mutation.Line, mutation.Root, mutation.FieldPath, mutation.Provenance)
		}
	}
	fmt.Fprintf(&text, "%d functions; %d findings; mean/max density %d/%d milli; %d functions with unknown effects\n", report.Summary.Functions, len(report.Diagnostics), report.Summary.MeanDensityMilli, report.Summary.MaxDensityMilli, report.Summary.IncompleteFunctions)
	_, err := io.WriteString(output, text.String())
	return err
}

func supportedFactsVersion(version int) bool { return version == 1 || version == 2 }
