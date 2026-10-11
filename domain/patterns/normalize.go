// Canonicalizing desugar pass over dependence graphs.
//
// Port of rstyle-core's normalize.rs: two functions that do the same thing but
// spell control flow or value construction differently have different graphs,
// and no scorer can fix a different shape. Each rule rewrites one spelling
// into a shared core form before hashing and aligning. Rules are pure
// MiningGraph -> MiningGraph functions, applied in a fixed order, switched on one by one.
//
// Invariants (tested): idempotent, node count never grows, original `detail`
// text survives on the rewritten node so holes can still be reported.
//
// Go adaptations (each documented at its use site):
//   - Op details carry a "class:" prefix in the Go schema (e.g. "cmp:<"),
//     while rstyle's are bare ("<"). Operator predicates compare the suffix
//     after the first ':', so both spellings work.
//   - Composite literals ("composite:{}") count as ctor ops: they are Go's
//     value-construction spelling.
//   - `i++` / `i--` ("inc:++" / "dec:--") count as counter steps alongside
//     `n += 1`, or the counter rule could never fire on Go loops.
//   - The propagate rule also recognizes residual Go error-propagate shapes
//     the extractor did not fold into Try (`if <x> != nil { return <x> }`
//     with nil on either side, `if <x> == nil {..} else { return <x> }`).
//     Only Branch nodes are examined, so existing Try nodes are never
//     double-converted.
//   - Defer/Go nodes are opaque like Closure: no rule deletes a Call node or
//     rewrites an edge into one, so the Call -> Defer/Go wrapping survives.
//   - The Go schema has no Macro kind and no macro_name field; drop_unreachable
//     matches Kind=="macro" with "unreachable" in the detail, inert on
//     extractor output but harmless if a macro node ever appears.
//   - is_unit_variant (Rust path literals) is ported verbatim; inert, since
//     the Go extractor never emits "path" literals.
package patterns

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Rules selects which canonicalizing rewrites run. Explicit value, no
// globals: callers pass it down.
type Rules struct {
	// Ctor is R1: value construction collapses into one Ctor node.
	Ctor bool
	// Pred is R2: &&, ||, ! over predicate calls disappear; the calls feed
	// the consumer.
	Pred bool
	// Case is R3: if, if let, match, == literal conditions and else-if
	// chains become one Case.
	Case bool
	// Exit is R4: unreachable!() after a loop is dropped.
	Exit bool
	// Counter is R5/R6: range for loops, while/loop with a manual counter
	// and while i < N become one Loop whose node is the counter.
	Counter bool
	// Cmp is R7: comparing the counter with a literal is one cmp operator
	// whatever the operator.
	Cmp bool
	// Propagate is R8: match r { Ok(..) => .., Err(e) => return Err(e) }
	// is the ? operator.
	Propagate bool
}

// RulesAll enables every rule. (A var: Go cannot const a struct.)
var RulesAll = Rules{
	Ctor: true, Pred: true, Case: true, Exit: true,
	Counter: true, Cmp: true, Propagate: true,
}

// IsNone reports whether no rule is enabled.
func (r Rules) IsNone() bool { return r == Rules{} }

// ruleSetters maps a rule name to the field it enables. "all" resets first,
// mirroring the Rust struct-update order.
var ruleSetters = map[string]func(*Rules){
	"all":     func(r *Rules) { *r = RulesAll },
	"ctor":    func(r *Rules) { r.Ctor = true },
	"pred":    func(r *Rules) { r.Pred = true },
	"case":    func(r *Rules) { r.Case = true },
	"exit":    func(r *Rules) { r.Exit = true },
	"counter": func(r *Rules) { r.Counter = true },
	"cmp":     func(r *Rules) { r.Cmp = true },
	"try":     func(r *Rules) { r.Propagate = true },
}

// ParseRules parses "ctor,pred", "all" or "". Unknown names are an error,
// mirroring the Rust messages.
func ParseRules(text string) (Rules, error) {
	rules := Rules{}
	for _, name := range strings.Split(text, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		set, ok := ruleSetters[name]
		if !ok {
			return Rules{}, fmt.Errorf(
				"unknown normalize rule `%s` (ctor, pred, case, exit, counter, cmp, try, all)", name)
		}
		set(&rules)
	}
	return rules, nil
}

// Canonicalize applies the enabled rules in a fixed order (R1, R2, ...).
// The input is never mutated: it is cloned up front and every rule maps
// MiningGraph to MiningGraph, like the Rust originals.
func Canonicalize(pdg *MiningGraph, rules Rules) *MiningGraph {
	steps := []struct {
		on   bool
		step func(*MiningGraph) *MiningGraph
	}{
		{rules.Propagate, propagateErrors},
		{rules.Ctor, foldCtors},
		{rules.Pred, collapsePreds},
		{rules.Case, unifyCase},
		{rules.Exit, dropUnreachable},
		{rules.Counter, counterLoops},
		{rules.Cmp, compareClass},
	}
	out := clonePdg(pdg)
	for _, s := range steps {
		if s.on {
			out = s.step(out)
		}
	}
	out.Paper = pdg.Paper
	out.baseLabels = pdg.baseLabels
	return out
}

func clonePdg(pdg *MiningGraph) *MiningGraph {
	out := &MiningGraph{
		baseLabels: pdg.baseLabels,
		Paper:      pdg.Paper,
		Nodes:      make([]PdgNode, len(pdg.Nodes)),
		Edges:      make([]PdgEdge, len(pdg.Edges)),
	}
	copy(out.Nodes, pdg.Nodes)
	copy(out.Edges, pdg.Edges)
	return out
}

// ---- graph helpers -----------------------------------------------------------

func dataProducers(pdg *MiningGraph, node int) []int {
	var out []int
	for _, e := range pdg.Edges {
		if e.To == node && e.Kind == Data {
			out = append(out, e.From)
		}
	}
	return out
}

func dataConsumers(pdg *MiningGraph, node int) map[int]struct{} {
	out := map[int]struct{}{}
	for _, e := range pdg.Edges {
		if e.From == node && e.Kind == Data {
			out[e.To] = struct{}{}
		}
	}
	return out
}

// ownedBy reports whether consumer is node's only data consumer.
func ownedBy(pdg *MiningGraph, node, consumer int) bool {
	c := dataConsumers(pdg, node)
	_, ok := c[consumer]
	return len(c) == 1 && ok
}

// ctrlParent is the single control-dependence parent of node, if any.
func ctrlParent(pdg *MiningGraph, node int) (int, bool) {
	for _, e := range pdg.Edges {
		if e.To == node && e.Kind == Ctrl {
			return e.From, true
		}
	}
	return 0, false
}

// dataOuts collects the data edges leaving from, in edge order.
func dataOuts(pdg *MiningGraph, from int) []PdgEdge {
	var outs []PdgEdge
	for _, e := range pdg.Edges {
		if e.From == from && e.Kind == Data {
			outs = append(outs, e)
		}
	}
	return outs
}

func reindexNodes(count int, dead map[int]struct{}) []int {
	newIndex := make([]int, count)
	next := 0
	for old := 0; old < count; old++ {
		if _, gone := dead[old]; gone {
			newIndex[old] = -1
			continue
		}
		newIndex[old] = next
		next++
	}
	return newIndex
}

func reindexEdge(e PdgEdge, newIndex []int) (PdgEdge, bool) {
	from, to := newIndex[e.From], newIndex[e.To]
	if from < 0 || to < 0 {
		return e, false
	}
	e.From, e.To = from, to
	return e, true
}

// without removes dead nodes and every edge touching them; indices are
// compacted in order.
func without(pdg *MiningGraph, dead map[int]struct{}) *MiningGraph {
	newIndex := reindexNodes(len(pdg.Nodes), dead)
	out := &MiningGraph{}
	for old, n := range pdg.Nodes {
		if newIndex[old] >= 0 {
			out.Nodes = append(out.Nodes, n)
		}
	}
	for _, e := range pdg.Edges {
		if ne, ok := reindexEdge(e, newIndex); ok {
			out.Edges = append(out.Edges, ne)
		}
	}
	return out
}

// dedupEdges drops duplicate edges. First occurrence wins, so adding an
// edge that exists is a no-op.
func dedupEdges(edges []PdgEdge) []PdgEdge {
	seen := make(map[PdgEdge]struct{}, len(edges))
	out := make([]PdgEdge, 0, len(edges))
	for _, e := range edges {
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out
}

// opSuffix returns the operator after the class prefix in an Op detail.
// Go details are always "class:op" (e.g. "cmp:<"); rstyle's are bare ("<").
// Comparing the suffix keeps the Rust operator predicates meaningful on
// both spellings.
func opSuffix(detail string) string {
	if i := strings.IndexByte(detail, ':'); i >= 0 {
		return detail[i+1:]
	}
	return detail
}

// detailClass returns the class before the colon, or "" when there is none.
func detailClass(detail string) string {
	if i := strings.IndexByte(detail, ':'); i >= 0 {
		return detail[:i]
	}
	return ""
}

// ---- R1: value construction --------------------------------------------------

// isCtorOp matches rstyle's ctor/tuple ops, plus Go's composite literals:
// "composite:{}" is Go's value-construction spelling.
func isCtorOp(n *PdgNode) bool {
	if n.Kind != Op {
		return false
	}
	return detailClass(n.Detail) == "composite" ||
		opSuffix(n.Detail) == "ctor" || opSuffix(n.Detail) == "tuple"
}

// isUnitVariant mirrors the Rust check verbatim: a path literal naming a
// variant (last segment capitalized, not a CONST). Inert on Go graphs: the
// extractor never emits "path" literals.
func isUnitVariant(n *PdgNode) bool {
	if n.Kind != Lit || n.LitKind != "path" || n.CalleeID != "" {
		return false
	}
	return isVariantName(variantLast(n.Detail))
}

func variantLast(detail string) string {
	if i := strings.LastIndex(detail, "::"); i >= 0 {
		return detail[i+2:]
	}
	return detail
}

func isVariantName(last string) bool {
	rs := []rune(last)
	return len(rs) > 0 && unicode.IsUpper(rs[0]) && slices.ContainsFunc(rs, unicode.IsLower)
}

// isValueLeaf: literal operands and ctor nodes, the only things a constant
// value is built from.
func isValueLeaf(n *PdgNode) bool {
	return n.Kind == Lit && n.CalleeID == ""
}

// constantTree is true if node is a ctor/tuple whose whole input tree is
// literals and ctors owned by it.
func constantTree(pdg *MiningGraph, node int) bool {
	if !isCtorOp(&pdg.Nodes[node]) {
		return false
	}
	for _, p := range dataProducers(pdg, node) {
		if !ownedBy(pdg, p, node) {
			return false
		}
		if !isValueLeaf(&pdg.Nodes[p]) && !constantTree(pdg, p) {
			return false
		}
	}
	return true
}

// constantRoot: a constant tree no constant ctor consumes.
func constantRoot(pdg *MiningGraph, node int) bool {
	if !constantTree(pdg, node) {
		return false
	}
	for c := range dataConsumers(pdg, node) {
		if constantTree(pdg, c) {
			return false
		}
	}
	return true
}

// builtFrom: node and everything it was built from.
func builtFrom(pdg *MiningGraph, node int) map[int]struct{} {
	out := map[int]struct{}{node: {}}
	for _, p := range dataProducers(pdg, node) {
		for n := range builtFrom(pdg, p) {
			out[n] = struct{}{}
		}
	}
	return out
}

func foldRoots(pdg *MiningGraph) []int {
	var roots []int
	for n := range pdg.Nodes {
		if constantRoot(pdg, n) {
			roots = append(roots, n)
		}
	}
	return roots
}

// absorbedBy maps each absorbed node to its constant root. Distinct roots
// have disjoint built_from sets (a shared node would break the ownership
// check), so iteration order cannot change the mapping.
func absorbedBy(pdg *MiningGraph, roots []int) map[int]int {
	rootOf := map[int]int{}
	for _, r := range roots {
		for n := range builtFrom(pdg, r) {
			if n != r {
				rootOf[n] = r
			}
		}
	}
	return rootOf
}

// movedCtrl: control that reached an absorbed node now reaches the folded value.
func movedCtrl(pdg *MiningGraph, rootOf map[int]int) []PdgEdge {
	var moved []PdgEdge
	for _, e := range pdg.Edges {
		root, ok := rootOf[e.To]
		if ok && e.Kind == Ctrl {
			e.To = root
			moved = append(moved, e)
		}
	}
	return moved
}

func foldNode(n *PdgNode, isRoot bool) PdgNode {
	if isRoot || isUnitVariant(n) {
		return asCtor(n)
	}
	return *n
}

func foldNodes(pdg *MiningGraph, roots []int) []PdgNode {
	inRoots := make(map[int]bool, len(roots))
	for _, r := range roots {
		inRoots[r] = true
	}
	nodes := make([]PdgNode, len(pdg.Nodes))
	for i := range pdg.Nodes {
		nodes[i] = foldNode(&pdg.Nodes[i], inRoots[i])
	}
	return nodes
}

func asCtor(n *PdgNode) PdgNode {
	c := *n
	c.Kind = Ctor
	return c
}

// foldCtors folds each constant value (ctor trees over literals, unit
// tuples) into one Ctor leaf and turns unit-variant literals into Ctor too.
// Non-constant ctor chains merge into the outermost ctor.
func foldCtors(pdg *MiningGraph) *MiningGraph {
	roots := foldRoots(pdg)
	rootOf := absorbedBy(pdg, roots)
	dead := make(map[int]struct{}, len(rootOf))
	for n := range rootOf {
		dead[n] = struct{}{}
	}
	edges := dedupEdges(append(append([]PdgEdge{}, pdg.Edges...), movedCtrl(pdg, rootOf)...))
	return spliceNested(without(&MiningGraph{Nodes: foldNodes(pdg, roots), Edges: edges}, dead))
}

// isNestedCtor: a ctor feeding only another ctor.
func isNestedCtor(pdg *MiningGraph, n int) bool {
	if !isCtorOp(&pdg.Nodes[n]) {
		return false
	}
	parents := dataConsumers(pdg, n)
	if len(parents) != 1 {
		return false
	}
	for p := range parents {
		return isCtorOp(&pdg.Nodes[p])
	}
	return false
}

func innerCtors(pdg *MiningGraph) map[int]struct{} {
	inner := map[int]struct{}{}
	for n := range pdg.Nodes {
		if isNestedCtor(pdg, n) {
			inner[n] = struct{}{}
		}
	}
	return inner
}

// outermostCtor: the outermost ctor above n (chains merge all the way up).
// inner nodes have exactly one data consumer by construction.
func outermostCtor(pdg *MiningGraph, inner map[int]struct{}, n int) int {
	for {
		if _, ok := inner[n]; !ok {
			return n
		}
		for p := range dataConsumers(pdg, n) {
			n = p
		}
	}
}

func spliceEdge(pdg *MiningGraph, e PdgEdge, inner map[int]struct{}) (PdgEdge, bool) {
	_, toInner := inner[e.To]
	_, fromInner := inner[e.From]
	if fromInner {
		return PdgEdge{}, false
	}
	if toInner && e.Kind == Data {
		e.To = outermostCtor(pdg, inner, e.To)
	}
	return e, true
}

func spliceEdges(pdg *MiningGraph, inner map[int]struct{}) []PdgEdge {
	var moved []PdgEdge
	for _, e := range pdg.Edges {
		if ne, ok := spliceEdge(pdg, e, inner); ok {
			moved = append(moved, ne)
		}
	}
	return moved
}

// spliceNested: a ctor feeding only another ctor merges into it; its inputs
// become the parent's inputs. Folded constant roots already have kind Ctor
// (not Op), so isCtorOp deliberately skips them here.
func spliceNested(pdg *MiningGraph) *MiningGraph {
	inner := innerCtors(pdg)
	return without(&MiningGraph{Nodes: pdg.Nodes, Edges: dedupEdges(spliceEdges(pdg, inner))}, inner)
}

// ---- R2: predicate collapse --------------------------------------------------

func isCombinator(n *PdgNode) bool {
	if n.Kind != Op {
		return false
	}
	switch opSuffix(n.Detail) {
	case "&&", "||", "!":
		return true
	}
	return false
}

// predicateTree: a combinator whose operands are calls or (recursively)
// such combinators.
func predicateTree(pdg *MiningGraph, node int) bool {
	if !isCombinator(&pdg.Nodes[node]) {
		return false
	}
	for _, p := range dataProducers(pdg, node) {
		if pdg.Nodes[p].Kind != Call && !predicateTree(pdg, p) {
			return false
		}
	}
	return true
}

type predLeaf struct {
	node    int
	negated bool
}

// predLeaves: call leaves under node, each with whether an odd number of
// ! sits above it.
func predLeaves(pdg *MiningGraph, node int, negated bool) []predLeaf {
	flip := negated != (opSuffix(pdg.Nodes[node].Detail) == "!")
	var out []predLeaf
	for _, p := range dataProducers(pdg, node) {
		if isCombinator(&pdg.Nodes[p]) {
			out = append(out, predLeaves(pdg, p, flip)...)
		} else {
			out = append(out, predLeaf{p, flip})
		}
	}
	return out
}

func predTrees(pdg *MiningGraph) map[int]struct{} {
	trees := map[int]struct{}{}
	for n := range pdg.Nodes {
		if predicateTree(pdg, n) {
			trees[n] = struct{}{}
		}
	}
	return trees
}

func treeRoot(pdg *MiningGraph, n int, trees map[int]struct{}) bool {
	for c := range dataConsumers(pdg, n) {
		if _, ok := trees[c]; ok {
			return false
		}
	}
	return true
}

func predRoots(pdg *MiningGraph, trees map[int]struct{}) []int {
	var roots []int
	for n := range trees {
		if treeRoot(pdg, n, trees) {
			roots = append(roots, n)
		}
	}
	slices.Sort(roots)
	return roots
}

func negatedLeaves(pdg *MiningGraph, roots []int) map[int]struct{} {
	negated := map[int]struct{}{}
	for _, r := range roots {
		for _, l := range predLeaves(pdg, r, false) {
			if l.negated {
				negated[l.node] = struct{}{}
			}
		}
	}
	return negated
}

func markNegated(pdg *MiningGraph, negated map[int]struct{}) []PdgNode {
	nodes := make([]PdgNode, len(pdg.Nodes))
	for i := range pdg.Nodes {
		n := pdg.Nodes[i]
		if _, ok := negated[i]; ok {
			n.Detail = "!"
		}
		nodes[i] = n
	}
	return nodes
}

func rewirePredRoot(pdg *MiningGraph, root int) []PdgEdge {
	outs := dataOuts(pdg, root)
	var rewired []PdgEdge
	for _, l := range predLeaves(pdg, root, false) {
		for _, o := range outs {
			o.From = l.node
			o.ArgPos = 0
			rewired = append(rewired, o)
		}
	}
	return rewired
}

func rewirePreds(pdg *MiningGraph, roots []int) []PdgEdge {
	var rewired []PdgEdge
	for _, r := range roots {
		rewired = append(rewired, rewirePredRoot(pdg, r)...)
	}
	return rewired
}

// collapsePreds: `if !a.ok() && !b.ok()` and `if a.ok()` differ only by
// calls: the combinator nodes go, the calls feed the consumer directly
// (negation survives as detail "!" on the call).
func collapsePreds(pdg *MiningGraph) *MiningGraph {
	trees := predTrees(pdg)
	roots := predRoots(pdg, trees)
	nodes := markNegated(pdg, negatedLeaves(pdg, roots))
	edges := dedupEdges(append(append([]PdgEdge{}, pdg.Edges...), rewirePreds(pdg, roots)...))
	return without(&MiningGraph{Nodes: nodes, Edges: edges}, trees)
}

// ---- R3: one multiway decision -----------------------------------------------

func isBranching(n *PdgNode) bool {
	return n.Kind == Branch || n.Kind == Match
}

func letBranches(pdg *MiningGraph) map[int]int {
	branchOf := map[int]int{}
	for _, e := range pdg.Edges {
		if e.Kind == Data && pdg.Nodes[e.From].Kind == Let && pdg.Nodes[e.To].Kind == Branch {
			branchOf[e.From] = e.To
		}
	}
	return branchOf
}

// contractEdge rewrites one edge for a contracted Let; ok=false drops it.
func contractEdge(e PdgEdge, branchOf map[int]int) (PdgEdge, bool) {
	if b, ok := branchOf[e.From]; ok {
		if e.To == b {
			return PdgEdge{}, false // the Let -> Branch edge itself goes
		}
		e.From = b
		return e, true
	}
	if b, ok := branchOf[e.To]; ok {
		if e.Kind != Data {
			return PdgEdge{}, false // control into the Let goes
		}
		e.To = b
		return e, true
	}
	return e, true
}

// contractLets: `if let P = e {..}` — the destructuring Let feeding a Branch
// is the scrutinee of a match. The Let goes away; its input and its bindings
// attach to the branch.
func contractLets(pdg *MiningGraph) *MiningGraph {
	branchOf := letBranches(pdg)
	dead := make(map[int]struct{}, len(branchOf))
	for l := range branchOf {
		dead[l] = struct{}{}
	}
	moved := make([]PdgEdge, 0, len(pdg.Edges))
	for _, e := range pdg.Edges {
		if ne, ok := contractEdge(e, branchOf); ok {
			moved = append(moved, ne)
		}
	}
	return without(&MiningGraph{Nodes: pdg.Nodes, Edges: dedupEdges(moved)}, dead)
}

// eqOp: the single == comparison feeding branch b, if any.
func eqOp(pdg *MiningGraph, b int) (int, bool) {
	cond := dataProducers(pdg, b)
	if len(cond) != 1 {
		return 0, false
	}
	op := cond[0]
	if pdg.Nodes[op].Kind != Op || opSuffix(pdg.Nodes[op].Detail) != "==" {
		return 0, false
	}
	return op, true
}

// litOperand splits the comparison inputs into the literal and the scrutinee.
func litOperand(pdg *MiningGraph, op int) (lit, x int, ok bool) {
	inputs := dataProducers(pdg, op)
	if len(inputs) != 2 {
		return 0, 0, false
	}
	p, q := inputs[0], inputs[1]
	if isValueLeaf(&pdg.Nodes[p]) {
		return p, q, true
	}
	if isValueLeaf(&pdg.Nodes[q]) {
		return q, p, true
	}
	return 0, 0, false
}

func litOwned(pdg *MiningGraph, op, lit, b, x int) bool {
	return ownedBy(pdg, lit, op) && ownedBy(pdg, op, b) && !isValueLeaf(&pdg.Nodes[x])
}

func litScrutineeOp(pdg *MiningGraph, b int) (op, lit, x int, ok bool) {
	op, ok = eqOp(pdg, b)
	if !ok {
		return 0, 0, 0, false
	}
	lit, x, ok = litOperand(pdg, op)
	if !ok || !litOwned(pdg, op, lit, b, x) {
		return 0, 0, 0, false
	}
	return op, lit, x, true
}

// litScrutinee: `if x == 3` — the comparison with a literal is the pattern
// of a match arm, not structure.
func litScrutinee(pdg *MiningGraph, b int) (op, lit, x int, ok bool) {
	if pdg.Nodes[b].Kind != Branch {
		return 0, 0, 0, false
	}
	return litScrutineeOp(pdg, b)
}

type litHit struct{ b, op, lit, x int }

func litScrutineeHits(pdg *MiningGraph) []litHit {
	var hits []litHit
	for b := range pdg.Nodes {
		if op, lit, x, ok := litScrutinee(pdg, b); ok {
			hits = append(hits, litHit{b, op, lit, x})
		}
	}
	return hits
}

// literalScrutinees: the branch becomes a decision on x alone.
func literalScrutinees(pdg *MiningGraph) *MiningGraph {
	hits := litScrutineeHits(pdg)
	dead := make(map[int]struct{}, 2*len(hits))
	extra := make([]PdgEdge, 0, len(hits))
	for _, h := range hits {
		dead[h.op] = struct{}{}
		dead[h.lit] = struct{}{}
		extra = append(extra, PdgEdge{From: h.x, To: h.b, Kind: Data, ArgPos: 0})
	}
	edges := dedupEdges(append(append([]PdgEdge{}, pdg.Edges...), extra...))
	return without(&MiningGraph{Nodes: pdg.Nodes, Edges: edges}, dead)
}

func armsOf(pdg *MiningGraph, c int) []PdgEdge {
	var out []PdgEdge
	for _, e := range pdg.Edges {
		if e.From == c && e.Kind == Ctrl {
			out = append(out, e)
		}
	}
	return out
}

func lastArm(arms []PdgEdge) (int, bool) {
	last := -1
	for _, e := range arms {
		if e.ArgPos > last {
			last = e.ArgPos
		}
	}
	return last, last >= 0
}

func armTargets(arms []PdgEdge, arm int) []int {
	var out []int
	for _, e := range arms {
		if e.ArgPos == arm {
			out = append(out, e.To)
		}
	}
	return out
}

func sameScrutineeAlone(pdg *MiningGraph, p, c int) bool {
	pp, cc := dataProducers(pdg, p), dataProducers(pdg, c)
	return pdg.Nodes[c].Kind == Case && len(pp) > 0 &&
		slices.Equal(pp, cc) && len(dataConsumers(pdg, c)) == 0
}

func elseIfChild(pdg *MiningGraph, p int) (parent, child, arm int, ok bool) {
	arms := armsOf(pdg, p)
	last, ok := lastArm(arms)
	if !ok {
		return 0, 0, 0, false
	}
	tail := armTargets(arms, last)
	if len(tail) != 1 || !sameScrutineeAlone(pdg, p, tail[0]) {
		return 0, 0, 0, false
	}
	return p, tail[0], last, true
}

// elseIf: a case sitting alone in the last arm of another case on the same
// scrutinee: (parent, child, arm).
func elseIf(pdg *MiningGraph) (parent, child, arm int, ok bool) {
	for p := range pdg.Nodes {
		if pdg.Nodes[p].Kind != Case {
			continue
		}
		if par, ch, a, ok := elseIfChild(pdg, p); ok {
			return par, ch, a, true
		}
	}
	return 0, 0, 0, false
}

func elseIfKept(pdg *MiningGraph, child int) []PdgEdge {
	var kept []PdgEdge
	for _, e := range pdg.Edges {
		if e.From == child || e.To == child {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

func elseIfLifted(pdg *MiningGraph, parent, child, arm int) []PdgEdge {
	var lifted []PdgEdge
	for _, e := range pdg.Edges {
		if e.From == child && e.Kind == Ctrl {
			e.From = parent
			e.ArgPos = arm + e.ArgPos
			lifted = append(lifted, e)
		}
	}
	return lifted
}

// flattenElseIf: `if x == 1 {..} else if x == 2 {..} else {..}` is one case
// with three arms.
func flattenElseIf(pdg *MiningGraph) *MiningGraph {
	parent, child, arm, ok := elseIf(pdg)
	if !ok {
		return pdg
	}
	edges := dedupEdges(append(elseIfKept(pdg, child), elseIfLifted(pdg, parent, child, arm)...))
	return flattenElseIf(without(&MiningGraph{Nodes: pdg.Nodes, Edges: edges},
		map[int]struct{}{child: {}}))
}

func relabelCases(pdg *MiningGraph) *MiningGraph {
	nodes := make([]PdgNode, len(pdg.Nodes))
	for i := range pdg.Nodes {
		n := pdg.Nodes[i]
		if isBranching(&n) {
			n.Kind = Case
		}
		nodes[i] = n
	}
	return &MiningGraph{Nodes: nodes, Edges: pdg.Edges}
}

// unifyCase: `if`, `else if`, `if let` and `match` all become Case; patterns
// spelled as == literal and else-if chains over one scrutinee disappear into
// the arms.
func unifyCase(pdg *MiningGraph) *MiningGraph {
	return flattenElseIf(relabelCases(literalScrutinees(contractLets(pdg))))
}

// ---- R4: exits -----------------------------------------------------------------

// macroKind is rstyle's Macro node kind. The Go schema has no Macro kind and
// no macro_name field, so this never matches extractor output; the rule is
// kept so a front end that does emit macro nodes still gets the R4
// treatment. The detail text stands in for Rust's macro_name.
const macroKind NodeKind = "macro"

// dropUnreachable: unreachable!() closes a `for` that always returns inside;
// the loop form has no such tail.
func dropUnreachable(pdg *MiningGraph) *MiningGraph {
	dead := map[int]struct{}{}
	for i := range pdg.Nodes {
		n := &pdg.Nodes[i]
		if n.Kind == macroKind && strings.Contains(n.Detail, "unreachable") &&
			len(dataProducers(pdg, i)) == 0 {
			dead[i] = struct{}{}
		}
	}
	return without(pdg, dead)
}

// ---- R8: error propagation -----------------------------------------------------

// matchArms: the distinct ctrl-arm targets of a match, sorted.
func matchArms(pdg *MiningGraph, m int) []int {
	set := map[int]struct{}{}
	for _, e := range armsOf(pdg, m) {
		set[e.To] = struct{}{}
	}
	arms := make([]int, 0, len(set))
	for a := range set {
		arms = append(arms, a)
	}
	slices.Sort(arms)
	return arms
}

func propagationShape(pdg *MiningGraph, c, r int) bool {
	return isCtorOp(&pdg.Nodes[c]) && pdg.Nodes[r].Kind == Return
}

func propagationWiring(pdg *MiningGraph, m, c, r int) bool {
	pc := dataProducers(pdg, c)
	pr := dataProducers(pdg, r)
	return len(pc) == 1 && pc[0] == m && ownedBy(pdg, c, r) &&
		len(pr) == 1 && pr[0] == c
}

// propagation checks one Match for the `?` shape: two ctrl arms, one a bare
// Return fed only by a ctor fed only by the match. Returns (ctor, return).
func propagation(pdg *MiningGraph, m int) (c, r int, ok bool) {
	arms := matchArms(pdg, m)
	if len(arms) != 2 {
		return 0, 0, false
	}
	c, r = arms[0], arms[1]
	if pdg.Nodes[c].Kind == Return {
		c, r = r, c
	}
	if !propagationShape(pdg, c, r) || !propagationWiring(pdg, m, c, r) {
		return 0, 0, false
	}
	return c, r, true
}

// propagations: (match, err ctor, return) of every match whose only job
// besides yielding the Ok value is Err(e) => return Err(e).
func propagations(pdg *MiningGraph) [][3]int {
	var out [][3]int
	for m := range pdg.Nodes {
		if pdg.Nodes[m].Kind != Match {
			continue
		}
		if c, r, ok := propagation(pdg, m); ok {
			out = append(out, [3]int{m, c, r})
		}
	}
	return out
}

// goProp is a residual Go error-propagate shape: the branch, the !=/==
// comparison, the nil literal, the checked value, and the return.
type goProp struct {
	branch int
	op     int
	nilLit int
	x      int
	ret    int
}

func isNilLit(n *PdgNode) bool {
	return n.Kind == Lit && n.LitKind == "nil" && n.CalleeID == ""
}

// nilCheckOp: the single != / == comparison feeding branch b.
// neq reports the != polarity (error arm 0); == uses error arm 1.
func nilCheckOp(pdg *MiningGraph, b int) (op int, neq bool, ok bool) {
	cond := dataProducers(pdg, b)
	if len(cond) != 1 {
		return 0, false, false
	}
	op = cond[0]
	if pdg.Nodes[op].Kind != Op {
		return 0, false, false
	}
	switch opSuffix(pdg.Nodes[op].Detail) {
	case "!=":
		return op, true, true
	case "==":
		return op, false, true
	}
	return 0, false, false
}

// nilCheckOperands splits the comparison into the checked value and the
// nil literal (nil on either side).
func nilCheckOperands(pdg *MiningGraph, op int) (x, nilLit int, ok bool) {
	inputs := dataProducers(pdg, op)
	if len(inputs) != 2 {
		return 0, 0, false
	}
	p, q := inputs[0], inputs[1]
	if isNilLit(&pdg.Nodes[p]) {
		return q, p, true
	}
	if isNilLit(&pdg.Nodes[q]) {
		return p, q, true
	}
	return 0, 0, false
}

func nilCheckOwned(pdg *MiningGraph, op, nilLit, b int) bool {
	return ownedBy(pdg, op, b) && ownedBy(pdg, nilLit, op)
}

func isBareReturnOf(pdg *MiningGraph, r, x int) bool {
	prods := dataProducers(pdg, r)
	return pdg.Nodes[r].Kind == Return && len(prods) == 1 && prods[0] == x
}

// loneReturn: the single Return fed only by x on arm `arm` of b.
func loneReturn(pdg *MiningGraph, b, arm, x int) (int, bool) {
	targets := armTargets(armsOf(pdg, b), arm)
	if len(targets) != 1 {
		return 0, false
	}
	return targets[0], isBareReturnOf(pdg, targets[0], x)
}

func goPropagation(pdg *MiningGraph, b int) (goProp, bool) {
	op, neq, ok := nilCheckOp(pdg, b)
	if !ok {
		return goProp{}, false
	}
	x, nilLit, ok := nilCheckOperands(pdg, op)
	if !ok || !nilCheckOwned(pdg, op, nilLit, b) {
		return goProp{}, false
	}
	arm := 1
	if neq {
		arm = 0
	}
	ret, ok := loneReturn(pdg, b, arm, x)
	if !ok {
		return goProp{}, false
	}
	return goProp{branch: b, op: op, nilLit: nilLit, x: x, ret: ret}, true
}

// goPropagations finds residual Go error-propagate shapes the extractor did
// not fold into Try: `if <x> != nil { return <x> }` (nil on either side,
// non-ident conditions) and `if <x> == nil {..} else { return <x> }`.
// Only Branch nodes are examined, so existing Try nodes are never
// double-converted; propagate also runs before unifyCase relabels branches.
func goPropagations(pdg *MiningGraph) []goProp {
	var out []goProp
	for b := range pdg.Nodes {
		if pdg.Nodes[b].Kind != Branch {
			continue
		}
		if g, ok := goPropagation(pdg, b); ok {
			out = append(out, g)
		}
	}
	return out
}

func propagationDead(found [][3]int, goFound []goProp) map[int]struct{} {
	dead := map[int]struct{}{}
	for _, t := range found {
		dead[t[1]] = struct{}{}
		dead[t[2]] = struct{}{}
	}
	for _, g := range goFound {
		dead[g.op] = struct{}{}
		dead[g.nilLit] = struct{}{}
		dead[g.ret] = struct{}{}
	}
	return dead
}

func propagationTries(pdg *MiningGraph, found [][3]int, goFound []goProp) []PdgNode {
	isTry := map[int]bool{}
	for _, t := range found {
		isTry[t[0]] = true
	}
	for _, g := range goFound {
		isTry[g.branch] = true
	}
	nodes := make([]PdgNode, len(pdg.Nodes))
	for i := range pdg.Nodes {
		n := pdg.Nodes[i]
		if isTry[i] {
			n.Kind = Try
		}
		nodes[i] = n
	}
	return nodes
}

// goPropFeeds: the checked value feeds the new Try at 0, like the
// extractor's own Try encoding.
func goPropFeeds(goFound []goProp) []PdgEdge {
	feeds := make([]PdgEdge, 0, len(goFound))
	for _, g := range goFound {
		feeds = append(feeds, PdgEdge{From: g.x, To: g.branch, Kind: Data, ArgPos: 0})
	}
	return feeds
}

// propagateErrors: `match r { Ok(v) => v, Err(e) => return Err(e) }` becomes
// the Try node (`r?`), and so does a residual Go `if err != nil { return
// err }`. The graph shrinks by the ctor/comparison and the return.
func propagateErrors(pdg *MiningGraph) *MiningGraph {
	found, goFound := propagations(pdg), goPropagations(pdg)
	nodes := propagationTries(pdg, found, goFound)
	edges := dedupEdges(append(append([]PdgEdge{}, pdg.Edges...), goPropFeeds(goFound)...))
	return without(&MiningGraph{Nodes: nodes, Edges: edges}, propagationDead(found, goFound))
}

// ---- R5-R7: loops and counters -------------------------------------------------

var compares = []string{"<", "<=", "==", ">=", ">", "!="}

func isCompare(n *PdgNode) bool {
	return n.Kind == Op && slices.Contains(compares, opSuffix(n.Detail))
}

func isIntLit(n *PdgNode) bool {
	return isValueLeaf(n) && n.LitKind == "int"
}

type rangeLoop struct {
	iterate int
	call    int
	bounds  []int
}

func isRangeCall(pdg *MiningGraph, call int) bool {
	return strings.Contains(pdg.Nodes[call].CalleeID, "ops::range::")
}

func literalBound(pdg *MiningGraph, b, call int) bool {
	return isIntLit(&pdg.Nodes[b]) && ownedBy(pdg, b, call)
}

func literalBounds(pdg *MiningGraph, call int, bounds []int, iterate int) bool {
	if len(bounds) != 2 || !ownedBy(pdg, call, iterate) {
		return false
	}
	for _, b := range bounds {
		if !literalBound(pdg, b, call) {
			return false
		}
	}
	return true
}

// rangeCall recognizes an Iterate over a literal range call.
func rangeCall(pdg *MiningGraph, iterate int) (call int, bounds []int, ok bool) {
	prods := dataProducers(pdg, iterate)
	if len(prods) != 1 {
		return 0, nil, false
	}
	call = prods[0]
	if !isRangeCall(pdg, call) {
		return 0, nil, false
	}
	bounds = dataProducers(pdg, call)
	if !literalBounds(pdg, call, bounds, iterate) {
		return 0, nil, false
	}
	return call, bounds, true
}

func findRangeLoops(pdg *MiningGraph) []rangeLoop {
	var out []rangeLoop
	for i := range pdg.Nodes {
		if pdg.Nodes[i].Kind != Iterate {
			continue
		}
		if call, bounds, ok := rangeCall(pdg, i); ok {
			out = append(out, rangeLoop{i, call, bounds})
		}
	}
	return out
}

// asRangeLoop turns the Iterate into a Loop, keeping the bounds in detail.
func asRangeLoop(pdg *MiningGraph, n PdgNode, f rangeLoop) PdgNode {
	n.Kind = Loop
	parts := make([]string, len(f.bounds))
	for i, b := range f.bounds {
		parts[i] = pdg.Nodes[b].Detail
	}
	n.Detail = strings.Join(parts, "..")
	return n
}

// rangeLoops: `for i in 1..=3` — the iterate over a literal range is a
// Loop; the range call and its bounds go (the bounds stay in detail).
func rangeLoops(pdg *MiningGraph) *MiningGraph {
	founds := findRangeLoops(pdg)
	byIterate := make(map[int]rangeLoop, len(founds))
	dead := make(map[int]struct{}, 3*len(founds))
	for _, f := range founds {
		byIterate[f.iterate] = f
		dead[f.call] = struct{}{}
		for _, b := range f.bounds {
			dead[b] = struct{}{}
		}
	}
	nodes := make([]PdgNode, len(pdg.Nodes))
	for i := range pdg.Nodes {
		if f, ok := byIterate[i]; ok {
			nodes[i] = asRangeLoop(pdg, pdg.Nodes[i], f)
		} else {
			nodes[i] = pdg.Nodes[i]
		}
	}
	return without(&MiningGraph{Nodes: nodes, Edges: pdg.Edges}, dead)
}

// enclosingLoop: the nearest Loop above node in control dependence
// (a while guard sits between).
func enclosingLoop(pdg *MiningGraph, node int) (int, bool) {
	at := node
	for i := 0; i < 4; i++ {
		next, ok := ctrlParent(pdg, at)
		if !ok {
			return 0, false
		}
		at = next
		if pdg.Nodes[at].Kind == Loop {
			return at, true
		}
	}
	return 0, false
}

// operandAt: the data producer of `to` at argument position pos.
func operandAt(pdg *MiningGraph, to, pos int) (int, bool) {
	for _, e := range pdg.Edges {
		if e.To == to && e.Kind == Data && e.ArgPos == pos {
			return e.From, true
		}
	}
	return 0, false
}

// addAssignOperands: Rust's `n += 1` step — amount at 0, init at 1, both
// int literals. Returns (init, amount).
func addAssignOperands(pdg *MiningGraph, step int) (init, amount int, ok bool) {
	amount, ok = operandAt(pdg, step, 0)
	if !ok || !isIntLit(&pdg.Nodes[amount]) {
		return 0, 0, false
	}
	init, ok = operandAt(pdg, step, 1)
	if !ok || !isIntLit(&pdg.Nodes[init]) {
		return 0, 0, false
	}
	return init, amount, true
}

// incOperands: Go's `i++` step — a single int-literal input at 0, the
// amount implicitly 1. Returns (init, -1): no amount node exists.
func incOperands(pdg *MiningGraph, step int) (init, amount int, ok bool) {
	init, ok = operandAt(pdg, step, 0)
	if !ok || !isIntLit(&pdg.Nodes[init]) {
		return 0, 0, false
	}
	if len(dataProducers(pdg, step)) != 1 {
		return 0, 0, false
	}
	return init, -1, true
}

// stepOperands returns (init, amount) of a counter step, or ok=false.
// Go adaptation: besides Rust's `+=` / `+==`, Go's `++` / `--` steps count.
func stepOperands(pdg *MiningGraph, step int) (init, amount int, ok bool) {
	if pdg.Nodes[step].Kind != Op {
		return 0, 0, false
	}
	switch opSuffix(pdg.Nodes[step].Detail) {
	case "+=", "+==":
		return addAssignOperands(pdg, step)
	case "++", "--":
		return incOperands(pdg, step)
	}
	return 0, 0, false
}

func stepShapeOk(pdg *MiningGraph, step, amount int) bool {
	return (amount < 0 || ownedBy(pdg, amount, step)) &&
		len(dataConsumers(pdg, step)) == 0
}

// manualCounter: a manual counter stepped in a loop — (init literal, step
// op, step literal or -1, loop) for `let mut n = 0; loop { n += 1; .. }`.
func manualCounter(pdg *MiningGraph, step int) (init, amount, looped int, ok bool) {
	init, amount, ok = stepOperands(pdg, step)
	if !ok || !stepShapeOk(pdg, step, amount) {
		return 0, 0, 0, false
	}
	looped, ok = enclosingLoop(pdg, step)
	if !ok {
		return 0, 0, 0, false
	}
	return init, amount, looped, true
}

type counterFound struct {
	step   int
	init   int
	amount int // -1 for ++/-- steps, which have no amount node
	looped int
}

func counterMaps(founds []counterFound) (map[int]int, map[int]bool) {
	loopOf := make(map[int]int, len(founds))
	steps := make(map[int]bool, len(founds))
	for _, f := range founds {
		loopOf[f.init] = f.looped
		steps[f.step] = true
	}
	return loopOf, steps
}

func isCounterRead(e PdgEdge, loopOf map[int]int, steps map[int]bool) (int, bool) {
	looped, ok := loopOf[e.From]
	if !ok || e.Kind != Data || steps[e.To] {
		return 0, false
	}
	return looped, true
}

// counterReads: whatever read the counter now reads the loop node.
func counterReads(pdg *MiningGraph, founds []counterFound) []PdgEdge {
	loopOf, steps := counterMaps(founds)
	var reads []PdgEdge
	for _, e := range pdg.Edges {
		looped, ok := isCounterRead(e, loopOf, steps)
		if !ok {
			continue
		}
		e.From = looped
		reads = append(reads, e)
	}
	return reads
}

func counterDead(founds []counterFound) map[int]struct{} {
	dead := map[int]struct{}{}
	for _, f := range founds {
		dead[f.step] = struct{}{}
		dead[f.init] = struct{}{}
		if f.amount >= 0 {
			dead[f.amount] = struct{}{}
		}
	}
	return dead
}

// manualCounters: the loop node stands for the counter; the initialisation
// and the step disappear.
func manualCounters(pdg *MiningGraph) *MiningGraph {
	var founds []counterFound
	for step := range pdg.Nodes {
		if init, amount, looped, ok := manualCounter(pdg, step); ok {
			founds = append(founds, counterFound{step, init, amount, looped})
		}
	}
	edges := dedupEdges(append(append([]PdgEdge{}, pdg.Edges...), counterReads(pdg, founds)...))
	return without(&MiningGraph{Nodes: pdg.Nodes, Edges: edges}, counterDead(founds))
}

type guardFound struct {
	guard int
	owner int
	op    int
	bound int
}

func loopGuardOwner(pdg *MiningGraph, g int) (int, bool) {
	owner, ok := ctrlParent(pdg, g)
	if !ok || pdg.Nodes[owner].Kind != Loop {
		return 0, false
	}
	return owner, true
}

// guardCond: the single comparison feeding guard g.
func guardCond(pdg *MiningGraph, g int) (int, bool) {
	cond := dataProducers(pdg, g)
	if len(cond) != 1 || !isCompare(&pdg.Nodes[cond[0]]) {
		return 0, false
	}
	return cond[0], true
}

func intLitInput(pdg *MiningGraph, inputs []int) (int, bool) {
	for _, n := range inputs {
		if isIntLit(&pdg.Nodes[n]) {
			return n, true
		}
	}
	return 0, false
}

func allArmZero(pdg *MiningGraph, g int) bool {
	for _, e := range armsOf(pdg, g) {
		if e.ArgPos != 0 {
			return false
		}
	}
	return true
}

func guardBoundWiring(pdg *MiningGraph, op, bound, g int) bool {
	return ownedBy(pdg, bound, op) && ownedBy(pdg, op, g) && allArmZero(pdg, g)
}

// guardBound: the literal bound compared against the counter loop owner.
func guardBound(pdg *MiningGraph, op, owner, g int) (int, bool) {
	inputs := dataProducers(pdg, op)
	if len(inputs) != 2 {
		return 0, false
	}
	bound, ok := intLitInput(pdg, inputs)
	if !ok || !slices.Contains(inputs, owner) {
		return 0, false
	}
	if !guardBoundWiring(pdg, op, bound, g) {
		return 0, false
	}
	return bound, true
}

// loopGuard: `while n < 3 { body }` where n is a counter loop — the guard is
// the loop bound, so the body hangs directly under the loop.
func loopGuard(pdg *MiningGraph, g int) (owner, op, bound int, ok bool) {
	k := pdg.Nodes[g].Kind
	if k != Case && k != Branch {
		return 0, 0, 0, false
	}
	owner, ok = loopGuardOwner(pdg, g)
	if !ok {
		return 0, 0, 0, false
	}
	op, ok = guardCond(pdg, g)
	if !ok {
		return 0, 0, 0, false
	}
	bound, ok = guardBound(pdg, op, owner, g)
	if !ok {
		return 0, 0, 0, false
	}
	return owner, op, bound, true
}

func findGuards(pdg *MiningGraph) []guardFound {
	var out []guardFound
	for g := range pdg.Nodes {
		if owner, op, bound, ok := loopGuard(pdg, g); ok {
			out = append(out, guardFound{g, owner, op, bound})
		}
	}
	return out
}

func guardBodyEdge(e PdgEdge, ownerOf map[int]int) (PdgEdge, bool) {
	owner, ok := ownerOf[e.From]
	if !ok || e.Kind != Ctrl {
		return e, false
	}
	e.From = owner
	return e, true
}

func guardBodies(pdg *MiningGraph, founds []guardFound) []PdgEdge {
	ownerOf := make(map[int]int, len(founds))
	for _, f := range founds {
		ownerOf[f.guard] = f.owner
	}
	var body []PdgEdge
	for _, e := range pdg.Edges {
		if ne, ok := guardBodyEdge(e, ownerOf); ok {
			body = append(body, ne)
		}
	}
	return body
}

func guardDead(founds []guardFound) map[int]struct{} {
	dead := make(map[int]struct{}, 3*len(founds))
	for _, f := range founds {
		dead[f.guard] = struct{}{}
		dead[f.op] = struct{}{}
		dead[f.bound] = struct{}{}
	}
	return dead
}

func loopGuards(pdg *MiningGraph) *MiningGraph {
	founds := findGuards(pdg)
	body := guardBodies(pdg, founds)
	edges := dedupEdges(append(append([]PdgEdge{}, pdg.Edges...), body...))
	return without(&MiningGraph{Nodes: pdg.Nodes, Edges: edges}, guardDead(founds))
}

func counterLoops(pdg *MiningGraph) *MiningGraph {
	return loopGuards(manualCounters(rangeLoops(pdg)))
}

// isCountedCompare: a comparison of a counter loop node against an int
// literal, whatever the operator.
func isCountedCompare(pdg *MiningGraph, n int) bool {
	inputs := dataProducers(pdg, n)
	if !isCompare(&pdg.Nodes[n]) || len(inputs) != 2 {
		return false
	}
	return hasLoopInput(pdg, inputs) && hasIntLitInput(pdg, inputs)
}

func hasLoopInput(pdg *MiningGraph, inputs []int) bool {
	for _, p := range inputs {
		if pdg.Nodes[p].Kind == Loop {
			return true
		}
	}
	return false
}

func hasIntLitInput(pdg *MiningGraph, inputs []int) bool {
	_, ok := intLitInput(pdg, inputs)
	return ok
}

// compareClass: `i < 3` and `i == 3` on a counter compare it with a literal
// bound: one operator class. Only when the other operand is a literal
// (otherwise the operators mean different things). The original operator
// survives after the colon as the hole value; Go details already carry the
// "cmp:" class, so this is idempotent on extractor output.
func compareClass(pdg *MiningGraph) *MiningGraph {
	nodes := make([]PdgNode, len(pdg.Nodes))
	for i := range pdg.Nodes {
		n := pdg.Nodes[i]
		if isCountedCompare(pdg, i) {
			n.Detail = "cmp:" + opSuffix(n.Detail)
		}
		nodes[i] = n
	}
	return &MiningGraph{Nodes: nodes, Edges: pdg.Edges}
}
