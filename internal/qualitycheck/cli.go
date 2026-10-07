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
  --changed         only report findings in functions touched by the git diff
  --base REF        git base for --changed (default: merge-base with main/master, else HEAD)

Exit 0: no findings (or facts exported); 1: findings; 2: analysis/config/IO failure.
Effects include syntactic operations and conservative same-package helper summaries.
Unclassified calls are unknown, not pure. Facts export version 2; version 1 remains readable.
`

type options struct {
	config, format, factsIn, tags, base string
	tests, changed                      bool
	paths                               []string
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
	return checkFacts(output, opts, facts, config)
}

// checkFacts evaluates facts, optionally narrows the report to diff-touched
// functions, writes it, and maps remaining findings to ErrFindings.
func checkFacts(output io.Writer, opts options, facts []quality.Function, config quality.Config) error {
	report, err := quality.Evaluate(facts, config)
	if err != nil {
		return err
	}
	if opts.changed {
		report, err = onlyChanged(opts, facts, report, config)
		if err != nil {
			return err
		}
	}
	if err := writeReport(output, report, opts.format); err != nil {
		return err
	}
	if len(report.Diagnostics) > 0 {
		return ErrFindings
	}
	return nil
}

// onlyChanged filters the report to diagnostics in functions the git diff
// touches. Facts are evaluated whole so helper summaries stay complete;
// only the reported findings are narrowed.
func onlyChanged(opts options, facts []quality.Function, report quality.Report, config quality.Config) (quality.Report, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return report, err
	}
	root, err := gitRoot(cwd)
	if err != nil {
		return report, err
	}
	base, err := resolveBase(root, opts.base)
	if err != nil {
		return report, err
	}
	ranges, err := changedRanges(root, base, rootSpecs(opts.paths, root, cwd))
	if err != nil {
		return report, err
	}
	return filterChanged(report, facts, touchedFunctions(facts, ranges, root, cwd), config)
}

// rootSpecs converts CLI paths (relative to cwd) to repo-root-relative git
// pathspecs, defaulting to the whole tree.
func rootSpecs(paths []string, root, cwd string) []string {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	specs := make([]string, len(paths))
	for index, path := range paths {
		specs[index] = rootRelative(path, root, cwd)
	}
	return specs
}

func writeReport(output io.Writer, report quality.Report, format string) error {
	if format == "json" {
		return writeJSON(output, report)
	}
	return writeText(output, report)
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
	flags.BoolVar(&result.changed, "changed", false, "only findings in diff-touched functions")
	flags.StringVar(&result.base, "base", "", "git base for --changed")
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
	if len(report.FixGroups) > 0 {
		writeFixGroups(&text, report.FixGroups)
	}
	fmt.Fprintf(&text, "%d functions; %d findings; mean/max density %d/%d milli; %d functions with unknown effects\n", report.Summary.Functions, len(report.Diagnostics), report.Summary.MeanDensityMilli, report.Summary.MaxDensityMilli, report.Summary.IncompleteFunctions)
	_, err := io.WriteString(output, text.String())
	return err
}

// writeFixGroups renders the work plan: which violations must be fixed
// together and which are independent.
func writeFixGroups(text *strings.Builder, groups []quality.FixGroup) {
	stacked := 0
	for _, group := range groups {
		if group.Stacked {
			stacked++
		}
	}
	fmt.Fprintf(text, "fix groups: %d (%d stacked, %d independent)\n", len(groups), stacked, len(groups)-stacked)
	for _, group := range groups {
		if !group.Stacked {
			continue
		}
		fmt.Fprintf(text, "  group %d (%d functions; land together or stack in this order):\n", group.ID, len(group.Functions))
		for _, function := range group.Functions {
			fmt.Fprintf(text, "    %s:%d:%d %s [%s]\n", function.Path, function.Line, function.Column, function.Name, strings.Join(function.Rules, ", "))
		}
	}
}

func supportedFactsVersion(version int) bool { return version == 1 || version == 2 }
