// Package patternfix is the IO boundary for cyclo fix: static auto-fix
// for patterns miner candidates. No LLM, no tokens — pure AST mechanics.
// Dry-run by default (prints a diff); --apply writes files.
//
// Unified pipeline: runs the miner, gets candidates with FixSpecs, and
// applies each fixer's transform. No re-detection.
package patternfix

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
	"github.com/shanejonas/cyclo/domain/patterns"
)

const usage = `Usage: cyclo fix [OPTIONS] [DIRECTORIES OR GO FILES...]

Statically apply fixes for patterns miner candidates. No LLM, no tokens:
pure AST rewrites that are provably behavior-preserving.
Runs the miner, then applies each candidate's fix.
Dry-run by default (shows a diff); --apply writes the files.

  --kind KIND   which fixes to apply: guard_clause, value_object, parameterize,
                trait_method, capability_set, enum_dispatch, generic_fn,
                anemic_model, primitive_obsession, or all (default)
  --apply       write the fixes to disk (default: dry-run diff only)

Exit 0: always, on success (even with no fixes). Exit 2: parse or IO failure.
`

type options struct {
	kind  string
	apply bool
	paths []string
}

// Run runs the miner, then applies fixes for candidates with FixSpecs.
func Run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	specs, err := collectFixSpecs(ctx, opts)
	if err != nil {
		return err
	}
	fixed, err := applyFixSpecs(ctx, specs, opts, output)
	if err != nil {
		return err
	}
	if !opts.apply {
		fmt.Fprintf(output, "%d fixable candidate(s) (dry-run; use --apply to write)\n", fixed)
	}
	return nil
}

// collectFixSpecs runs the miner and returns FixSpecs filtered by kind.
func collectFixSpecs(ctx context.Context, opts options) ([]*patterns.FixSpec, error) {
	ext, err := gopatterns.Extract(ctx, "", opts.paths)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	facts, anemicHits := toFacts(ext)
	report := patterns.Run(facts, patterns.Options{AnemicModels: anemicHits})
	var specs []*patterns.FixSpec
	for _, c := range report.Candidates {
		if c.FixSpec == nil {
			continue
		}
		if opts.kind != "all" && string(c.Kind) != opts.kind {
			continue
		}
		specs = append(specs, c.FixSpec)
	}
	return specs, nil
}

// applyFixSpecs groups specs by file and applies them.
func applyFixSpecs(ctx context.Context, specs []*patterns.FixSpec, opts options, output io.Writer) (int, error) {
	byFile := groupByFile(specs)
	var fixed int
	for file, fileSpecs := range byFile {
		if err := ctx.Err(); err != nil {
			return fixed, err
		}
		n, err := fixFileWithSpecs(file, fileSpecs, opts, output)
		if err != nil {
			return fixed, err
		}
		fixed += n
	}
	return fixed, nil
}

// groupByFile groups FixSpecs by their file.
func groupByFile(specs []*patterns.FixSpec) map[string][]*patterns.FixSpec {
	byFile := make(map[string][]*patterns.FixSpec)
	for _, s := range specs {
		byFile[s.File] = append(byFile[s.File], s)
	}
	return byFile
}

// fixFileWithSpecs applies FixSpecs to a file.
func fixFileWithSpecs(path string, specs []*patterns.FixSpec, opts options, output io.Writer) (int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	out, applied := applySpecsToSource(path, specs, src)
	if applied == 0 {
		return 0, nil
	}
	if opts.apply {
		return writeFixed(path, out, applied, output)
	}
	return showDiff(path, src, out, applied, output)
}

// applySpecsToSource applies each spec, skipping failures.
func applySpecsToSource(path string, specs []*patterns.FixSpec, src []byte) ([]byte, int) {
	out := src
	var applied int
	for _, spec := range specs {
		fixed, err := gopatterns.ApplyFix(spec, out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", path, err)
			continue
		}
		if string(fixed) != string(out) {
			applied++
			out = fixed
		}
	}
	return out, applied
}

// writeFixed writes the fixed source and reports.
func writeFixed(path string, out []byte, applied int, output io.Writer) (int, error) {
	if err := os.WriteFile(path, out, 0644); err != nil {
		return 0, err
	}
	fmt.Fprintf(output, "fixed %d candidate(s) in %s\n", applied, path)
	return applied, nil
}

// showDiff prints a unified diff for dry-run.
func showDiff(path string, src, out []byte, n int, output io.Writer) (int, error) {
	diff, err := unifiedDiff(path, src, out)
	if err != nil {
		fmt.Fprintf(output, "%s: would fix %d candidate(s)\n", path, n)
		return n, nil
	}
	io.WriteString(output, diff)
	return n, nil
}

// unifiedDiff returns a unified diff of old -> new via diff -u.
func unifiedDiff(path string, old, new []byte) (string, error) {
	oldFile, err := os.CreateTemp("", "cyclo-fix-old-*.go")
	if err != nil {
		return "", err
	}
	defer os.Remove(oldFile.Name())
	newFile, err := os.CreateTemp("", "cyclo-fix-new-*.go")
	if err != nil {
		return "", err
	}
	defer os.Remove(newFile.Name())
	if _, err := oldFile.Write(old); err != nil {
		return "", err
	}
	if _, err := newFile.Write(new); err != nil {
		return "", err
	}
	oldFile.Close()
	newFile.Close()

	cmd := exec.Command("diff", "-u", "--label", "a/"+path, "--label", "b/"+path, oldFile.Name(), newFile.Name())
	out, _ := cmd.Output()
	return string(out), nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("fix", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "all", "which fixes to apply")
	apply := flags.Bool("apply", false, "write fixes to disk")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	result := options{kind: *kind, apply: *apply, paths: flags.Args()}
	return result, result.validate()
}

func (opts options) validate() error {
	valid := []string{"guard_clause", "value_object", "parameterize", "trait_method", "capability_set", "enum_dispatch", "generic_fn", "anemic_model", "primitive_obsession", "all"}
	if !slices.Contains(valid, opts.kind) {
		return fmt.Errorf("kind must be one of %v", valid)
	}
	return nil
}

// toFacts converts extraction to FuncFacts (copied from patterncheck).
func toFacts(ext *gopatterns.Extraction) ([]*patterns.FuncFacts, []patterns.AnemicModelHit) {
	var facts []*patterns.FuncFacts
	for _, fp := range ext.Funcs {
		fp := fp
		var guards []patterns.GuardClauseHit
		for _, g := range fp.GuardClauses {
			guards = append(guards, patterns.GuardClauseHit{
				Line:      g.Line,
				BodyStmts: g.BodyStmts,
			})
		}
		var dispatches []patterns.EnumDispatchHit
		for _, d := range fp.EnumDispatches {
			dispatches = append(dispatches, patterns.EnumDispatchHit{
				Line:     d.Line,
				NumCases: d.NumCases,
			})
		}
		var tss []patterns.TypeSwitchHit
		for _, t := range fp.TypeSwitches {
			tss = append(tss, patterns.TypeSwitchHit{
				Line:   t.Line,
				Bound:  t.Bound,
				Expr:   t.Expr,
				Method: t.Method,
				Types:  t.Types,
				Args:   t.Args,
			})
		}
		var params []patterns.ParamInfo
		for _, p := range fp.Params {
			params = append(params, patterns.ParamInfo{
				Name: p.Name,
				Type: p.Type,
			})
		}
		facts = append(facts, &patterns.FuncFacts{
			ID:             fp.Name,
			Name:           fp.Name,
			Path:           fp.Path,
			Line:           fp.Line,
			EndLine:        fp.EndLine,
			Pdg:            &fp.Pdg,
			GuardClauses:   guards,
			EnumDispatches: dispatches,
			TypeSwitches:   tss,
			Params:         params,
		})
	}
	return facts, ext.AnemicModels
}
