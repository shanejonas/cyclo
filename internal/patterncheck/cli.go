// Package patterncheck is the IO boundary for cyclo's patterns miner: it
// proposes latent shared abstractions (interfaces, params structs, generics)
// found by structural mining. It is informational only: it always exits 0 on
// success and never feeds the quality gate. Pattern facts live apart from
// quality facts and are never passed to quality.Evaluate.
package patterncheck

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/internal/gitchanged"
)

const usage = `Usage: cyclo patterns [OPTIONS] [DIRECTORIES OR GO FILES...]

Mine the codebase for latent shared abstractions: structurally parallel
functions that suggest interfaces, params structs, or generics.
Informational only: always exits 0, never a quality gate.
  --format FORMAT   text (default) or json
  --threshold N     minimum WL similarity (0-1000) for clustering [default 600]
  --changed         only report candidates with a site touched by the git diff
  --base REF        git base for --changed (default: merge-base with main/master, else HEAD)

Exit 0: always, on success (even with no candidates). Exit 2: extraction or
analysis failure.
`

type options struct {
	format    string
	threshold uint
	base      string
	changed   bool
	paths     []string
}

// Run extracts PDGs for paths, mines them for abstraction candidates, and
// writes the report. It returns nil on success regardless of how many (or
// how few) candidates the miner finds; only extraction, analysis, or IO
// failures are errors.
func Run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	report, err := mine(ctx, opts)
	if err != nil {
		return err
	}
	return writePatterns(output, opts.format, report)
}

// mine extracts PDGs, runs the miner, and narrows to the diff when --changed
// is set.
func mine(ctx context.Context, opts options) (patterns.PatternsReport, error) {
	pdgs, err := gopatterns.Extract(ctx, "", opts.paths)
	if err != nil {
		return patterns.PatternsReport{}, err
	}
	params := patterns.DefaultParams()
	params.ThresholdMilli = uint32(opts.threshold)
	report := patterns.Run(toFacts(pdgs), patterns.Options{Params: params})
	if opts.changed {
		return onlyChanged(opts, report)
	}
	return report, nil
}

// writePatterns renders the report in the requested format.
func writePatterns(output io.Writer, format string, report patterns.PatternsReport) error {
	if format == "json" {
		out, err := patterns.JSON(&report)
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, out+"\n")
		return err
	}
	_, err := io.WriteString(output, patterns.Text(&report)+"\n")
	return err
}

// onlyChanged keeps only candidates with at least one site whose body the
// git diff touches. The full codebase is mined first (so a changed function
// can match an unchanged one); only the reported candidates are narrowed.
func onlyChanged(opts options, report patterns.PatternsReport) (patterns.PatternsReport, error) {
	ranges, root, cwd, err := diffRanges(opts)
	if err != nil {
		return report, err
	}
	kept := report.Candidates[:0]
	for _, candidate := range report.Candidates {
		if candidateTouchesDiff(candidate, ranges, root, cwd) {
			kept = append(kept, candidate)
		}
	}
	report.Candidates = kept
	return report, nil
}

// diffRanges resolves the git root, base, and changed line ranges for --changed.
func diffRanges(opts options) (map[string][]gitchanged.LineRange, string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", "", err
	}
	root, err := gitchanged.GitRoot(cwd)
	if err != nil {
		return nil, "", "", err
	}
	base, err := gitchanged.ResolveBase(root, opts.base)
	if err != nil {
		return nil, "", "", err
	}
	ranges, err := gitchanged.ChangedRanges(root, base, gitchanged.RootSpecs(opts.paths, root, cwd))
	if err != nil {
		return nil, "", "", err
	}
	return ranges, root, cwd, nil
}

// candidateTouchesDiff reports whether any site's body overlaps a diff touch
// range in the same file.
func candidateTouchesDiff(candidate patterns.Candidate, ranges map[string][]gitchanged.LineRange, root, cwd string) bool {
	for _, site := range candidate.Sites {
		rel := gitchanged.RootRelative(site.Path, root, cwd)
		for _, r := range ranges[rel] {
			if gitchanged.Overlaps(site.Line, site.EndLine, r) {
				return true
			}
		}
	}
	return false
}
// from the parameter type classes, so functions with the same shape of
// signature block together in sigmine.
// toFacts converts extracted PDGs to miner facts. The signature key derives
// from the parameter type classes, so functions with the same shape of
// signature block together in sigmine.
func toFacts(pdgs []gopatterns.FuncPdg) []*patterns.FuncFacts {
	facts := make([]*patterns.FuncFacts, 0, len(pdgs))
	for _, fp := range pdgs {
		var params []string
		for _, node := range fp.Pdg.Nodes {
			if node.Kind == patterns.Param {
				params = append(params, node.TyClass)
			}
		}
		fp := fp
		facts = append(facts, &patterns.FuncFacts{
			ID:      fp.Name,
			Name:    fp.Name,
			Path:    fp.Path,
			Line:    fp.Line,
			EndLine: fp.EndLine,
			Pdg:     &fp.Pdg,
			SigKey:  "fn(" + strings.Join(params, ",") + ")",
		})
	}
	return facts
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("patterns", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format")
	threshold := flags.Uint("threshold", 600, "minimum WL similarity (0-1000)")
	changed := flags.Bool("changed", false, "only candidates with a diff-touched site")
	base := flags.String("base", "", "git base for --changed")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	result := options{format: *format, threshold: *threshold, changed: *changed, base: *base, paths: flags.Args()}
	return result, result.validate()
}

func (opts options) validate() error {
	if !slices.Contains([]string{"text", "json"}, opts.format) {
		return fmt.Errorf("format must be text or json")
	}
	if opts.threshold > 1000 {
		return fmt.Errorf("threshold must be between 0 and 1000")
	}
	return nil
}
