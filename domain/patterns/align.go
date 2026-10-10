package patterns

import (
	"sort"
	"strconv"
	"strings"
)

// Graph alignment: which node of one PDG corresponds to which node of
// another, and where the matched nodes differ (the holes). Ported from
// rstyle's align.rs.
//
// Holes are the substitution table of the anti-unification:
// `T0 = Dog | Cat`, `M0 = bark | meow`.

// HoleKind is the kind of a hole: what differs at the aligned sites.
type HoleKind string

const (
	HoleType    HoleKind = "type"
	HoleMethod  HoleKind = "method"
	HoleFreeFn  HoleKind = "free_fn"
	HoleLiteral HoleKind = "literal"
	HoleField   HoleKind = "field"
	HoleOp      HoleKind = "op"
	HoleEffect  HoleKind = "effect"
)

// Prefix is the hole variable prefix of the kind: T, M, F, L, D, O, E.
func (k HoleKind) Prefix() byte {
	if p, ok := holeKindPrefixes[k]; ok {
		return p
	}
	return 'O'
}

var holeKindPrefixes = map[HoleKind]byte{
	HoleType:    'T',
	HoleMethod:  'M',
	HoleFreeFn:  'F',
	HoleLiteral: 'L',
	HoleField:   'D',
	HoleEffect:  'E',
}

// HoleVar is the variable name of the index-th hole of a kind: T0, M1, ...
func HoleVar(kind HoleKind, index int) string {
	return string([]byte{kind.Prefix()}) + strconv.Itoa(index)
}

// holeRank reproduces Rust's HoleKind declaration order for sorting holes.
func holeRank(k HoleKind) int {
	switch k {
	case HoleType:
		return 0
	case HoleMethod:
		return 1
	case HoleFreeFn:
		return 2
	case HoleLiteral:
		return 3
	case HoleField:
		return 4
	default:
		return 5
	}
}

// Hole is one substitution a -> b. Every site where the same pair occurs
// shares the variable.
type Hole struct {
	Kind HoleKind
	Var  string
	A    string
	B    string
	// Sites are aligned node pairs (node in a, node in b) where the
	// substitution applies.
	Sites [][2]int
}

// Alignment is the result of aligning two PDGs.
type Alignment struct {
	Pairs [][2]int
	Holes []Hole
	// CoverageMilli is aligned pairs over the larger node count, in
	// thousandths.
	CoverageMilli uint32
}

// ctx bundles everything alignment reads: the two PDGs and their WL
// refinements, the cross-graph colors, and the deterministic DFS ordinals.
// It ports Rust's align::Ctx.
type ctx struct {
	pa, pb *Pdg
	wa, wb *Wl
	ca, cb []uint64
	oa, ob []int
}

// newCtx ports Ctx::new: the cross colors are the deepest refinement level
// both graphs support.
func newCtx(pa *Pdg, wa *Wl, pb *Pdg, wb *Wl) *ctx {
	h := levels(wa, wb)
	return &ctx{
		pa: pa, pb: pb,
		wa: wa, wb: wb,
		ca: wa.rounds[h-1],
		cb: wb.rounds[h-1],
		oa: ordinals(wa, wa.rounds[h-1]),
		ob: ordinals(wb, wb.rounds[h-1]),
	}
}

// levelsUsed is the number of WL levels both graphs support (wl::levels).
func (c *ctx) levelsUsed() int { return levels(c.wa, c.wb) }

// atLevel compares the same graphs by the colors of WL refinement level
// (0 = raw labels).
func (c *ctx) atLevel(level int) *ctx {
	return &ctx{
		pa: c.pa, pb: c.pb,
		wa: c.wa, wb: c.wb,
		ca: c.wa.rounds[level],
		cb: c.wb.rounds[level],
		oa: c.oa, ob: c.ob,
	}
}

// childrenOf returns the out-neighbours of v ordered for DFS: ascending by
// (edge tag, color, index), reversed so the stack pops the smallest first.
func childrenOf(w *Wl, colors []uint64, v int) []int {
	kids := append([]Nb(nil), w.graph.out[v]...)
	sort.Slice(kids, func(i, j int) bool {
		if kids[i].tag != kids[j].tag {
			return kids[i].tag < kids[j].tag
		}
		ci, cj := colors[kids[i].node], colors[kids[j].node]
		if ci != cj {
			return ci < cj
		}
		return kids[i].node < kids[j].node
	})
	out := make([]int, len(kids))
	for i, nb := range kids {
		out[len(kids)-1-i] = int(nb.node)
	}
	return out
}

// rootsByColor lists the nodes with no incoming edges, sorted by (color,
// index).
func rootsByColor(w *Wl, colors []uint64) []int {
	var roots []int
	for v := range colors {
		if len(w.graph.inc[v]) == 0 {
			roots = append(roots, v)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if colors[roots[i]] != colors[roots[j]] {
			return colors[roots[i]] < colors[roots[j]]
		}
		return roots[i] < roots[j]
	})
	return roots
}

// dfs carries the state of one deterministic depth-first traversal,
// mirroring Rust's Dfs.
type dfs struct {
	w      *Wl
	colors []uint64
	seen   []bool
	order  []int
}

func (d *dfs) from(start int) {
	stack := []int{start}
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if d.seen[v] {
			continue
		}
		d.seen[v] = true
		d.order = append(d.order, v)
		stack = append(stack, childrenOf(d.w, d.colors, v)...)
	}
}

// ordinals is the deterministic DFS ordinal of every node: roots by (color,
// index), then anything left (cycles) by index. Symmetric graphs still get
// one fixed order.
func ordinals(w *Wl, colors []uint64) []int {
	n := len(colors)
	d := &dfs{w: w, colors: colors, seen: make([]bool, n)}
	for _, r := range rootsByColor(w, colors) {
		d.from(r)
	}
	for v := 0; v < n; v++ {
		d.from(v)
	}
	ordinal := make([]int, n)
	for i, v := range d.order {
		ordinal[v] = i
	}
	return ordinal
}

// matching is a partial bijection between the node indices of the two
// graphs: -1 means unmatched. It ports Rust's align::Matching.
type matching struct {
	fwd  []int
	back []int
}

func newMatching(na, nb int) *matching {
	m := &matching{fwd: make([]int, na), back: make([]int, nb)}
	for i := range m.fwd {
		m.fwd[i] = -1
	}
	for i := range m.back {
		m.back[i] = -1
	}
	return m
}

func (m *matching) link(a, b int) {
	m.fwd[a] = b
	m.back[b] = a
}

func (m *matching) unlink(a int) {
	if b := m.fwd[a]; b != -1 {
		m.fwd[a] = -1
		m.back[b] = -1
	}
}

// pairs lists the linked pairs in a-index order.
func (m *matching) pairs() [][2]int {
	var out [][2]int
	for a, b := range m.fwd {
		if b != -1 {
			out = append(out, [2]int{a, b})
		}
	}
	return out
}

func hasEdge(list []Nb, node int, tag uint64) bool {
	for _, nb := range list {
		if nb.node == uint64(node) && nb.tag == tag {
			return true
		}
	}
	return false
}

// evidence counts neighbours of a that are matched: those the counterpart b
// mirrors (agree) and those it does not (conflict).
func evidence(c *ctx, m *matching, a, b int) (agree, conflict int) {
	count := func(mine, theirs []Nb) {
		for _, nb := range mine {
			image := m.fwd[nb.node]
			if image == -1 {
				continue
			}
			if hasEdge(theirs, image, nb.tag) {
				agree++
			} else {
				conflict++
			}
		}
	}
	count(c.wa.graph.inc[a], c.wb.graph.inc[b])
	count(c.wa.graph.out[a], c.wb.graph.out[b])
	return agree, conflict
}

// reverseConflicts counts edges of b into matched nodes that a does not
// mirror.
func reverseConflicts(c *ctx, m *matching, a, b int) int {
	missing := 0
	count := func(theirs, mine []Nb) {
		for _, nb := range theirs {
			preimage := m.back[nb.node]
			if preimage == -1 {
				continue
			}
			if !hasEdge(mine, preimage, nb.tag) {
				missing++
			}
		}
	}
	count(c.wb.graph.inc[b], c.wa.graph.inc[a])
	count(c.wb.graph.out[b], c.wa.graph.out[a])
	return missing
}

// violations is what prune minimizes: conflicts plus reverse conflicts.
func violations(c *ctx, m *matching, a, b int) int {
	_, conflict := evidence(c, m, a, b)
	return conflict + reverseConflicts(c, m, a, b)
}

// classes groups node indices by color, each class in ordinal order. Rust
// uses a BTreeMap; callers sort the keys where iteration order matters.
func classes(colors []uint64, ordinal []int) map[uint64][]int {
	byColor := map[uint64][]int{}
	for v, color := range colors {
		byColor[color] = append(byColor[color], v)
	}
	for _, nodes := range byColor {
		sort.Slice(nodes, func(i, j int) bool { return ordinal[nodes[i]] < ordinal[nodes[j]] })
	}
	return byColor
}

// orderByOrdinal lists 0..n-1 in ordinal order.
func orderByOrdinal(n int, ordinal []int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return ordinal[order[i]] < ordinal[order[j]] })
	return order
}

// uniquePairs are the pairs unique in their color class, colors ascending.
func uniquePairs(c *ctx) [][2]int {
	ka, kb := classes(c.ca, c.oa), classes(c.cb, c.ob)
	keys := make([]uint64, 0, len(ka))
	for k := range ka {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var out [][2]int
	for _, k := range keys {
		na, nb := ka[k], kb[k]
		if len(na) == 1 && len(nb) == 1 {
			out = append(out, [2]int{na[0], nb[0]})
		}
	}
	return out
}

// seeded links the unique pairs that are still unmatched.
func seeded(c *ctx, m *matching) *matching {
	for _, p := range uniquePairs(c) {
		if m.fwd[p[0]] == -1 && m.back[p[1]] == -1 {
			m.link(p[0], p[1])
		}
	}
	return m
}

// partnerScore rates b as a partner for a: the mirrored-neighbour count, or
// -1 when b is taken or has any conflict.
func partnerScore(c *ctx, m *matching, a, b int) int {
	if m.back[b] != -1 {
		return -1
	}
	agree, conflict := evidence(c, m, a, b)
	if conflict != 0 {
		return -1
	}
	return agree
}

// betterPartner reports whether candidate b (score agree) beats the current
// best: most mirrored neighbours wins, ties go to the lower ordinal.
func betterPartner(c *ctx, agree, bestAgree, b, best int) bool {
	if agree < 0 {
		return false
	}
	return agree > bestAgree || (agree == bestAgree && c.ob[b] < c.ob[best])
}

// bestPartner is the best unmatched same-color partner of a.
func bestPartner(c *ctx, m *matching, a int, kb map[uint64][]int) (int, int, bool) {
	best, bestAgree := -1, -1
	for _, b := range kb[c.ca[a]] {
		if agree := partnerScore(c, m, a, b); betterPartner(c, agree, bestAgree, b, best) {
			best, bestAgree = b, agree
		}
	}
	return bestAgree, best, best != -1
}

// pass matches unmatched nodes in ordinal order. With needEvidence, a node
// is only matched when a neighbour already vouches for the partner.
func pass(c *ctx, m *matching, needEvidence bool) (*matching, bool) {
	kb := classes(c.cb, c.ob)
	moved := false
	for _, a := range orderByOrdinal(len(c.ca), c.oa) {
		if m.fwd[a] != -1 {
			continue
		}
		agree, b, ok := bestPartner(c, m, a, kb)
		if ok && (agree > 0 || !needEvidence) {
			m.link(a, b)
			moved = true
		}
	}
	return m, moved
}

// settle repeats evidence-backed passes to a fixpoint; greedy then matches
// what is left by ordinal.
func settle(c *ctx, m *matching, greedy bool) *matching {
	for {
		next, moved := pass(c, m, true)
		m = next
		if !moved {
			if greedy {
				m, _ = pass(c, m, false)
			}
			return m
		}
	}
}

// holey reports whether unmatched nodes of the kind pair up in complete:
// leaf-like nodes whose label differs but whose matched neighbours agree.
func holey(kind NodeKind) bool {
	return kind == Call || kind == Lit || kind == Field
}

// completeFits checks the cheap preconditions for a completion partner.
func completeFits(c *ctx, m *matching, a, b int) bool {
	return m.back[b] == -1 && c.pb.Nodes[b].Kind == c.pa.Nodes[a].Kind
}

// completeScore rates b as a completion partner for a: the mirrored
// neighbour count, or -1 on any rejection.
func completeScore(c *ctx, m *matching, a, b int, strict bool) int {
	if !completeFits(c, m, a, b) {
		return -1
	}
	agree, conflict := evidence(c, m, a, b)
	if agree == 0 || conflict != 0 {
		return -1
	}
	if strict && reverseConflicts(c, m, a, b) != 0 {
		return -1
	}
	return agree
}

// completePartner picks the best completion partner for a, or -1.
func completePartner(c *ctx, m *matching, a int, strict bool) int {
	best, bestAgree := -1, -1
	for b := range c.cb {
		if agree := completeScore(c, m, a, b, strict); betterPartner(c, agree, bestAgree, b, best) {
			best, bestAgree = b, agree
		}
	}
	return best
}

// complete pairs unmatched same-kind leaf-like nodes whose label differs but
// whose matched neighbours all agree.
func complete(c *ctx, m *matching, strict bool) *matching {
	for _, a := range orderByOrdinal(len(c.ca), c.oa) {
		if m.fwd[a] != -1 || !holey(c.pa.Nodes[a].Kind) {
			continue
		}
		if b := completePartner(c, m, a, strict); b != -1 {
			m.link(a, b)
		}
	}
	return m
}

// worstViolation returns the a-index of the pair with the most violations
// (ties go to the larger index), or -1 when the matching is edge-consistent.
func worstViolation(c *ctx, m *matching) int {
	worst, worstV := -1, 0
	for _, p := range m.pairs() {
		v := violations(c, m, p[0], p[1])
		if v == 0 || v < worstV {
			continue
		}
		if v > worstV || p[0] > worst {
			worst, worstV = p[0], v
		}
	}
	return worst
}

// prune drops pairs whose edges are not mirrored, worst first, until the
// matching is edge-consistent.
func prune(c *ctx, m *matching) *matching {
	for {
		worst := worstViolation(c, m)
		if worst == -1 {
			return m
		}
		m.unlink(worst)
	}
}

// holeDiff is one detected difference between two aligned nodes.
type holeDiff struct {
	kind HoleKind
	a, b string
}

// hasReceiver reports whether a Go callee id names a method. FuncID renders
// methods as "pkgpath.Type.Method" and free functions as "pkgpath.Func", so
// the segment after the last "/" carries two dots for a method and one for a
// function. (A package whose final path segment itself contains a dot, e.g.
// gopkg.in/yaml.v2, defeats this; the extractor does not record receivers,
// so this heuristic is the best available.)
func hasReceiver(calleeID string) bool {
	seg := calleeID
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	return strings.Count(seg, ".") >= 2
}

// calleeKind ports the Method-vs-FreeFn rule: a Call whose callee (or its
// counterpart's) has a receiver is a Method hole, otherwise FreeFn (which
// also covers fn items passed as values on Lit nodes).
func calleeKind(a *PdgNode, x, y string) HoleKind {
	if a.Kind == Call && (hasReceiver(x) || hasReceiver(y)) {
		return HoleMethod
	}
	return HoleFreeFn
}

// calleeDiff ports Rust's callee_diff: a differing callee id between two
// aligned nodes.
func calleeDiff(a, b *PdgNode) (holeDiff, bool) {
	x, y := a.CalleeID, b.CalleeID
	if x == "" || y == "" || x == y {
		return holeDiff{}, false
	}
	return holeDiff{calleeKind(a, x, y), x, y}, true
}

// diffOf builds a hole when x and y differ.
func diffOf(kind HoleKind, x, y string) (holeDiff, bool) {
	if x == y {
		return holeDiff{}, false
	}
	return holeDiff{kind, x, y}, true
}

// opDiff ports the cmp: arm of text_diff: only canonical comparisons
// (detail "cmp:<op>") on both sides hole the operator itself.
func opDiff(x, y string) (holeDiff, bool) {
	if !strings.HasPrefix(x, "cmp:") || !strings.HasPrefix(y, "cmp:") || x == y {
		return holeDiff{}, false
	}
	return holeDiff{HoleOp, x[4:], y[4:]}, true
}

// textDiff ports Rust's text_diff: differing literal text, field names, and
// canonical comparison operators.
func textDiff(a, b *PdgNode) (holeDiff, bool) {
	switch a.Kind {
	case Lit:
		if a.CalleeID != "" {
			break
		}
		return diffOf(HoleLiteral, a.Detail, b.Detail)
	case Field:
		return diffOf(HoleField, a.Detail, b.Detail)
	case Op:
		return opDiff(a.Detail, b.Detail)
	}
	return holeDiff{}, false
}

// typeDiffs ports Rust's type_diffs/adt_diffs. Rust walks the raw type
// shapes and reports differing ADT ids at parallel positions; the Go schema
// keeps only the flat TyClass label, so the walk collapses to one
// inequality. Consequence: same-ADT/differing-arg diffs (Vec<Dog> vs
// Vec<Cat>) are invisible — the extractor erases args by design — while any
// other class difference, primitives included, becomes one Type hole.
func typeDiffs(a, b *PdgNode) []holeDiff {
	if a.TyClass == "" || b.TyClass == "" || a.TyClass == b.TyClass {
		return nil
	}
	return []holeDiff{{HoleType, a.TyClass, b.TyClass}}
}

// effectDiff reports when two aligned Call nodes have different side-effect
// profiles. Effect differences are holes: the abstraction must account for
// what each side does, not just what it calls.
func effectDiff(a, b *PdgNode) (holeDiff, bool) {
	if a.Kind != Call || b.Kind != Call {
		return holeDiff{}, false
	}
	if a.Effects == b.Effects {
		return holeDiff{}, false
	}
	return holeDiff{HoleEffect, effectName(a.Effects), effectName(b.Effects)}, true
}

// effectName renders effect bitflags for hole display.
func effectName(e uint16) string {
	if e == EffectNone {
		return "pure"
	}
	var parts []string
	for _, ef := range effectFlagNames {
		if e&ef.bit != 0 {
			parts = append(parts, ef.name)
		}
	}
	return strings.Join(parts, "|")
}

type effectFlagName struct {
	bit  uint16
	name string
}

var effectFlagNames = []effectFlagName{
	{EffectMutates, "mutates"},
	{EffectIO, "io"},
	{EffectNetwork, "network"},
	{EffectGlobal, "global"},
	{EffectUnsafe, "unsafe"},
	{EffectTime, "time"},
	{EffectRandom, "random"},
	{EffectPanic, "panic"},
	{EffectUnknown, "unknown"},
}

// nodeDiffs collects every hole between two aligned nodes.
func nodeDiffs(a, b *PdgNode) []holeDiff {
	var out []holeDiff
	if d, ok := calleeDiff(a, b); ok {
		out = append(out, d)
	}
	if d, ok := textDiff(a, b); ok {
		out = append(out, d)
	}
	if d, ok := effectDiff(a, b); ok {
		out = append(out, d)
	}
	return append(out, typeDiffs(a, b)...)
}

// holeKey identifies one substitution: equal triples share a variable.
type holeKey struct {
	kind HoleKind
	a, b string
}

// holeTable groups the detected diffs by substitution, sites in pair order.
func holeTable(c *ctx, pairs [][2]int) map[holeKey][][2]int {
	table := map[holeKey][][2]int{}
	for _, p := range pairs {
		for _, d := range nodeDiffs(&c.pa.Nodes[p[0]], &c.pb.Nodes[p[1]]) {
			key := holeKey{d.kind, d.a, d.b}
			table[key] = append(table[key], p)
		}
	}
	return table
}

// holesOf builds the substitution table: holes ordered by first site, then
// kind; each kind numbered from zero.
func holesOf(c *ctx, pairs [][2]int) []Hole {
	type keyed struct {
		key   holeKey
		sites [][2]int
	}
	var entries []keyed
	for key, sites := range holeTable(c, pairs) {
		entries = append(entries, keyed{key, sites})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].sites[0][0] != entries[j].sites[0][0] {
			return entries[i].sites[0][0] < entries[j].sites[0][0]
		}
		return holeRank(entries[i].key.kind) < holeRank(entries[j].key.kind)
	})
	counts := map[HoleKind]int{}
	var holes []Hole
	for _, e := range entries {
		counts[e.key.kind]++
		holes = append(holes, Hole{
			Kind:  e.key.kind,
			Var:   HoleVar(e.key.kind, counts[e.key.kind]-1),
			A:     e.key.a,
			B:     e.key.b,
			Sites: e.sites,
		})
	}
	return holes
}

// Stage is the matching after one pipeline step, for the walkthrough.
type Stage struct {
	Name  string
	Pairs [][2]int
}

// Variant selects an alignment pipeline; Baseline is Align.
type Variant int

const (
	Baseline Variant = iota
	// LabelsOnly skips the refined-color stages; starts from raw labels.
	LabelsOnly
	// NoFinalPrune omits the prune after complete.
	NoFinalPrune
	// CompleteChecked makes complete reject pairs with reverse conflicts.
	CompleteChecked
	// CoarseToFine seeds and settles at every WL level from deepest to raw
	// labels, pruning after each.
	CoarseToFine
	// Combined is CoarseToFine plus CompleteChecked.
	Combined
)

// step is one named pipeline stage.
type step struct {
	name string
	run  func(*matching) *matching
}

func grow(c *ctx, name string, greedy bool) step {
	return step{name, func(m *matching) *matching {
		return settle(c, seeded(c, m), greedy)
	}}
}

func pruneStep(c *ctx, name string) step {
	return step{name, func(m *matching) *matching { return prune(c, m) }}
}

var levelNames = [4]string{"level0", "level1", "level2", "level3"}

// pairingSteps are the refined-color stages, then raw labels (the pipeline
// before complete).
func pairingSteps(c *ctx, levelCtxs []*ctx, v Variant) []step {
	switch v {
	case LabelsOnly:
		return []step{grow(levelCtxs[0], "labels", false), pruneStep(c, "prune_labels")}
	case CoarseToFine, Combined:
		var out []step
		for k := len(levelCtxs) - 1; k >= 0; k-- {
			deepest := k == len(levelCtxs)-1
			out = append(out, grow(levelCtxs[k], levelNames[min(k, 3)], deepest), pruneStep(c, "prune"))
		}
		return out
	default:
		return []step{
			{"seed", func(m *matching) *matching { return seeded(c, m) }},
			{"settle", func(m *matching) *matching { return settle(c, m, true) }},
			pruneStep(c, "prune"),
			grow(levelCtxs[0], "labels", false),
			pruneStep(c, "prune_labels"),
		}
	}
}

// steps is the full pipeline: pairing stages, complete, and the final prune.
func steps(c *ctx, levelCtxs []*ctx, v Variant) []step {
	strict := v == CompleteChecked || v == Combined
	out := pairingSteps(c, levelCtxs, v)
	out = append(out, step{"complete", func(m *matching) *matching {
		return complete(c, m, strict)
	}})
	if v != NoFinalPrune {
		out = append(out, pruneStep(c, "final"))
	}
	return out
}

// alignVariant runs the pipeline and scores the final matching.
func alignVariant(c *ctx, v Variant) (Alignment, []Stage) {
	depth := len(c.wa.rounds)
	if l := c.levelsUsed(); l < depth {
		depth = l
	}
	if depth < 1 {
		depth = 1
	}
	levelCtxs := make([]*ctx, depth)
	for k := range levelCtxs {
		levelCtxs[k] = c.atLevel(k)
	}
	m := newMatching(len(c.ca), len(c.cb))
	var stages []Stage
	for _, s := range steps(c, levelCtxs, v) {
		m = s.run(m)
		stages = append(stages, Stage{Name: s.name, Pairs: m.pairs()})
	}
	pairs := stages[len(stages)-1].Pairs
	larger := max(len(c.ca), len(c.cb), 1)
	return Alignment{
		Pairs:         pairs,
		Holes:         holesOf(c, pairs),
		CoverageMilli: uint32(len(pairs) * 1000 / larger),
	}, stages
}

// Align aligns two PDGs: the default (Baseline) pipeline variant.
func Align(pa *Pdg, wa *Wl, pb *Pdg, wb *Wl) Alignment {
	al, _ := AlignStaged(pa, wa, pb, wb)
	return al
}

// AlignStaged is Align, also reporting the matching after every step.
func AlignStaged(pa *Pdg, wa *Wl, pb *Pdg, wb *Wl) (Alignment, []Stage) {
	return alignVariant(newCtx(pa, wa, pb, wb), Baseline)
}

// AlignVariant runs one pipeline variant, for the ablation.
func AlignVariant(pa *Pdg, wa *Wl, pb *Pdg, wb *Wl, v Variant) (Alignment, []Stage) {
	return alignVariant(newCtx(pa, wa, pb, wb), v)
}
