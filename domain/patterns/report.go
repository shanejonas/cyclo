// Port of rstyle's patterns_report.rs: the output of the patterns miner,
// with text and JSON rendering over signature groups (layer 1),
// abstraction candidates (layer 2), and suppressed already-abstracted patterns.
package patterns

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// PatternsReport is the full output of the patterns miner.
type PatternsReport struct {
	// SignatureGroups are the layer-1 groups, best first.
	SignatureGroups []SigGroup `json:"signature_groups"`
	// Candidates are the ranked layer-2 abstraction proposals, best first.
	// Absent facts give none.
	Candidates []Candidate `json:"candidates"`
	// Suppressed are patterns that look like candidates but are already
	// abstracted: listed separately, never dropped.
	Suppressed []Suppressed `json:"suppressed"`
}

// Options selects what to show and what to pretend.
type Options struct {
	// MinScoreMilli drops groups and candidates scoring below this.
	MinScoreMilli uint32
	// Top keeps only the first N groups and candidates (all when nil).
	Top *int
	// HideImplements is the trait-hiding oracle: facts whose function id
	// contains one of these patterns lose their implements link ("*" hides
	// all), so their methods look like ordinary inherent ones. (Rust matches
	// the trait method id; Go's FuncFacts keeps implements as a bool, so the
	// function id is the closest available key.)
	HideImplements []string
	// Params tunes clustering; the zero value means DefaultParams(), mirroring
	// Rust where Options::default() carries Params::default().
	Params Params
	// AnemicModels are methodless structs with field-operating functions,
	// from the extractor's AST-level type analysis. Empty means the
	// extractor did not run or found none.
	AnemicModels []AnemicModelHit
	// MissingIdentities are structs used as entities without an ID field,
	// from the extractor's type analysis.
	MissingIdentities []MissingIdentityHit
	// WlCache memoizes WL refinements across runs for incremental mining.
	// Nil computes every refinement fresh.
	WlCache *WlCache
}

// Build keeps groups at or above minScoreMilli, then the first top (all when
// top is nil). Candidates and suppressed start empty; Run fills them in.
func Build(groups []SigGroup, minScoreMilli uint32, top *int) PatternsReport {
	return PatternsReport{
		SignatureGroups: keepGroups(groups, minScoreMilli, top),
		Candidates:      []Candidate{},
		Suppressed:      []Suppressed{},
	}
}

// Run is the full pipeline over driver facts: prepare (merge duplicate
// targets, hide implements), layer 1 (signature groups), layer 2
// (candidates), then filter and rank.
func Run(facts []*FuncFacts, options Options) PatternsReport {
	prepared := hideImplements(mergeFacts(facts), options.HideImplements)
	groups := MineGroups(prepared) // layer 1: sigmine
	// Layer 2: candidates. NOTE: candidates.go is ported in parallel with the
	// contract `func Mine(facts []*FuncFacts, groups []SigGroup, params Params) Mined`,
	// which collides with sigmine.go's `func Mine`; see the commit report.
	mined := MineCached(prepared, groups, defaultParams(options.Params), options.WlCache)
	// Type switches: same-method arms want interface dispatch, also
	// single-pass and fixed-score.
	if tss := typeSwitchCandidates(prepared); len(tss) > 0 {
		mined.Candidates = append(mined.Candidates, tss...)
		sortMined(&mined)
	}
	// Enum dispatch: value switches that want to be dispatch tables,
	// single-pass AST findings, fixed score.
	if eds := enumDispatchCandidates(prepared); len(eds) > 0 {
		mined.Candidates = append(mined.Candidates, eds...)
		sortMined(&mined)
	}
	addSingleFunctionCandidates(&mined, prepared, options)
	report := Build(groups, options.MinScoreMilli, options.Top)
	report.Candidates = keepCandidates(mined.Candidates, options.MinScoreMilli, options.Top)
	report.Suppressed = mined.Suppressed
	return report
}

// singleFunctionBuilder pairs a pattern kind with its candidate builder.
type singleFunctionBuilder struct {
	kind  CandidateKind
	build func(prepared []*FuncFacts, options Options) []Candidate
}

// singleFunctionBuilders runs in FixKindOrder so detection respects
// dependencies: guards simplify control flow first, primitive types are
// created before value objects bundle them, factories run after the
// type-creating passes. Detection-only kinds sort last.
var singleFunctionBuilders = []singleFunctionBuilder{
	{GuardClause, func(prepared []*FuncFacts, options Options) []Candidate { return guardCandidates(prepared) }},
	{PrimitiveObsession, func(prepared []*FuncFacts, options Options) []Candidate { return primitiveObsessionCandidates(prepared) }},
	{ValueObject, func(prepared []*FuncFacts, options Options) []Candidate { return valueObjectCandidates(prepared) }},
	{Factory, func(prepared []*FuncFacts, options Options) []Candidate { return factoryCandidates(prepared) }},
	{MissingIdentity, func(prepared []*FuncFacts, options Options) []Candidate { return missingIdentityCandidates(options.MissingIdentities) }},
	{EntityIdentity, func(prepared []*FuncFacts, options Options) []Candidate { return entityIdentityCandidates(prepared) }},
	{AnemicModel, func(prepared []*FuncFacts, options Options) []Candidate { return anemicModelCandidates(options.AnemicModels) }},
	{MutableIdentity, func(prepared []*FuncFacts, options Options) []Candidate { return mutableIdentityCandidates(prepared) }},
	{Aggregate, func(prepared []*FuncFacts, options Options) []Candidate { return aggregateCandidates(prepared) }},
	{Repository, func(prepared []*FuncFacts, options Options) []Candidate { return repositoryCandidates(prepared) }},
}

// addSingleFunctionCandidates appends fixed-score candidates that need no
// clustering. Builders run in FixKindOrder (see singleFunctionBuilders)
// so detection respects pattern dependencies.
func addSingleFunctionCandidates(mined *Mined, prepared []*FuncFacts, options Options) {
	for _, b := range singleFunctionBuilders {
		if cands := b.build(prepared, options); len(cands) > 0 {
			mined.Candidates = append(mined.Candidates, cands...)
			sortMined(mined)
		}
	}
}

// Text renders the report for humans, mirroring rstyle's text output.
func Text(report *PatternsReport) string {
	groups := make([]string, len(report.SignatureGroups))
	for i := range report.SignatureGroups {
		groups[i] = groupText(&report.SignatureGroups[i])
	}
	found := make([]string, len(report.Candidates))
	for i := range report.Candidates {
		found[i] = candidateText(i, &report.Candidates[i])
	}
	parts := []string{
		section(fmt.Sprintf("%d signature groups", len(groups)), groups),
		section(fmt.Sprintf("%d candidates", len(found)), found),
	}
	if len(report.Suppressed) > 0 {
		lines := make([]string, len(report.Suppressed))
		for i := range report.Suppressed {
			lines[i] = suppressedText(&report.Suppressed[i])
		}
		parts = append(parts, fmt.Sprintf("%d suppressed (already abstracted)\n%s",
			len(lines), strings.Join(lines, "\n")))
	}
	return strings.Join(parts, "\n\n")
}

// JSON renders the report as pretty JSON.
func JSON(report *PatternsReport) (string, error) {
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// mergeFacts dedupes facts by id, mirroring rstyle's facts::merge: lib and
// test targets compile the same functions, so one id must not mine twice.
func mergeFacts(facts []*FuncFacts) []*FuncFacts {
	merged := make([]*FuncFacts, len(facts))
	copy(merged, facts)
	sort.SliceStable(merged, func(i, j int) bool { return factID(merged[i]) < factID(merged[j]) })
	out := merged[:0]
	for _, f := range merged {
		if len(out) == 0 || factID(out[len(out)-1]) != factID(f) {
			out = append(out, f)
		}
	}
	return out
}

func factID(f *FuncFacts) string {
	if f == nil {
		return ""
	}
	return f.ID
}

// hideImplements forgets implements links whose function id contains one of
// the patterns ("*" hides all), mirroring rstyle's prep::hide_implements.
// Facts are copied, never mutated in place.
func hideImplements(facts []*FuncFacts, patterns []string) []*FuncFacts {
	prepared := make([]*FuncFacts, len(facts))
	for i, f := range facts {
		if f == nil || !f.Implements || !hiddenLink(f.ID, patterns) {
			prepared[i] = f
			continue
		}
		c := *f
		c.Implements = false
		prepared[i] = &c
	}
	return prepared
}

func hiddenLink(id string, patterns []string) bool {
	for _, p := range patterns {
		if p == "*" || strings.Contains(id, p) {
			return true
		}
	}
	return false
}

// defaultParams maps the zero Params to DefaultParams, mirroring Rust where
// Options::default() carries Params::default().
func defaultParams(p Params) Params {
	if reflect.DeepEqual(p, Params{}) {
		return DefaultParams()
	}
	return p
}

// keepTop keeps items at or above minScoreMilli, then the first top.
// It generalizes keepGroups and keepCandidates, which shared one body.
func keepTop[T any](items []T, score func(T) uint32, minScoreMilli uint32, top *int) []T {
	kept := []T{}
	for _, item := range items {
		if score(item) < minScoreMilli {
			continue
		}
		kept = append(kept, item)
		if top != nil && len(kept) >= *top {
			break
		}
	}
	return kept
}

// keepGroups keeps groups at or above minScoreMilli, then the first top.
func keepGroups(groups []SigGroup, minScoreMilli uint32, top *int) []SigGroup {
	return keepTop(groups, func(g SigGroup) uint32 { return g.ScoreMilli }, minScoreMilli, top)
}

// keepCandidates keeps candidates at or above minScoreMilli, then the first top.
func keepCandidates(candidates []Candidate, minScoreMilli uint32, top *int) []Candidate {
	return keepTop(candidates, func(c Candidate) uint32 { return c.ScoreMilli }, minScoreMilli, top)
}

func milli(value uint32) string {
	return fmt.Sprintf("%.2f", float64(value)/1000.0)
}

func memberSite(path string, line int, name string) string {
	return fmt.Sprintf("    %s:%d  %s", path, line, name)
}

func groupText(g *SigGroup) string {
	suffix := ""
	if g.SameName {
		suffix = ", same name"
	}
	lines := []string{fmt.Sprintf("%s  [%d types, score %s%s]",
		g.Key, g.SelfTypes, milli(g.ScoreMilli), suffix)}
	for _, m := range g.Members {
		lines = append(lines, memberSite(m.Path, m.Line, m.Name))
	}
	if len(g.Traited) > 0 {
		lines = append(lines, fmt.Sprintf("    (%d via trait impls)", len(g.Traited)))
	}
	return strings.Join(lines, "\n")
}

func headline(index int, c *Candidate) string {
	kind := string(c.Kind)
	if kind == "" {
		kind = "candidate"
	}
	b := c.Breakdown
	return fmt.Sprintf("#%d %s  score %s  [support %d, lift %s, holes %d, coverage %s]",
		index+1, kind, milli(c.ScoreMilli),
		b.Support, milli(b.LiftMilli), b.Holes, milli(b.CoverageMilli))
}

func candidateText(index int, c *Candidate) string {
	lines := []string{
		headline(index, c),
		fmt.Sprintf("  observation: %s", c.Observation),
		fmt.Sprintf("  inference: %s", c.Inference),
		fmt.Sprintf("  possible refactor: %s", c.PossibleRefactor),
		"  sites:",
	}
	for _, s := range c.Sites {
		lines = append(lines, memberSite(s.Path, s.Line, s.Name))
	}
	lines = append(lines, "  definitions:")
	for _, s := range c.Definitions {
		lines = append(lines, memberSite(s.Path, s.Line, s.Name))
	}
	if len(c.CounterEvidence) > 0 {
		lines = append(lines, "  counter evidence:")
		for _, e := range c.CounterEvidence {
			lines = append(lines, fmt.Sprintf("    - %s", e))
		}
	}
	return strings.Join(lines, "\n")
}

func suppressedText(s *Suppressed) string {
	sites := make([]string, len(s.Sites))
	for i, x := range s.Sites {
		sites[i] = fmt.Sprintf("%s:%d  %s", x.Path, x.Line, x.Name)
	}
	return fmt.Sprintf("  %s (%s)", s.Reason, strings.Join(sites, ", "))
}

func section(title string, body []string) string {
	return strings.Join(append([]string{title}, body...), "\n\n")
}
