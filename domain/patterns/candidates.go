// Layer 2 of the patterns miner: hole columns of function clusters (and
// dispatch inside one function) are matched against layer-1 signature groups
// to propose interfaces, generic functions, or parameters.
//
// Port of rstyle-core's candidates.rs.
//
// Go adaptations (each noted at its use site):
//   - The main entry is Mine, colliding with sigmine's layer-1 Mine; the
//     layer-1 entry was renamed to MineGroups.
//   - FuncFacts carries no receiver: Rust's `receiver == None` (associated
//     function without self) check becomes "no SelfTy at all", which the
//     types.len() < 2 check would already reject; the explicit test keeps
//     the Rust structure.
//   - FuncFacts.Implements is a bool: the interface method id is unknown, so
//     the "already abstracted" suppression names no interface, the "extend
//     the existing trait" text is unreachable, and the same-crate versus
//     external-trait distinction in generic_fn/parameterize collapses to
//     "any interface impl counts as already abstracted" (conservative).
//   - Callee owners come from the callee id itself ("pkgpath.Type.Method"
//     -> "Type"); rstyle reads the callee's signature owner instead.
//   - sketch renders Go method sketches ("func method() string"); the
//     SigClass receiver (the erased "_" first parameter) is dropped as
//     implicit in a method set.
//   - "trait" prose becomes "interface"; the kind names stay snake_case per
//     the serialized contract.
//   - Rust's text_diff walks raw type shapes, so it reports differing ADT
//     ids on any paired node; the Go schema keeps only the flat TyClass,
//     and Param WL labels include TyClass, so a Go Type hole needs
//     TyClass differences on nodes whose label hides it (e.g. Iterate).
package patterns

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
)

// CandidateKind is the kind of an abstraction proposal.
type CandidateKind string

const (
	// TraitMethod proposes an interface from parallel methods.
	TraitMethod CandidateKind = "trait_method"
	// CapabilitySet proposes an interface from several parallel methods.
	CapabilitySet CandidateKind = "capability_set"
	// EnumDispatch proposes an interface from a type switch's arm calls.
	EnumDispatch CandidateKind = "enum_dispatch"
	// GenericFn proposes one generic definition over workspace types.
	GenericFn CandidateKind = "generic_fn"
	// FnParam is reserved by the Rust original; never constructed here.
	FnParam CandidateKind = "fn_param"
	// Parameterize proposes extracting one helper taking the differing
	// parts as parameters.
	Parameterize CandidateKind = "parameterize"
	// GuardClause proposes inverting an if/else into a guard clause so the
	// happy path flows at top level. Single-function finding: no
	// clustering, fixed score.
	GuardClause CandidateKind = "guard_clause"
	// ValueObject proposes extracting an immutable value object from a
	// data clump: the same primitive params traveling together across
	// functions. DDD-inspired; fixed score below guard clauses.
	ValueObject CandidateKind = "value_object"
	// AnemicModel proposes moving behavior into methods for a struct
	// that holds data but has no behavior while external functions
	// operate on its fields. DDD-inspired; fixed score below value
	// objects (more judgment involved).
	AnemicModel CandidateKind = "anemic_model"
	// PrimitiveObsession proposes a named type for a domain concept used as
	// a raw string/int param across functions. DDD-inspired; fixed score
	// below value objects (more heuristic).
	PrimitiveObsession CandidateKind = "primitive_obsession"
)

// Site is one function that shows the pattern.
type Site struct {
	Path string
	Line int
	// EndLine is the site's last line, for range-overlap checks
	// (e.g. --changed filtering). Zero when unknown.
	EndLine int
	ID      string
	Name    string
}

// Breakdown splits the score so each signal stays visible.
type Breakdown struct {
	// Support is the functions in the cluster, or arms of the match.
	Support int
	// LiftMilli is the specificity of the joined signature group.
	LiftMilli uint32
	// Holes counts hole columns; many holes mean a weak template.
	Holes         int
	CoverageMilli uint32
}

// Candidate is one ranked abstraction proposal.
type Candidate struct {
	Kind             CandidateKind
	ScoreMilli       uint32
	Breakdown        Breakdown
	Observation      string
	Inference        string
	PossibleRefactor string
	// Sites is every function that shows the pattern.
	Sites []Site
	// Definitions are the functions that fill the method holes.
	Definitions     []Site
	CounterEvidence []string
	// FixSpec is the precise info a fixer needs to apply this candidate
	// statically. Nil if the candidate is not auto-fixable (should not
	// happen — every kind must have a fixer).
	FixSpec *FixSpec
}

// FixSpec carries the exact positions and parameters a fixer needs to
// apply a candidate without re-detecting. It is JSON-serializable so
// `cyclo patterns --format json` output can be piped to `cyclo fix`.
type FixSpec struct {
	Kind CandidateKind `json:"kind"`
	// File is the path to the source file containing the pattern.
	File string `json:"file"`
	// Line and EndLine bound the primary location (e.g. the if statement
	// for guard_clause, the struct definition for value_object).
	Line    int `json:"line"`
	EndLine int `json:"end_line"`
	// Params holds kind-specific transform parameters as strings.
	// Each fixer documents the keys it expects.
	Params map[string]string `json:"params"`
	// Definitions are the method/function sites for interface generation
	// (trait_method, capability_set). Each entry is "path:line:name".
	Definitions []string `json:"definitions,omitempty"`
}

// Suppressed is a pattern that looks like a candidate but is already
// abstracted: listed separately, never dropped.
type Suppressed struct {
	Reason string
	Sites  []Site
}

// Mined is the layer-2 result: ranked candidates and suppressions.
type Mined struct {
	Candidates []Candidate
	Suppressed []Suppressed
}

// candidateIndex holds the lookup tables over the corpus.
type candidateIndex struct {
	byID   map[string]*FuncFacts
	groups map[string]*SigGroup
	// workspaceADTs are SelfTy values with facts in this corpus: the only
	// types worth abstracting over.
	workspaceADTs map[string]bool
	// calleeOwners is the owner type of every called id seen in a PDG.
	calleeOwners map[string]string
	// singleCallGuard is cluster.Params.SingleCallGuard.
	singleCallGuard bool
	// maxHoles is cluster.Params.MaxHoles.
	maxHoles int
}

func newCandidateIndex(facts []*FuncFacts, groups []SigGroup, params Params) *candidateIndex {
	ix := &candidateIndex{
		byID:            map[string]*FuncFacts{},
		groups:          map[string]*SigGroup{},
		workspaceADTs:   map[string]bool{},
		calleeOwners:    map[string]string{},
		singleCallGuard: params.SingleCallGuard,
		maxHoles:        params.MaxHoles,
	}
	for _, f := range facts {
		if f == nil {
			continue
		}
		ix.byID[f.ID] = f
		if f.SelfTy != "" {
			ix.workspaceADTs[f.SelfTy] = true
		}
	}
	for i := range groups {
		ix.groups[groups[i].Key] = &groups[i]
	}
	for id, owner := range calleeOwners(facts) {
		ix.calleeOwners[id] = owner
	}
	return ix
}

// calleeOwners maps every called id seen in a PDG to its owner type.
// Go adaptation: rstyle reads the callee's signature owner; the Go schema
// keeps no callee signatures, so the owner is derived from the callee id:
// "pkgpath.Type.Method" -> "Type" (see hasReceiver in align.go). Free
// functions have no owner.
// indexCalleeOwners records owners for every callee id in one PDG.
func indexCalleeOwners(out map[string]string, pdg *Pdg) {
	for _, n := range pdg.Nodes {
		id := n.CalleeID
		if id == "" {
			continue
		}
		if owner, ok := calleeOwner(id); ok {
			out[id] = owner
		}
	}
}

func calleeOwners(facts []*FuncFacts) map[string]string {
	out := map[string]string{}
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		indexCalleeOwners(out, f.Pdg)
	}
	return out
}

func calleeOwner(id string) (string, bool) {
	if !hasReceiver(id) {
		return "", false
	}
	return shortGoPaths(id[:strings.LastIndexByte(id, '.')]), true
}

// display renders an id: the function name when known, else the short name
// after the last '.'.
func (ix *candidateIndex) display(value string) string {
	if f, ok := ix.byID[value]; ok {
		return f.Name
	}
	return shortName(value)
}

// displayCallee renders a callee id: external ones keep their owner
// ("Tcp.Connect", not "Connect").
func (ix *candidateIndex) displayCallee(value string) string {
	if f, ok := ix.byID[value]; ok {
		return f.Name
	}
	if owner, ok := ix.calleeOwners[value]; ok {
		return owner + "." + shortName(value)
	}
	return lastTwo(value)
}

// lastTwo keeps the last two '.' segments ("a.Tcp.Connect" ->
// "Tcp.Connect").
func lastTwo(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return id
}

// shortGoPaths drops module paths inside a rendered owner: "a.W[b.X]" ->
// "W[X]". Port of Rust's short_paths ("a::W<b::X>" -> "W<X>"), with '.'
// as the Go path separator.
// shortWord strips the package path from a dotted word, keeping the last segment.
func shortWord(s string) string {
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// isWordChar reports whether r continues an identifier-like word.
func isWordChar(r rune) bool {
	return r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func shortGoPaths(owner string) string {
	var out, word strings.Builder
	for _, c := range owner {
		if isWordChar(c) {
			word.WriteRune(c)
		} else {
			out.WriteString(shortWord(word.String()))
			word.Reset()
			out.WriteRune(c)
		}
	}
	out.WriteString(shortWord(word.String()))
	return out.String()
}

func makeSite(f *FuncFacts) Site {
	return Site{Path: f.Path, Line: f.Line, EndLine: f.EndLine, ID: f.ID, Name: f.Name}
}

func sitesOfFacts(fns []*FuncFacts) []Site {
	sites := make([]Site, len(fns))
	for i, f := range fns {
		sites[i] = makeSite(f)
	}
	return sites
}

// existing is a method column that is already an interface method on every
// owner type.
type existing struct {
	reason string
	// traitMethod is the interface method id; "" when unknown (Go records
	// Implements as a bool, so the id is never known).
	traitMethod string
	owners      map[string]bool
}

type verdictKind int

const (
	verdictSkip verdictKind = iota
	verdictTrait
	verdictSuppressed
)

// verdict is what a set of hole values (callee ids) amounts to.
type verdict struct {
	kind     verdictKind
	group    *SigGroup
	defs     []*FuncFacts
	existing *existing
}

func distinctSorted(values []string) []string {
	seen := map[string]bool{}
	for _, v := range values {
		seen[v] = true
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func distinctInOrder(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// sameTraitMethod reports whether every def implements an interface method.
// Go adaptation: rstyle returns the shared trait method id; the Go schema
// records only that a method implements an interface, so the id is unknown.
func sameTraitMethod(defs []*FuncFacts) bool {
	if len(defs) == 0 {
		return false
	}
	for _, d := range defs {
		if !d.Implements {
			return false
		}
	}
	return true
}

func lookupDefs(ix *candidateIndex, wanted []string) []*FuncFacts {
	var defs []*FuncFacts
	for _, v := range wanted {
		if f, ok := ix.byID[v]; ok {
			defs = append(defs, f)
		}
	}
	return defs
}

func ownerSet(defs []*FuncFacts) map[string]bool {
	types := map[string]bool{}
	for _, d := range defs {
		if d.SelfTy != "" {
			types[d.SelfTy] = true
		}
	}
	return types
}

// noReceiver reports whether no def has a receiver type. Go adaptation:
// rstyle checks receiver == None (associated function without self); the Go
// schema records only SelfTy, so a receiver-less function is one with no
// SelfTy at all. Such defs already fail the two-types check below; the
// explicit test keeps the Rust structure.
func noReceiver(defs []*FuncFacts) bool {
	for _, d := range defs {
		if d.SelfTy != "" {
			return false
		}
	}
	return true
}

// judgeSigGroup joins the defs' signature keys against the layer-1 groups.
func judgeSigGroup(ix *candidateIndex, defs []*FuncFacts) verdict {
	keys := map[string]bool{}
	for _, d := range defs {
		keys[d.SigKey] = true
	}
	if len(keys) == 1 {
		for k := range keys {
			if g, ok := ix.groups[k]; ok {
				return verdict{kind: verdictTrait, group: g, defs: defs}
			}
		}
	}
	return verdict{kind: verdictSkip}
}

func judge(ix *candidateIndex, values []string) verdict {
	wanted := distinctSorted(values)
	defs := lookupDefs(ix, wanted)
	// Constructors are not a capability callers use polymorphically.
	if noReceiver(defs) || len(defs) != len(wanted) || len(ownerSet(defs)) < 2 {
		return verdict{kind: verdictSkip}
	}
	if sameTraitMethod(defs) {
		return verdict{kind: verdictSuppressed, existing: &existing{
			reason: fmt.Sprintf("all %d impls are of the same interface method", len(defs)),
			owners: ownerSet(defs),
		}}
	}
	return judgeSigGroup(ix, defs)
}

func equalStringSets(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func showClass(classes map[string]bool) string {
	if len(classes) == 0 {
		return "pure"
	}
	names := make([]string, 0, len(classes))
	for k := range classes {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "+")
}

// effectMismatch notes when the implementations differ in observable effects
// (one mutates, one is pure). Empty means uniform.
func effectMismatch(defs []*FuncFacts) string {
	classes := make([]map[string]bool, len(defs))
	for i, d := range defs {
		classes[i] = d.EffectClass()
	}
	uniform := true
	for i := 1; i < len(classes); i++ {
		if !equalStringSets(classes[0], classes[i]) {
			uniform = false
			break
		}
	}
	if uniform {
		return ""
	}
	parts := make([]string, len(defs))
	for i, d := range defs {
		parts[i] = d.Name + " is " + showClass(classes[i])
	}
	return "effect mismatch: " + strings.Join(parts, ", ")
}

// ScoreMilli scores a breakdown: (1000·ln(support) + lift) × coverage/1000,
// damped past 3 holes (×3/holes), halved on effect mismatch.
func ScoreMilli(b Breakdown, mismatch bool) uint32 {
	support := math.Log(math.Max(1, float64(b.Support))) * 1000.0
	base := (support + float64(b.LiftMilli)) * float64(b.CoverageMilli) / 1000.0
	damped := base
	if b.Holes > 3 {
		damped = base * 3.0 / float64(b.Holes)
	}
	if mismatch {
		damped /= 2.0
	}
	return uint32(math.Round(damped))
}

// sketch renders a method sketch from a signature key and name.
// Go adaptation: rstyle strips "&mut Self"/"&Self"/"Self" receiver prefixes
// and renders "fn name(&self) -> T;". The Go SigClass carries the receiver
// as the erased "_" first parameter, which an interface method sketch drops
// as implicit; the result is a Go method sketch ("func method() string").
func sketch(key, name string) string {
	body := strings.TrimPrefix(key, "fn(")
	if rest, ok := strings.CutPrefix(body, "_, "); ok {
		body = rest
	} else if rest, ok := strings.CutPrefix(body, "_)"); ok {
		body = ")" + rest
	}
	params, ret, _ := strings.Cut(body, ") -> ")
	out := "func " + name + "(" + params + ")"
	if ret != "" && ret != "()" {
		out += " " + ret
	}
	return out
}

// sharedName is the common short method name, or "method".
func sharedName(defs []*FuncFacts) string {
	names := map[string]bool{}
	for _, d := range defs {
		names[shortName(d.Name)] = true
	}
	if len(names) == 1 {
		for n := range names {
			return n
		}
	}
	return "method"
}

// traitName is the interface of an interface-method id:
// "k.public.Signature.is_valid" -> "Signature". (Unreachable in practice:
// Go never knows the method id; kept for parity.)
func traitName(traitMethod string) string {
	parts := strings.Split(traitMethod, ".")
	traitPath := parts[:len(parts)-1]
	if len(traitPath) == 0 {
		return traitMethod
	}
	return traitPath[len(traitPath)-1]
}

// traitText renders the inference and the possible refactor for trait-like
// candidateEvidence. Go adaptation: "trait" becomes "interface" in the prose.
func traitText(ev *candidateEvidence) (inference, refactor string) {
	methods := make([]string, len(ev.verdicts))
	for i, v := range ev.verdicts {
		methods[i] = sketch(v.group.Key, sharedName(v.defs))
	}
	types := ownerSet(ev.verdicts[0].defs)
	sorted := make([]string, 0, len(types))
	for t := range types {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)
	shorted := make([]string, len(sorted))
	for i, t := range sorted {
		shorted[i] = shortGoPaths(t)
	}
	impls := strings.Join(shorted, ", ")
	inference = impls + " are used interchangeably through methods of identical signature: an implicit interface."
	if ev.existingTrait != "" {
		refactor = fmt.Sprintf("add %s to existing interface `%s`, already implemented by %s; make the sites generic over it",
			strings.Join(methods, " "), traitName(ev.existingTrait), impls)
	} else {
		refactor = fmt.Sprintf("interface Shared { %s } implemented by %s; make the sites generic over it",
			strings.Join(methods, " "), impls)
	}
	return inference, refactor
}

// traitVerdict pairs a joined signature group with its definitions.
type traitVerdict struct {
	group *SigGroup
	defs  []*FuncFacts
}

// candidateEvidence is the finished evidence for one candidate, shared by the
// cluster and dispatch paths.
type candidateEvidence struct {
	kind      CandidateKind
	breakdown Breakdown
	sites     []*FuncFacts
	verdicts  []traitVerdict
	summary   string
	// existingTrait is the interface method id of a suppressed column in the
	// same cluster: extend that interface instead. "" when unknown (always,
	// in Go: Implements is a bool).
	existingTrait string
}

func definitionSites(defs []*FuncFacts) []Site {
	seen := map[string]bool{}
	var out []Site
	for _, d := range defs {
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		out = append(out, makeSite(d))
	}
	sort.Slice(out, func(i, j int) bool { return compareSite(out[i], out[j]) < 0 })
	return out
}

func finish(ev *candidateEvidence) Candidate {
	var allDefs []*FuncFacts
	for _, v := range ev.verdicts {
		allDefs = append(allDefs, v.defs...)
	}
	var counter []string
	for _, v := range ev.verdicts {
		if note := effectMismatch(v.defs); note != "" {
			counter = append(counter, note)
		}
	}
	mismatch := len(counter) > 0
	if ev.breakdown.LiftMilli < 500 {
		counter = append(counter, "signature is common in this codebase (low specificity)")
	}
	inference, refactor := traitText(ev)
	sitesList := sitesOfFacts(ev.sites)
	defsList := definitionSites(allDefs)
	// Build definitions for FixSpec: "path:line:name" entries.
	var defStrs []string
	for _, d := range defsList {
		defStrs = append(defStrs, fmt.Sprintf("%s:%d:%s", d.Path, d.Line, d.Name))
	}
	return Candidate{
		Kind:             ev.kind,
		ScoreMilli:       ScoreMilli(ev.breakdown, mismatch),
		Breakdown:        ev.breakdown,
		Observation:      ev.summary,
		Inference:        inference,
		PossibleRefactor: refactor,
		Sites:            sitesList,
		Definitions:      defsList,
		CounterEvidence:  counter,
		FixSpec: &FixSpec{
			Kind:        ev.kind,
			File:        sitesList[0].Path,
			Line:        sitesList[0].Line,
			EndLine:     sitesList[0].EndLine,
			Params:      map[string]string{},
			Definitions: defStrs,
		},
	}
}

// methodLike reports whether a hole kind is a call: method or free function.
func methodLike(kind HoleKind) bool {
	return kind == HoleMethod || kind == HoleFreeFn
}

// columnsText renders hole columns: "T0 = Dog | Cat; M0 = Dog.bark | Cat.meow".
func columnsText(ix *candidateIndex, columns []Column) string {
	cols := make([]*Column, len(columns))
	for i := range columns {
		cols[i] = &columns[i]
	}
	return columnsTextOf(ix, cols)
}

func columnsTextOf(ix *candidateIndex, columns []*Column) string {
	parts := make([]string, len(columns))
	for i, c := range columns {
		shown := make([]string, 0, len(c.Values))
		for _, v := range distinctInOrder(c.Values) {
			if methodLike(c.Kind) {
				shown = append(shown, ix.displayCallee(v))
			} else {
				shown = append(shown, ix.display(v))
			}
		}
		parts[i] = c.Var + " = " + strings.Join(shown, " | ")
	}
	return strings.Join(parts, "; ")
}

type outcomeKind int

const (
	outcomeNothing outcomeKind = iota
	outcomeFound
	// outcomeReady is a candidate that needs no trait evidence
	// (type-only clusters).
	outcomeReady
	outcomeHidden
)

type outcome struct {
	kind       outcomeKind
	ev         *candidateEvidence
	candidate  *Candidate
	suppressed *Suppressed
}

func sitesOf(fns []*FuncFacts, cluster *Cluster) []*FuncFacts {
	sites := make([]*FuncFacts, len(cluster.Members))
	for i, m := range cluster.Members {
		sites[i] = fns[m]
	}
	sort.Slice(sites, func(i, j int) bool {
		a, b := sites[i], sites[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.ID < b.ID
	})
	return sites
}

// judgeColumns judges the method-like columns: trait evidence, and the
// suppressed columns.
func judgeColumns(ix *candidateIndex, cols []*Column) (verdicts []traitVerdict, hidden []existing) {
	for _, col := range cols {
		switch v := judge(ix, col.Values); v.kind {
		case verdictTrait:
			verdicts = append(verdicts, traitVerdict{group: v.group, defs: v.defs})
		case verdictSuppressed:
			hidden = append(hidden, *v.existing)
		}
	}
	return verdicts, hidden
}

func liftOf(verdicts []traitVerdict) uint32 {
	lift := uint32(0)
	first := true
	for _, v := range verdicts {
		if first || v.group.ScoreMilli < lift {
			lift, first = v.group.ScoreMilli, false
		}
	}
	return lift
}

// minGenericCalls: fewer calls than this is a delegation or accessor, not a
// body worth generalizing.
const minGenericCalls = 2

// forwardsParams reports whether every call in f is fed by parameters alone:
// a delegation shim.
// callArgsAllParams reports whether every data input to a call is a parameter.
func callArgsAllParams(pdg *Pdg, call int) bool {
	for _, e := range pdg.Edges {
		if e.To == call && e.Kind == Data && pdg.Nodes[e.From].Kind != Param {
			return false
		}
	}
	return true
}

func forwardsParams(f *FuncFacts) bool {
	pdg := f.Pdg
	if pdg == nil {
		return true
	}
	for call, n := range pdg.Nodes {
		if n.Kind != Call {
			continue
		}
		if !callArgsAllParams(pdg, call) {
			return false
		}
	}
	return true
}

func callCount(f *FuncFacts) int {
	if f.Pdg == nil {
		return 0
	}
	count := 0
	for _, n := range f.Pdg.Nodes {
		if n.Kind == Call {
			count++
		}
	}
	return count
}

// hasSubstance reports whether a body is worth generalizing. With the guard,
// a single call counts too unless it is a pure parameter forward.
func hasSubstance(ix *candidateIndex, f *FuncFacts) bool {
	switch n := callCount(f); {
	case n >= minGenericCalls:
		return true
	case n == 1:
		return ix.singleCallGuard && !forwardsParams(f)
	default:
		return false
	}
}

func summaryOf(ix *candidateIndex, sites []*FuncFacts, cluster *Cluster) string {
	return fmt.Sprintf("%d functions share one dependence structure and differ only at %s",
		len(sites), columnsText(ix, cluster.Columns))
}

func breakdownOf(sites []*FuncFacts, cluster *Cluster, liftMilli uint32) Breakdown {
	var sum uint32
	for _, c := range cluster.CoverageMilli {
		sum += c
	}
	denom := uint32(len(cluster.CoverageMilli))
	if denom == 0 {
		denom = 1
	}
	return Breakdown{
		Support:       len(sites),
		LiftMilli:     liftMilli,
		Holes:         len(cluster.Columns),
		CoverageMilli: sum / denom,
	}
}

// genericColumns splits a cluster's columns into type holes and the rest,
// checking the generic_fn preconditions: some type column over workspace
// types only, and every other column an external callee hole.
// allWorkspaceADTs reports whether every hole value is a workspace ADT.
func allWorkspaceADTs(ix *candidateIndex, c *Column) bool {
	for _, v := range c.Values {
		if !ix.workspaceADTs[v] {
			return false
		}
	}
	return true
}

// externalCallee reports whether a method-like hole refers only to external
// callees: a workspace callee that differs per type is behaviour, not just
// a type parameter.
func externalCallee(ix *candidateIndex, c *Column) bool {
	if !methodLike(c.Kind) {
		return false
	}
	for _, v := range c.Values {
		if _, known := ix.byID[v]; known {
			return false
		}
	}
	return true
}

// splitColumns partitions hole columns into type holes and the rest.
func splitColumns(cluster *Cluster) (types, others []*Column) {
	for i := range cluster.Columns {
		c := &cluster.Columns[i]
		if c.Kind == HoleType {
			types = append(types, c)
		} else {
			others = append(others, c)
		}
	}
	return types, others
}

// anyAllWorkspaceADTs reports whether some type hole is all workspace ADTs.
func anyAllWorkspaceADTs(ix *candidateIndex, types []*Column) bool {
	for _, c := range types {
		if allWorkspaceADTs(ix, c) {
			return true
		}
	}
	return false
}

func genericColumns(ix *candidateIndex, cluster *Cluster) (types, others []*Column, ok bool) {
	types, others = splitColumns(cluster)
	anyOwn := anyAllWorkspaceADTs(ix, types)
	for _, c := range others {
		if !externalCallee(ix, c) {
			return nil, nil, false
		}
	}
	return types, others, anyOwn
}

// genericFn proposes one generic definition over workspace types for a
// cluster whose only holes are workspace types (and external callees).
// Ranked below trait candidateEvidence (score halved).
func genericFn(ix *candidateIndex, sites []*FuncFacts, cluster *Cluster) *Candidate {
	// Go adaptation: rstyle suppresses only impls of a same-crate trait
	// (external-trait impls are forced by it, so duplication stays real).
	// The Go schema never knows the interface id, so any all-implementing
	// site set counts as already abstracted.
	if sameTraitMethod(sites) {
		return nil
	}
	for _, f := range sites {
		if !hasSubstance(ix, f) || isBoilerplate(f) {
			return nil
		}
	}
	types, _, ok := genericColumns(ix, cluster)
	if !ok {
		return nil
	}
	// External type holes are often the ones that matter: show all.
	holes := columnsTextOf(ix, types)
	breakdown := breakdownOf(sites, cluster, 0)
	sitesList := sitesOfFacts(sites)
	return &Candidate{
		Kind:             GenericFn,
		ScoreMilli:       ScoreMilli(breakdown, false) / 2,
		Breakdown:        breakdown,
		Observation:      summaryOf(ix, sites, cluster),
		Inference:        "the same code runs over several workspace types that differ only by type",
		PossibleRefactor: fmt.Sprintf("one generic definition over %s; keep the existing names as type aliases", holes),
		Sites:            sitesList,
		FixSpec: &FixSpec{
			Kind:    GenericFn,
			File:    sitesList[0].Path,
			Line:    sitesList[0].Line,
			EndLine: sitesList[0].EndLine,
			Params:  map[string]string{},
		},
	}
}

// isBoilerplate reports code whose repetition is not a refactoring target:
// tests repeat arrange/act on purpose, `main` is per-binary boilerplate,
// `ffi`/`raw` modules and `bindgen_*` items are thin shims over C.
// hasSegment reports whether any path segment matches.
func hasSegment(segs []string, match func(string) bool) bool {
	for _, s := range segs {
		if match(s) {
			return true
		}
	}
	return false
}

// inTestPath reports whether the function lives under a tests directory.
func inTestPath(f *FuncFacts, segs []string) bool {
	return hasSegment(segs, func(s string) bool { return s == "tests" }) ||
		strings.HasPrefix(f.Path, "tests/") || strings.Contains(f.Path, "/tests/")
}

// isFFIModule reports whether the function is in an ffi/raw/bindgen module.
func isFFIModule(segs []string) bool {
	return hasSegment(segs, func(s string) bool {
		return s == "ffi" || s == "raw" || strings.HasPrefix(s, "bindgen")
	})
}

func isBoilerplate(f *FuncFacts) bool {
	segs := strings.FieldsFunc(f.ID, func(r rune) bool { return r == '.' || r == '/' })
	main := len(segs) > 0 && segs[len(segs)-1] == "main"
	return inTestPath(f, segs) || isFFIModule(segs) || main
}

// parameterizable reports whether a cluster is worth a parameterize
// suggestion: not boilerplate, not impls forced by an interface, and few
// enough holes to be a helper's parameters.
func parameterizable(sites []*FuncFacts, cluster *Cluster, maxHoles int) bool {
	forced := true
	for _, f := range sites {
		if !f.Implements {
			forced = false
		}
	}
	if forced {
		return false
	}
	for _, f := range sites {
		if isBoilerplate(f) {
			return false
		}
	}
	return len(cluster.Columns) <= maxHoles
}

// parameterize proposes one helper taking the differing parts as parameters
// for a cluster that is neither a trait nor a generic fn over workspace
// types. Ranked below generic_fn (score thirded).
func parameterize(ix *candidateIndex, sites []*FuncFacts, cluster *Cluster) *Candidate {
	// Same Go adaptation as genericFn: the interface id is unknown.
	if sameTraitMethod(sites) {
		return nil
	}
	if !parameterizable(sites, cluster, ix.maxHoles) {
		return nil
	}
	for _, f := range sites {
		if !hasSubstance(ix, f) {
			return nil
		}
	}
	breakdown := breakdownOf(sites, cluster, 0)
	holes := "nothing (exact clone)"
	if len(cluster.Columns) > 0 {
		holes = columnsText(ix, cluster.Columns)
	}
	sitesList := sitesOfFacts(sites)
	return &Candidate{
		Kind:             Parameterize,
		ScoreMilli:       ScoreMilli(breakdown, false) / 3,
		Breakdown:        breakdown,
		Observation:      summaryOf(ix, sites, cluster),
		Inference:        "the same computation is written out once per site",
		PossibleRefactor: fmt.Sprintf("extract one helper and pass the differing parts as parameters: %s", holes),
		Sites:            sitesList,
		FixSpec: &FixSpec{
			Kind:    Parameterize,
			File:    sitesList[0].Path,
			Line:    sitesList[0].Line,
			EndLine: sitesList[0].EndLine,
			Params:  map[string]string{},
		},
	}
}

// withoutTrait handles a cluster where no method hole joined a signature
// group: a type-only generic fn, a hidden pattern, or nothing.
func withoutTrait(ix *candidateIndex, sites []*FuncFacts, cluster *Cluster, hidden *string) outcome {
	if hidden != nil {
		if strings.HasPrefix(*hidden, "all ") {
			return outcome{kind: outcomeHidden, suppressed: &Suppressed{
				Reason: *hidden,
				Sites:  sitesOfFacts(sites),
			}}
		}
		return outcome{kind: outcomeNothing}
	}
	if c := genericFn(ix, sites, cluster); c != nil {
		return outcome{kind: outcomeReady, candidate: c}
	}
	if c := parameterize(ix, sites, cluster); c != nil {
		return outcome{kind: outcomeReady, candidate: c}
	}
	return outcome{kind: outcomeNothing}
}

func fromCluster(ix *candidateIndex, fns []*FuncFacts, cluster *Cluster) outcome {
	sites := sitesOf(fns, cluster)
	var cols []*Column
	for i := range cluster.Columns {
		if methodLike(cluster.Columns[i].Kind) {
			cols = append(cols, &cluster.Columns[i])
		}
	}
	verdicts, hidden := judgeColumns(ix, cols)
	if len(verdicts) == 0 {
		var reason *string
		if len(hidden) > 0 {
			reason = &hidden[len(hidden)-1].reason
		}
		return withoutTrait(ix, sites, cluster, reason)
	}
	kind := TraitMethod
	if len(verdicts) > 1 {
		kind = CapabilitySet
	}
	return outcome{kind: outcomeFound, ev: &candidateEvidence{
		kind:          kind,
		breakdown:     breakdownOf(sites, cluster, liftOf(verdicts)),
		sites:         sites,
		verdicts:      verdicts,
		summary:       summaryOf(ix, sites, cluster),
		existingTrait: existingTraitFor(verdicts, hidden),
	}}
}

func ownersOf(defs []*FuncFacts) map[string]bool {
	return ownerSet(defs)
}

// existingTraitFor finds the suppressed column whose interface is
// implemented by the same owner types as the proposed method: that is the
// interface to extend (an unrelated one in the cluster is not).
func existingTraitFor(verdicts []traitVerdict, hidden []existing) string {
	if len(verdicts) == 0 {
		return ""
	}
	wanted := ownersOf(verdicts[0].defs)
	for _, e := range hidden {
		if equalStringSets(wanted, e.owners) {
			return e.traitMethod
		}
	}
	return ""
}

// fromDispatch judges one type-switch dispatch: the arm callee ids against
// the layer-1 groups.
func fromDispatch(ix *candidateIndex, f *FuncFacts, d *Dispatch) outcome {
	values := make([]string, len(d.Arms))
	for i, a := range d.Arms {
		values[i] = a.CalleeID
	}
	shown := make([]string, len(values))
	for i, v := range values {
		shown[i] = ix.displayCallee(v)
	}
	// Go adaptation: rstyle matches on an enum; the Go detector finds type
	// switches.
	summary := fmt.Sprintf("`%s` switches on a type and calls analogous methods per arm: M0 = %s",
		f.Name, strings.Join(shown, " | "))
	switch v := judge(ix, values); v.kind {
	case verdictTrait:
		return outcome{kind: outcomeFound, ev: &candidateEvidence{
			kind: EnumDispatch,
			breakdown: Breakdown{
				Support:       len(d.Arms),
				LiftMilli:     v.group.ScoreMilli,
				Holes:         1,
				CoverageMilli: 1000,
			},
			sites:    []*FuncFacts{f},
			verdicts: []traitVerdict{{group: v.group, defs: v.defs}},
			summary:  summary,
		}}
	case verdictSuppressed:
		return outcome{kind: outcomeHidden, suppressed: &Suppressed{
			Reason: v.existing.reason,
			Sites:  []Site{makeSite(f)},
		}}
	default:
		return outcome{kind: outcomeNothing}
	}
}

func compareSite(a, b Site) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Line != b.Line {
		switch {
		case a.Line < b.Line:
			return -1
		case a.Line > b.Line:
			return 1
		}
	}
	if a.ID != b.ID {
		return strings.Compare(a.ID, b.ID)
	}
	return strings.Compare(a.Name, b.Name)
}

func compareSites(a, b []Site) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareSite(a[i], b[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// collectOutcomes folds outcomes into a Mined, best first; ties by site, so
// output never depends on input order.
// sortMined orders candidates by score then sites, suppressed by sites then reason.
func sortMined(mined *Mined) {
	sort.SliceStable(mined.Candidates, func(i, j int) bool {
		a, b := mined.Candidates[i], mined.Candidates[j]
		if a.ScoreMilli != b.ScoreMilli {
			return a.ScoreMilli > b.ScoreMilli
		}
		return compareSites(a.Sites, b.Sites) < 0
	})
	sort.SliceStable(mined.Suppressed, func(i, j int) bool {
		a, b := mined.Suppressed[i], mined.Suppressed[j]
		if c := compareSites(a.Sites, b.Sites); c != 0 {
			return c < 0
		}
		return a.Reason < b.Reason
	})
}

func collectOutcomes(outcomes []outcome) Mined {
	mined := Mined{}
	for _, o := range outcomes {
		switch o.kind {
		case outcomeFound:
			mined.Candidates = append(mined.Candidates, finish(o.ev))
		case outcomeReady:
			mined.Candidates = append(mined.Candidates, *o.candidate)
		case outcomeHidden:
			mined.Suppressed = append(mined.Suppressed, *o.suppressed)
		}
	}
	sortMined(&mined)
	return mined
}

// withPdgs keeps the facts that have a PDG, sorted by (path, line, id) so
// mining never depends on input order.
func withPdgs(facts []*FuncFacts) []*FuncFacts {
	var fns []*FuncFacts
	for _, f := range facts {
		if f != nil && f.Pdg != nil {
			fns = append(fns, f)
		}
	}
	sort.SliceStable(fns, func(i, j int) bool {
		a, b := fns[i], fns[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.ID < b.ID
	})
	return fns
}

// Mine is the layer-2 entry: candidates and suppressed patterns for facts,
// given the layer-1 signature groups and the clustering params.
func Mine(facts []*FuncFacts, groups []SigGroup, params Params) Mined {
	ix := newCandidateIndex(facts, groups, params)
	fns := withPdgs(facts)
	pdgs := make([]*Pdg, len(fns))
	for i, f := range fns {
		pdgs[i] = f.Pdg
	}
	var outcomes []outcome
	for _, c := range ClusterPdgs(pdgs, params) {
		cluster := c
		outcomes = append(outcomes, fromCluster(ix, fns, &cluster))
	}
	for i, f := range fns {
		for _, d := range FindDispatch(pdgs[i]) {
			dispatch := d
			outcomes = append(outcomes, fromDispatch(ix, f, &dispatch))
		}
	}
	return collectOutcomes(outcomes)
}
