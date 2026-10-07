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
	"strings"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/internal/gitchanged"
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
  --changed     only fix candidates in functions touched by the git diff
  --base REF    git base for --changed (default: merge-base with main/master)
  --phased      run fixes in dependency phases, re-mining changed files
                between phases (value_object fixes feed factory, etc.)

Exit 0: always, on success (even with no fixes). Exit 2: parse or IO failure.
`

type options struct {
	kind    string
	base    string
	apply   bool
	changed bool
	phased  bool
	paths   []string
}

// fixPhases groups pattern kinds by dependency level for phased fixing.
// Each phase runs after the previous phase's fixes are written to disk,
// then the next phase re-mines only the files that changed. This lets
// later phases see the results of earlier ones (e.g. value_object
// bundling params before factory builds constructors from them).
//
// The grouping mirrors FixKindOrder:
//  1. guard_clause + primitive_obsession: simplify control flow, create types
//  2. value_object: bundle params into the new types
//  3. factory + independent semantic fixes: use the bundled types
//  4. parameterize: PDG-based, most sensitive to code shape, runs last
var fixPhases = [][]patterns.CandidateKind{
	{patterns.GuardClause, patterns.PrimitiveObsession},
	{patterns.ValueObject},
	{patterns.Factory, patterns.MissingIdentity, patterns.EntityIdentity,
		patterns.AnemicModel, patterns.TypeSwitch, patterns.EnumDispatch,
		patterns.TraitMethod, patterns.CapabilitySet, patterns.GenericFn},
	{patterns.Parameterize},
}

// maxPhasedCycles bounds the fixpoint loop. In practice it terminates
// after 1-2 cycles; 3 is a safety net against oscillation.
const maxPhasedCycles = 3

// phasedState tracks the fixpoint loop state across phases.
type phasedState struct {
	dirty      []string
	firstCycle bool
	totalFixed int
}

// Run runs the miner, then applies fixes for candidates with FixSpecs.
func Run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if opts.phased {
		return runPhased(ctx, opts, output)
	}
	return runSinglePass(ctx, opts, output)
}

// runSinglePass is the original fix flow: mine once, apply all.
func runSinglePass(ctx context.Context, opts options, output io.Writer) error {
	specs, err := collectFixSpecs(ctx, opts)
	if err != nil {
		return err
	}
	fixed, err := applyFixSpecs(ctx, specs, opts, output)
	if err != nil {
		return err
	}
	reportDryRun(opts, output, fixed)
	return nil
}

// reportDryRun prints the dry-run summary.
func reportDryRun(opts options, output io.Writer, fixed int) {
	if opts.apply {
		return
	}
	if opts.changed {
		fmt.Fprintf(output, "%d fixable candidate(s) in changed functions (dry-run; use --apply to write)\n", fixed)
	} else {
		fmt.Fprintf(output, "%d fixable candidate(s) (dry-run; use --apply to write)\n", fixed)
	}
}

// runPhased applies fixes in dependency phases, re-mining changed files
// between phases. After each phase writes fixes, the next phase mines
// only those files, so it sees the updated code. Repeats until a full
// cycle produces no fixes or maxPhasedCycles is reached.
func runPhased(ctx context.Context, opts options, output io.Writer) error {
	if !opts.apply {
		return runPhasedDryRun(ctx, opts, output)
	}
	st := &phasedState{firstCycle: true}
	for cycle := 0; cycle < maxPhasedCycles; cycle++ {
		fixed, err := runPhasedCycle(ctx, opts, output, st)
		if err != nil {
			return err
		}
		st.totalFixed += fixed
		if fixed == 0 {
			break
		}
	}
	fmt.Fprintf(output, "phased fix: %d candidate(s) fixed\n", st.totalFixed)
	return nil
}

// runPhasedCycle runs one full pass through all phases. Returns the number
// of fixes applied; updates st.dirty with files written for the next cycle.
// Within a cycle, each phase mines all inputs for its kinds (we haven't
// checked those kinds yet). Across cycles, only re-mine changed files.
func runPhasedCycle(ctx context.Context, opts options, output io.Writer, st *phasedState) (int, error) {
	var cycleFixed int
	var cycleWritten []string
	for phaseIdx, phase := range fixPhases {
		if err := ctx.Err(); err != nil {
			return cycleFixed, err
		}
		minePaths := phasedMinePaths(opts, st, phaseIdx)
		if minePaths == nil {
			continue // nothing changed; skip
		}
		pp := phasedPhase{phase: phase, phaseIdx: phaseIdx, paths: minePaths}
		fixed, written, err := runPhasedPhase(ctx, opts, output, pp)
		if err != nil {
			return cycleFixed, err
		}
		cycleFixed += fixed
		cycleWritten = append(cycleWritten, written...)
	}
	st.dirty = dedupe(cycleWritten)
	st.firstCycle = false
	return cycleFixed, nil
}

// phasedMinePaths decides which files to mine for a phase.
// Returns nil to skip (nothing changed since last check).
// Within the first cycle, every phase mines all inputs for its kinds.
// In later cycles, only re-mine files written in the previous cycle.
// parameterize always mines everything (needs cross-file view).
func phasedMinePaths(opts options, st *phasedState, phaseIdx int) []string {
	phase := fixPhases[phaseIdx]
	// parameterize needs the full cross-file view for clustering.
	if phaseHas(phase, patterns.Parameterize) {
		return opts.paths
	}
	if st.firstCycle {
		return opts.paths
	}
	if len(st.dirty) == 0 {
		return nil
	}
	return st.dirty
}

// phasedPhase bundles the inputs for running one phase.
type phasedPhase struct {
	phase    []patterns.CandidateKind
	phaseIdx int
	paths    []string
}

// runPhasedPhase mines one phase's kinds and applies the fixes.
// Returns fixes applied and files written.
func runPhasedPhase(ctx context.Context, opts options, output io.Writer, pp phasedPhase) (int, []string, error) {
	specs, err := mineKinds(ctx, opts, pp.phase, pp.paths)
	if err != nil {
		return 0, nil, err
	}
	if len(specs) == 0 {
		return 0, nil, nil
	}
	fixed, written := applyAndTrack(ctx, specs, opts, output)
	fmt.Fprintf(output, "phase %d (%s): fixed %d candidate(s)\n",
		pp.phaseIdx+1, phaseNames(pp.phase), fixed)
	return fixed, written, nil
}

// runPhasedDryRun shows what phased fixing would do without writing.
// Single full mine, specs grouped and displayed in phase order.
func runPhasedDryRun(ctx context.Context, opts options, output io.Writer) error {
	specs, err := collectFixSpecs(ctx, opts)
	if err != nil {
		return err
	}
	byPhase, other := groupSpecsByPhase(specs)
	fixed, err := showPhasedSpecs(ctx, opts, output, byPhase, other)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "%d fixable candidate(s) across %d phases (dry-run; use --apply to write)\n", fixed, len(fixPhases))
	return nil
}

// groupSpecsByPhase buckets specs by their phase; unlisted kinds go to other.
func groupSpecsByPhase(specs []*patterns.FixSpec) ([][]*patterns.FixSpec, []*patterns.FixSpec) {
	byPhase := make([][]*patterns.FixSpec, len(fixPhases))
	var other []*patterns.FixSpec
	for _, s := range specs {
		if idx := phaseIndex(s.Kind); idx >= 0 {
			byPhase[idx] = append(byPhase[idx], s)
		} else {
			other = append(other, s)
		}
	}
	return byPhase, other
}

// phaseIndex returns the phase containing kind, or -1.
func phaseIndex(kind patterns.CandidateKind) int {
	for i, phase := range fixPhases {
		if phaseHas(phase, kind) {
			return i
		}
	}
	return -1
}

// showPhasedSpecs displays specs grouped by phase. Returns total shown.
func showPhasedSpecs(ctx context.Context, opts options, output io.Writer, byPhase [][]*patterns.FixSpec, other []*patterns.FixSpec) (int, error) {
	var fixed int
	for i, phaseSpecs := range byPhase {
		if len(phaseSpecs) == 0 {
			continue
		}
		fmt.Fprintf(output, "--- phase %d (%s) ---\n", i+1, phaseNames(fixPhases[i]))
		n, err := applyFixSpecs(ctx, phaseSpecs, opts, output)
		if err != nil {
			return fixed, err
		}
		fixed += n
	}
	if len(other) > 0 {
		n, err := applyFixSpecs(ctx, other, opts, output)
		if err != nil {
			return fixed, err
		}
		fixed += n
	}
	return fixed, nil
}

// phaseHas reports whether a phase includes a kind.
func phaseHas(phase []patterns.CandidateKind, kind patterns.CandidateKind) bool {
	for _, k := range phase {
		if k == kind {
			return true
		}
	}
	return false
}

// phaseNames joins kind names for display.
func phaseNames(phase []patterns.CandidateKind) string {
	names := make([]string, len(phase))
	for i, k := range phase {
		names[i] = string(k)
	}
	return strings.Join(names, ",")
}

// mineKinds mines candidates for specific kinds, limited to paths.
// An empty paths slice mines the input paths; nil mines everything the
// caller passed in opts.paths.
func mineKinds(ctx context.Context, opts options, kinds []patterns.CandidateKind, paths []string) ([]*patterns.FixSpec, error) {
	kindSet := make(map[patterns.CandidateKind]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}
	mineOpts := opts
	mineOpts.paths = paths
	candidates, err := mineCandidates(ctx, mineOpts)
	if err != nil {
		return nil, err
	}
	return filterKindSpecs(candidates, kindSet, opts)
}

// filterKindSpecs keeps FixSpecs for kinds in the set, applying --changed
// filtering when requested.
func filterKindSpecs(candidates []patterns.Candidate, kindSet map[patterns.CandidateKind]bool, opts options) ([]*patterns.FixSpec, error) {
	var specs []*patterns.FixSpec
	for _, c := range candidates {
		if !kindSet[c.Kind] {
			continue
		}
		kept, err := keepCandidateSpec(c, opts)
		if err != nil {
			return nil, err
		}
		specs = append(specs, kept...)
	}
	return specs, nil
}

// keepCandidateSpec returns the FixSpec for a candidate, applying --changed
// filtering. Returns empty when filtered out.
func keepCandidateSpec(c patterns.Candidate, opts options) ([]*patterns.FixSpec, error) {
	if opts.changed {
		return onlyChangedSpecs([]patterns.Candidate{c}, opts)
	}
	if c.FixSpec != nil {
		return []*patterns.FixSpec{c.FixSpec}, nil
	}
	return nil, nil
}

// applyAndTrack applies specs and returns the files that were written.
func applyAndTrack(ctx context.Context, specs []*patterns.FixSpec, opts options, output io.Writer) (int, []string) {
	byFile := groupByFile(specs)
	var fixed int
	var written []string
	for file, fileSpecs := range byFile {
		if err := ctx.Err(); err != nil {
			return fixed, written
		}
		n, err := fixFileWithSpecs(file, fileSpecs, opts, output)
		if err != nil {
			continue
		}
		fixed += n
		if n > 0 && opts.apply {
			written = append(written, file)
		}
	}
	return fixed, dedupe(written)
}

// dedupe removes duplicate strings, preserving order.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// collectFixSpecs runs the miner and returns FixSpecs filtered by kind,
// narrowing to diff-touched functions when --changed is set.
func collectFixSpecs(ctx context.Context, opts options) ([]*patterns.FixSpec, error) {
	candidates, err := mineCandidates(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.changed {
		return onlyChangedSpecs(candidates, opts)
	}
	return candidateSpecs(candidates), nil
}

// mineCandidates runs the miner and keeps candidates with a FixSpec of the
// requested kind.
func mineCandidates(ctx context.Context, opts options) ([]patterns.Candidate, error) {
	ext, err := gopatterns.Extract(ctx, "", opts.paths)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	facts, anemicHits, missingHits := toFacts(ext)
	report := patterns.Run(facts, patterns.Options{AnemicModels: anemicHits, MissingIdentities: missingHits})
	var candidates []patterns.Candidate
	for _, c := range report.Candidates {
		if c.FixSpec == nil {
			continue
		}
		if opts.kind != "all" && string(c.Kind) != opts.kind {
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// candidateSpecs extracts FixSpecs from filtered candidates.
func candidateSpecs(candidates []patterns.Candidate) []*patterns.FixSpec {
	specs := make([]*patterns.FixSpec, 0, len(candidates))
	for _, c := range candidates {
		specs = append(specs, c.FixSpec)
	}
	return specs
}

// onlyChangedSpecs keeps FixSpecs whose candidate site the git diff touches.
// The miner runs whole so clustering stays complete; only the fixes that
// land are narrowed. Filtering uses the candidate's site range (the
// containing function) rather than the FixSpec's precise edit location.
func onlyChangedSpecs(candidates []patterns.Candidate, opts options) ([]*patterns.FixSpec, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	diff, err := gitchanged.NewDiff(cwd, opts.paths, opts.base)
	if err != nil {
		return nil, err
	}
	var kept []*patterns.FixSpec
	for _, c := range candidates {
		if len(c.Sites) == 0 {
			continue
		}
		site := c.Sites[0]
		if diff.Touched(site.Path, site.Line, site.EndLine) {
			kept = append(kept, c.FixSpec)
		}
	}
	return kept, nil
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

// applySpecsToSource applies each spec, skipping failures. Specs run in
// pattern dependency order (see patterns.FixKindOrder): guard clauses
// first to simplify control flow, type-creating passes next, parameterize
// last since it's the most sensitive to code shape. Within a kind, specs
// run bottom-up (highest line first) so an edit never shifts the line
// numbers of specs that have not run yet.
func applySpecsToSource(path string, specs []*patterns.FixSpec, src []byte) ([]byte, int) {
	ordered := sortSpecsForApply(specs)
	out := src
	var applied int
	for _, spec := range ordered {
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

// sortSpecsForApply orders specs by pattern dependency rank, then bottom-up
// by line within a kind.
func sortSpecsForApply(specs []*patterns.FixSpec) []*patterns.FixSpec {
	ordered := make([]*patterns.FixSpec, len(specs))
	copy(ordered, specs)
	slices.SortFunc(ordered, func(a, b *patterns.FixSpec) int {
		if ra, rb := patterns.FixKindRank(a.Kind), patterns.FixKindRank(b.Kind); ra != rb {
			return ra - rb
		}
		return b.Line - a.Line
	})
	return ordered
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
	changed := flags.Bool("changed", false, "only fix candidates in diff-touched functions")
	base := flags.String("base", "", "git base for --changed")
	phased := flags.Bool("phased", false, "run in dependency phases with re-mining")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	result := options{kind: *kind, apply: *apply, changed: *changed, base: *base, phased: *phased, paths: flags.Args()}
	return result, result.validate()
}

func (opts options) validate() error {
	valid := []string{"guard_clause", "value_object", "parameterize", "trait_method", "capability_set", "enum_dispatch", "generic_fn", "anemic_model", "primitive_obsession", "type_switch", "entity_identity", "missing_identity", "mutable_identity", "aggregate", "repository", "factory", "all"}
	if !slices.Contains(valid, opts.kind) {
		return fmt.Errorf("kind must be one of %v", valid)
	}
	return nil
}

// toFacts converts extraction to FuncFacts (copied from patterncheck).
func toFacts(ext *gopatterns.Extraction) ([]*patterns.FuncFacts, []patterns.AnemicModelHit, []patterns.MissingIdentityHit) {
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
			ID:               fp.Name,
			Name:             fp.Name,
			Path:             fp.Path,
			Line:             fp.Line,
			EndLine:          fp.EndLine,
			Pdg:              &fp.Pdg,
			GuardClauses:     guards,
			EnumDispatches:   dispatches,
			TypeSwitches:     tss,
			EntityIdentities: fp.EntityIdentities,
			MutableIdentities: fp.MutableIdentities,
			AggregateMods:    fp.AggregateMods,
			DbCalls:          fp.DbCalls,
			FactoryLits:      fp.FactoryLits,
			Params:           params,
		})
	}
	return facts, ext.AnemicModels, ext.MissingIdentities
}
