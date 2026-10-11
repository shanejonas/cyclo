package patterns

import (
	"slices"
	"sort"
	"testing"
)

// Hand-built PDG fixtures, mirroring rstyle's pdgtest helpers.

func pn(kind NodeKind) PdgNode { return PdgNode{Kind: kind, Line: 1} }

func pOp(detail string) PdgNode {
	n := pn(Op)
	n.Detail = detail
	return n
}

func pLit(kind, text string) PdgNode {
	n := pn(Lit)
	n.LitKind = kind
	n.Detail = text
	return n
}

func pCall(id string) PdgNode {
	n := pn(Call)
	n.CalleeID = id
	return n
}

func pData(from, to, argPos int) PdgEdge {
	return PdgEdge{From: from, To: to, Kind: Data, ArgPos: argPos}
}

func pCtrl(from, to, argPos int) PdgEdge {
	return PdgEdge{From: from, To: to, Kind: Ctrl, ArgPos: argPos}
}

func pMacro(detail string) PdgNode {
	// The Go schema has no Macro kind; the detail stands in for macro_name.
	return PdgNode{Kind: macroKind, Detail: detail, Line: 1}
}

func pKinds(pdg *MiningGraph) []NodeKind {
	kinds := make([]NodeKind, len(pdg.Nodes))
	for i, n := range pdg.Nodes {
		kinds[i] = n.Kind
	}
	return kinds
}

func sortedEdges(pdg *MiningGraph) []PdgEdge {
	edges := append([]PdgEdge{}, pdg.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ArgPos < b.ArgPos
	})
	return edges
}

func sameGraph(a, b *MiningGraph) bool {
	if len(a.Nodes) != len(b.Nodes) {
		return false
	}
	for i := range a.Nodes {
		if a.Nodes[i] != b.Nodes[i] {
			return false
		}
	}
	return slices.Equal(sortedEdges(a), sortedEdges(b))
}

func mustParse(t *testing.T, text string) Rules {
	t.Helper()
	r, err := ParseRules(text)
	if err != nil {
		t.Fatalf("ParseRules(%q): %v", text, err)
	}
	return r
}

// `return Err(Bad("x"))`
func nestedConstant() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{pLit("str", `"x"`), pOp("ctor"), pOp("ctor"), pn(Return)},
		Edges: []PdgEdge{pData(0, 1, 0), pData(1, 2, 0), pData(2, 3, 0)},
	}
}

// `if !a.ok() && !b.ok() {..}`
func negatedPair() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pCall("k::A::ok"), pCall("k::B::ok"),
			pOp("!"), pOp("!"), pOp("&&"), pn(Branch),
		},
		Edges: []PdgEdge{
			pData(0, 2, 0), pData(1, 3, 0), pData(2, 4, 0),
			pData(3, 4, 1), pData(4, 5, 0),
		},
	}
}

// `for x in xs { out.push(x.method()) }` over a collection: no rule fires.
func emitLoop() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Param), pn(Param), pn(Iterate),
			pCall("alloc::vec::Vec::push"), pCall("k::Dog::bark"), pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 2, 0), pData(1, 3, 0), pData(2, 4, 0), pData(4, 3, 1),
			pCtrl(2, 3, 0), pCtrl(2, 4, 0), pData(2, 5, 0),
		},
	}
}

// `if let Some(v) = p.find() { hit(v) } else { miss() }`
func ifLet() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Param), pCall("k::R::find"), pn(Let), pn(Branch),
			pCall("k::R::hit"), pCall("k::R::miss"), pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 1, 0), pData(1, 2, 0), pData(2, 3, 0), pData(2, 4, 1),
			pCtrl(3, 4, 0), pCtrl(3, 5, 1), pData(3, 6, 0),
		},
	}
}

// `match p.find() { Some(v) => hit(v), None => miss() }`
func twoArmMatch() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Param), pCall("k::R::find"), pn(Match),
			pCall("k::R::hit"), pCall("k::R::miss"), pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 1, 0), pData(1, 2, 0), pData(2, 3, 1),
			pCtrl(2, 3, 0), pCtrl(2, 4, 1), pData(2, 5, 0),
		},
	}
}

// literalChainOtherScrutinee: the else-if chain with the inner condition on
// a different value, so the chain must stay nested.
func literalChainOtherScrutinee() *MiningGraph {
	pdg := literalChain()
	pdg.Nodes = append(pdg.Nodes, pn(Param))
	other := len(pdg.Nodes) - 1
	edges := make([]PdgEdge, 0, len(pdg.Edges))
	for _, e := range pdg.Edges {
		if e.From == 0 && e.To == 6 {
			continue
		}
		edges = append(edges, e)
	}
	pdg.Edges = append(edges, pData(other, 6, 0))
	return pdg
}

// `if code == 200 { a } else if code == 404 { b } else { c }` (three arms).
// Go spelling: the extractor emits "cmp:==" details.
func literalChain() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Param), pn(Branch), pOp("cmp:=="), pLit("int", "200"),
			pCall("k::R::a"), pn(Branch), pOp("cmp:=="), pLit("int", "404"),
			pCall("k::R::b"), pCall("k::R::c"), pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 2, 0), pData(3, 2, 1), pData(2, 1, 0),
			pCtrl(1, 4, 0), pCtrl(1, 5, 1), pCtrl(1, 6, 1), pCtrl(1, 7, 1),
			pData(0, 6, 0), pData(7, 6, 1), pData(6, 5, 0),
			pCtrl(5, 8, 0), pCtrl(5, 9, 1), pData(1, 10, 0),
		},
	}
}

func matchOnCode() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Param), pn(Match),
			pCall("k::R::a"), pCall("k::R::b"), pCall("k::R::c"), pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 1, 0),
			pCtrl(1, 2, 0), pCtrl(1, 3, 1), pCtrl(1, 4, 2),
			pData(1, 5, 0),
		},
	}
}

// `for n in 1..=3 { body(n) }`
func rangeForm() *MiningGraph {
	call := pCall("core::ops::range::{impl#7}::new")
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Iterate), call, pLit("int", "1"), pLit("int", "3"), pCall("k::R::body"),
		},
		Edges: []PdgEdge{
			pData(2, 1, 0), pData(3, 1, 1), pData(1, 0, 0),
			pData(0, 4, 1), pCtrl(0, 4, 0),
		},
	}
}

// `let mut n = 0; while n < 3 { n += 1; body(n) }`.
// Go spellings: "cmp:<" and "assignop:+=".
func whileForm() *MiningGraph {
	loop := pn(Loop)
	loop.Detail = "while"
	return &MiningGraph{
		Nodes: []PdgNode{
			pLit("int", "0"), loop, pn(Branch), pOp("cmp:<"),
			pLit("int", "3"), pOp("assignop:+="), pLit("int", "1"), pCall("k::R::body"),
		},
		Edges: []PdgEdge{
			pData(0, 3, 0), pData(4, 3, 1), pData(3, 2, 0),
			pData(0, 5, 1), pData(6, 5, 0), pData(0, 7, 1),
			pCtrl(1, 2, 0), pCtrl(1, 3, 0), pCtrl(1, 4, 0),
			pCtrl(2, 5, 0), pCtrl(2, 6, 0), pCtrl(2, 7, 0),
		},
	}
}

// `i := 0; for i < 3 { use(i); i++ }`: the Go manual-counter spelling.
func incWhileForm() *MiningGraph {
	loop := pn(Loop)
	loop.Detail = "for"
	return &MiningGraph{
		Nodes: []PdgNode{
			pLit("int", "0"), loop, pn(Branch), pOp("cmp:<"),
			pLit("int", "3"), pOp("inc:++"), pCall("k::R::use"),
		},
		Edges: []PdgEdge{
			pData(0, 3, 0), pData(4, 3, 1), pData(3, 2, 0),
			pData(0, 5, 0), pData(0, 6, 1),
			pCtrl(1, 2, 0), pCtrl(1, 3, 0), pCtrl(1, 4, 0),
			pCtrl(2, 5, 0), pCtrl(2, 6, 0),
		},
	}
}

// `match call(p) { Ok(v) => v, Err(e) => return Err(e) }` then `use(v)`.
func explicitPropagation() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pn(Param), pn(Match), pn(Return), pOp("ctor"), pMacro("whatever"),
		},
		Edges: []PdgEdge{
			pData(0, 1, 0), pData(1, 3, 0), pCtrl(1, 3, 1),
			pCtrl(1, 2, 1), pData(3, 2, 0), pData(1, 4, 0),
		},
	}
}

// `if nil != err { return err }`: the residual Go shape the extractor misses
// (nil on the left of !=).
func goResidualNeq() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pCall("k::R::getErr"), pLit("nil", "nil"), pOp("cmp:!="),
			pn(Branch), pn(Return), pCall("k::R::use"),
		},
		Edges: []PdgEdge{
			pData(1, 2, 0), pData(0, 2, 1), pData(2, 3, 0),
			pCtrl(3, 4, 0), pData(0, 4, 0), pCtrl(3, 5, 1),
		},
	}
}

// `if err == nil { work() } else { return err }`: inverted polarity.
func goResidualEq() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			pCall("k::R::getErr"), pLit("nil", "nil"), pOp("cmp:=="),
			pn(Branch), pCall("k::R::work"), pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 2, 0), pData(1, 2, 1), pData(2, 3, 0),
			pCtrl(3, 4, 0), pCtrl(3, 5, 1), pData(0, 5, 0),
		},
	}
}

// The extractor's own Try encoding: Try takes the error value at 0, the
// Return takes it at 0 and is Ctrl-dependent on the Try (arm 0).
func existingTry() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{pCall("k::R::getErr"), pn(Try), pn(Return)},
		Edges: []PdgEdge{pData(0, 1, 0), pData(0, 2, 0), pCtrl(1, 2, 0)},
	}
}

func TestConstantCtorTreeBecomesOneCtor(t *testing.T) {
	out := Canonicalize(nestedConstant(), mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), []NodeKind{Ctor, Return}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if want := []PdgEdge{pData(0, 1, 0)}; !slices.Equal(out.Edges, want) {
		t.Fatalf("edges = %v, want %v", out.Edges, want)
	}
}

func TestControlEdgesIntoAbsorbedNodesMoveToFoldedCtor(t *testing.T) {
	pdg := nestedConstant()
	nodes := append([]PdgNode{pn(Branch)}, pdg.Nodes...)
	pdg = &MiningGraph{Nodes: nodes, Edges: []PdgEdge{
		pCtrl(0, 2, 0), pData(1, 2, 0), pData(2, 3, 0), pData(3, 4, 0),
	}}
	out := Canonicalize(pdg, mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), []NodeKind{Branch, Ctor, Return}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if !slices.Contains(out.Edges, pCtrl(0, 1, 0)) {
		t.Fatalf("missing moved ctrl edge in %v", out.Edges)
	}
	if !slices.Contains(out.Edges, pData(1, 2, 0)) {
		t.Fatalf("missing data edge in %v", out.Edges)
	}
	if len(out.Edges) != 2 {
		t.Fatalf("edges = %v", out.Edges)
	}
}

func TestUnitVariantLiteralBecomesCtor(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			pLit("path", "Mode::Fast"), pn(Return), pOp("tuple"), pOp("ctor"),
		},
		Edges: []PdgEdge{pData(0, 1, 0), pData(2, 3, 0), pData(3, 1, 1)},
	}
	out := Canonicalize(pdg, mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), []NodeKind{Ctor, Return, Ctor}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if out.Nodes[0].Detail != "Mode::Fast" {
		t.Fatalf("hole text lost: %q", out.Nodes[0].Detail)
	}
}

func TestPlainPathLiteralsStayLiterals(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pLit("path", "LIMIT"), pn(Return)},
		Edges: []PdgEdge{pData(0, 1, 0)},
	}
	if !sameGraph(Canonicalize(pdg, mustParse(t, "ctor,pred")), pdg) {
		t.Fatal("plain path literal was rewritten")
	}
}

func TestNonConstantCtorChainMergesIntoOuterCtor(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pCall("alloc::vec::Vec::push"), pOp("ctor"), pOp("ctor"), pn(Return)},
		Edges: []PdgEdge{pData(0, 1, 0), pData(1, 2, 0), pData(2, 3, 0)},
	}
	out := Canonicalize(pdg, mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), []NodeKind{Call, Op, Return}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if want := []PdgEdge{pData(0, 1, 0), pData(1, 2, 0)}; !slices.Equal(out.Edges, want) {
		t.Fatalf("edges = %v, want %v", out.Edges, want)
	}
}

func TestCompositeLiteralFoldsLikeACtor(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			pLit("int", "1"), pLit("int", "2"), pOp("composite:{}"), pn(Return),
		},
		Edges: []PdgEdge{pData(0, 2, 0), pData(1, 2, 1), pData(2, 3, 0)},
	}
	out := Canonicalize(pdg, mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), []NodeKind{Ctor, Return}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if out.Nodes[0].Detail != "composite:{}" {
		t.Fatalf("detail lost: %q", out.Nodes[0].Detail)
	}
	if want := []PdgEdge{pData(0, 1, 0)}; !slices.Equal(out.Edges, want) {
		t.Fatalf("edges = %v, want %v", out.Edges, want)
	}
}

func TestLiteralSharedWithACallIsNotFolded(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pLit("str", `"x"`), pOp("ctor"), pCall("k::R::push")},
		Edges: []PdgEdge{pData(0, 1, 0), pData(0, 2, 1)},
	}
	out := Canonicalize(pdg, mustParse(t, "ctor,pred"))
	if len(out.Nodes) != 3 {
		t.Fatalf("shared literal was folded: %v", pKinds(out))
	}
}

func TestNegatedConjunctionLeavesTwoCallsFeedingBranch(t *testing.T) {
	out := Canonicalize(negatedPair(), mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), []NodeKind{Call, Call, Branch}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if want := []PdgEdge{pData(0, 2, 0), pData(1, 2, 0)}; !slices.Equal(out.Edges, want) {
		t.Fatalf("edges = %v, want %v", out.Edges, want)
	}
	for _, n := range out.Nodes[:2] {
		if n.Detail != "!" {
			t.Fatalf("negation flag lost: %+v", n)
		}
	}
}

func TestSingleCallPredicateMatchesNegatedForm(t *testing.T) {
	plain := &MiningGraph{
		Nodes: []PdgNode{pCall("k::A::ok"), pn(Branch)},
		Edges: []PdgEdge{pData(0, 1, 0)},
	}
	negated := &MiningGraph{
		Nodes: []PdgNode{pCall("k::A::ok"), pOp("!"), pn(Branch)},
		Edges: []PdgEdge{pData(0, 1, 0), pData(1, 2, 0)},
	}
	out := Canonicalize(negated, mustParse(t, "ctor,pred"))
	if !slices.Equal(pKinds(out), pKinds(plain)) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if !slices.Equal(out.Edges, plain.Edges) {
		t.Fatalf("edges = %v, want %v", out.Edges, plain.Edges)
	}
}

func TestCombinatorsOverNonCallsLeftAlone(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pLit("bool", "true"), pOp("!"), pn(Branch)},
		Edges: []PdgEdge{pData(0, 1, 0), pData(1, 2, 0)},
	}
	if !sameGraph(Canonicalize(pdg, mustParse(t, "ctor,pred")), pdg) {
		t.Fatal("non-call combinator was rewritten")
	}
}

func TestRulesAreIdempotentAndNeverGrow(t *testing.T) {
	rules := mustParse(t, "ctor,pred")
	for _, pdg := range []*MiningGraph{nestedConstant(), negatedPair(), emitLoop()} {
		once := Canonicalize(pdg, rules)
		if !sameGraph(Canonicalize(once, rules), once) {
			t.Fatal("not idempotent")
		}
		if len(once.Nodes) > len(pdg.Nodes) {
			t.Fatal("node count grew")
		}
		if len(once.Edges) > len(pdg.Edges)+2 {
			t.Fatal("edge count grew too much")
		}
	}
}

func TestGraphsWithoutMatchingShapesUnchanged(t *testing.T) {
	pdg := emitLoop()
	if !sameGraph(Canonicalize(pdg, mustParse(t, "ctor,pred")), pdg) {
		t.Fatal("loop over collection was rewritten")
	}
}

func TestNoRulesIsIdentity(t *testing.T) {
	pdg := negatedPair()
	if !sameGraph(Canonicalize(pdg, Rules{}), pdg) {
		t.Fatal("no-rules canonicalize changed the graph")
	}
}

func TestParseRules(t *testing.T) {
	if got := mustParse(t, "ctor"); got != (Rules{Ctor: true}) {
		t.Fatalf("parse ctor = %+v", got)
	}
	if got := mustParse(t, "ctor, pred,case,exit,counter,cmp,try"); got != RulesAll {
		t.Fatalf("parse all names = %+v", got)
	}
	if got := mustParse(t, "all"); got != RulesAll {
		t.Fatalf("parse all = %+v", got)
	}
	if got := mustParse(t, ""); !got.IsNone() {
		t.Fatalf("parse empty = %+v", got)
	}
	assertParseError(t, "bogus")
}

func assertParseError(t *testing.T, text string) {
	t.Helper()
	_, err := ParseRules(text)
	if err == nil {
		t.Fatalf("ParseRules(%q): expected error", text)
	}
	want := "unknown normalize rule `bogus` (ctor, pred, case, exit, counter, cmp, try, all)"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestRewrittenGraphsKeepValidIndices(t *testing.T) {
	out := Canonicalize(negatedPair(), mustParse(t, "ctor,pred"))
	n := len(out.Nodes)
	for _, e := range out.Edges {
		if e.From >= n || e.To >= n {
			t.Fatalf("dangling edge %+v in %d nodes", e, n)
		}
	}
}

func TestIfLetAndMatchBecomeSameCase(t *testing.T) {
	rules := mustParse(t, "case")
	a := Canonicalize(ifLet(), rules)
	b := Canonicalize(twoArmMatch(), rules)
	if !sameGraph(a, b) {
		t.Fatalf("differ:\n%+v\n%+v", a, b)
	}
	if a.Nodes[2].Kind != Case {
		t.Fatalf("node 2 = %v, want Case", a.Nodes[2].Kind)
	}
}

func TestLiteralElseIfChainEqualsMatchOnScrutinee(t *testing.T) {
	rules := mustParse(t, "case")
	a := Canonicalize(literalChain(), rules)
	b := Canonicalize(matchOnCode(), rules)
	if !sameGraph(a, b) {
		t.Fatalf("differ:\n%+v\n%+v", a, b)
	}
}

func TestElseIfOnDifferentScrutineeStaysNested(t *testing.T) {
	out := Canonicalize(literalChainOtherScrutinee(), mustParse(t, "case"))
	count := 0
	for _, n := range out.Nodes {
		if n.Kind == Case {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("cases = %d, want 2", count)
	}
}

func TestPlainBooleanBranchBecomesCase(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pCall("k::R::ok"), pn(Branch)},
		Edges: []PdgEdge{pData(0, 1, 0)},
	}
	out := Canonicalize(pdg, mustParse(t, "case"))
	if !slices.Equal(pKinds(out), []NodeKind{Call, Case}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if !slices.Equal(out.Edges, pdg.Edges) {
		t.Fatalf("edges = %v, want %v", out.Edges, pdg.Edges)
	}
}

func TestUnreachableTailDroppedButMacroWithInputsKept(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pn(Param), pMacro("$crate::panic::unreachable_2021")},
	}
	rules := mustParse(t, "exit")
	if got := Canonicalize(pdg, rules); len(got.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(got.Nodes))
	}
	used := &MiningGraph{
		Nodes: pdg.Nodes,
		Edges: []PdgEdge{pData(0, 1, 0)},
	}
	if !sameGraph(Canonicalize(used, rules), used) {
		t.Fatal("macro with inputs was dropped")
	}
}

func TestCaseRulesAreIdempotent(t *testing.T) {
	for _, pdg := range []*MiningGraph{ifLet(), twoArmMatch(), literalChain(), matchOnCode()} {
		once := Canonicalize(pdg, RulesAll)
		if !sameGraph(Canonicalize(once, RulesAll), once) {
			t.Fatal("not idempotent")
		}
		if len(once.Nodes) > len(pdg.Nodes) {
			t.Fatal("node count grew")
		}
	}
}

func TestRangeForLoopBecomesLoopKeepingBoundsInDetail(t *testing.T) {
	out := Canonicalize(rangeForm(), mustParse(t, "counter,cmp"))
	if !slices.Equal(pKinds(out), []NodeKind{Loop, Call}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if out.Nodes[0].Detail != "1..3" {
		t.Fatalf("detail = %q, want 1..3", out.Nodes[0].Detail)
	}
}

func TestWhileWithManualCounterEqualsRangeLoop(t *testing.T) {
	rules := mustParse(t, "counter,cmp")
	w := Canonicalize(whileForm(), rules)
	r := Canonicalize(rangeForm(), rules)
	if !slices.Equal(pKinds(w), pKinds(r)) {
		t.Fatalf("kinds %v vs %v", pKinds(w), pKinds(r))
	}
	if !slices.Equal(sortedEdges(w), sortedEdges(r)) {
		t.Fatalf("edges differ:\n%v\n%v", sortedEdges(w), sortedEdges(r))
	}
	want := []PdgEdge{pCtrl(0, 1, 0), pData(0, 1, 1)} // sortedEdges order
	if !slices.Equal(sortedEdges(w), want) {
		t.Fatalf("edges = %v, want %v", sortedEdges(w), want)
	}
}

func TestManualCounterWithIncStep(t *testing.T) {
	out := Canonicalize(incWhileForm(), mustParse(t, "counter,cmp"))
	if !slices.Equal(pKinds(out), []NodeKind{Loop, Call}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	want := []PdgEdge{pCtrl(0, 1, 0), pData(0, 1, 1)} // sortedEdges order
	if !slices.Equal(sortedEdges(out), want) {
		t.Fatalf("edges = %v, want %v", sortedEdges(out), want)
	}
	// Idempotent, and the Go spelling matches the += spelling.
	again := Canonicalize(out, RulesAll)
	if !sameGraph(again, out) {
		t.Fatal("not idempotent")
	}
	plusEq := Canonicalize(whileForm(), mustParse(t, "counter,cmp"))
	if !slices.Equal(sortedEdges(out), sortedEdges(plusEq)) {
		t.Fatalf("i++ form differs from += form:\n%v\n%v", sortedEdges(out), sortedEdges(plusEq))
	}
}

func TestLoopOverCollectionIsNotCounterLoop(t *testing.T) {
	pdg := emitLoop()
	if !sameGraph(Canonicalize(pdg, mustParse(t, "counter,cmp")), pdg) {
		t.Fatal("loop over collection was rewritten")
	}
}

func TestRangeWithNonLiteralBoundStaysIterate(t *testing.T) {
	pdg := rangeForm()
	pdg.Nodes[3] = pn(Param)
	out := Canonicalize(pdg, mustParse(t, "counter,cmp"))
	if out.Nodes[0].Kind != Iterate {
		t.Fatalf("kind = %v, want Iterate", out.Nodes[0].Kind)
	}
}

func TestCounterComparisonHasOneClassWhateverTheOperator(t *testing.T) {
	with := func(detail string) *MiningGraph {
		pdg := &MiningGraph{
			Nodes: []PdgNode{pn(Loop), pLit("int", "3"), pOp(detail), pn(Branch)},
			Edges: []PdgEdge{pData(0, 2, 0), pData(1, 2, 1), pData(2, 3, 0)},
		}
		return Canonicalize(pdg, mustParse(t, "counter,cmp"))
	}
	lt, eq := with("cmp:<"), with("cmp:==")
	if !slices.Equal(lt.Edges, eq.Edges) {
		t.Fatalf("edges differ:\n%v\n%v", lt.Edges, eq.Edges)
	}
	if label(lt.Nodes[2]) != label(eq.Nodes[2]) {
		t.Fatal("labels differ")
	}
	// The original operator survives as the hole value; raw Rust spellings
	// normalize into the cmp class too.
	if lt.Nodes[2].Detail != "cmp:<" || eq.Nodes[2].Detail != "cmp:==" {
		t.Fatalf("details = %q, %q", lt.Nodes[2].Detail, eq.Nodes[2].Detail)
	}
	if raw := with("<"); raw.Nodes[2].Detail != "cmp:<" {
		t.Fatalf("raw detail = %q, want cmp:<", raw.Nodes[2].Detail)
	}
}

func TestComparisonOfNonCounterLeftAlone(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pn(Param), pLit("int", "3"), pOp("cmp:<"), pn(Branch)},
		Edges: []PdgEdge{pData(0, 2, 0), pData(1, 2, 1), pData(2, 3, 0)},
	}
	if !sameGraph(Canonicalize(pdg, mustParse(t, "counter,cmp")), pdg) {
		t.Fatal("non-counter comparison was rewritten")
	}
}

func TestCounterRulesAreIdempotentAndShrink(t *testing.T) {
	for _, pdg := range []*MiningGraph{rangeForm(), whileForm(), incWhileForm()} {
		once := Canonicalize(pdg, RulesAll)
		if !sameGraph(Canonicalize(once, RulesAll), once) {
			t.Fatal("not idempotent")
		}
		if len(once.Nodes) >= len(pdg.Nodes) {
			t.Fatal("node count did not shrink")
		}
	}
}

func TestExplicitErrorPropagationBecomesTry(t *testing.T) {
	out := Canonicalize(explicitPropagation(), mustParse(t, "try"))
	if !slices.Equal(pKinds(out), []NodeKind{Param, Try, macroKind}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	if want := []PdgEdge{pData(0, 1, 0), pData(1, 2, 0)}; !slices.Equal(out.Edges, want) {
		t.Fatalf("edges = %v, want %v", out.Edges, want)
	}
}

func TestMatchWithOtherArmWorkIsNotTry(t *testing.T) {
	g := explicitPropagation()
	g.Nodes = append(g.Nodes, pCall("k::R::work"))
	g.Edges = append(g.Edges, pCtrl(1, 5, 0))
	if !sameGraph(Canonicalize(g, mustParse(t, "try")), g) {
		t.Fatal("match with other arm work became Try")
	}
}

func TestReturningSomethingElseIsNotTry(t *testing.T) {
	g := explicitPropagation()
	var edges []PdgEdge
	for _, e := range g.Edges {
		if !(e.From == 1 && e.To == 3 && e.Kind == Data) {
			edges = append(edges, e)
		}
	}
	edges = append(edges, pData(0, 3, 0))
	g.Edges = edges
	if !sameGraph(Canonicalize(g, mustParse(t, "try")), g) {
		t.Fatal("returning something else became Try")
	}
}

func TestPropagationRuleIsIdempotentAndShrinks(t *testing.T) {
	once := Canonicalize(explicitPropagation(), mustParse(t, "try"))
	if !sameGraph(Canonicalize(once, mustParse(t, "try")), once) {
		t.Fatal("not idempotent")
	}
	if len(once.Nodes) >= len(explicitPropagation().Nodes) {
		t.Fatal("node count did not shrink")
	}
}

func TestGoResidualNeqNilBecomesTry(t *testing.T) {
	out := Canonicalize(goResidualNeq(), mustParse(t, "try"))
	if !slices.Equal(pKinds(out), []NodeKind{Call, Try, Call}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	want := []PdgEdge{pData(0, 1, 0), pCtrl(1, 2, 1)}
	if !slices.Equal(sortedEdges(out), want) {
		t.Fatalf("edges = %v, want %v", sortedEdges(out), want)
	}
	// Idempotent: a second pass must not touch the Try.
	if !sameGraph(Canonicalize(out, RulesAll), out) {
		t.Fatal("Try was double-converted")
	}
}

func TestGoResidualEqNilElseReturnBecomesTry(t *testing.T) {
	out := Canonicalize(goResidualEq(), mustParse(t, "try"))
	if !slices.Equal(pKinds(out), []NodeKind{Call, Try, Call}) {
		t.Fatalf("kinds = %v", pKinds(out))
	}
	want := []PdgEdge{pData(0, 1, 0), pCtrl(1, 2, 0)}
	if !slices.Equal(sortedEdges(out), want) {
		t.Fatalf("edges = %v, want %v", sortedEdges(out), want)
	}
}

func TestExistingTryIsNotDoubleConverted(t *testing.T) {
	pdg := existingTry()
	if !sameGraph(Canonicalize(pdg, mustParse(t, "try")), pdg) {
		t.Fatal("existing Try was rewritten")
	}
	if !sameGraph(Canonicalize(pdg, RulesAll), pdg) {
		t.Fatal("existing Try was rewritten by full rules")
	}
}

func TestDeferWrappingSurvives(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pn(Defer), pCall("k::R::cleanup"), pn(Return)},
		Edges: []PdgEdge{pData(1, 0, 0), pData(1, 2, 0)},
	}
	if !sameGraph(Canonicalize(pdg, RulesAll), pdg) {
		t.Fatal("defer wrapping was disturbed")
	}
}

func TestDeferWrappingSurvivesPredicateCollapse(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{
			pCall("k::A::ok"), pCall("k::B::ok"), pOp("logic:&&"),
			pCall("k::R::f"), pn(Defer),
		},
		Edges: []PdgEdge{
			pData(0, 2, 0), pData(1, 2, 1), pData(2, 3, 0), pData(3, 4, 0),
		},
	}
	out := Canonicalize(pdg, mustParse(t, "pred"))
	want := &MiningGraph{
		Nodes: []PdgNode{pCall("k::A::ok"), pCall("k::B::ok"), pCall("k::R::f"), pn(Defer)},
		Edges: []PdgEdge{pData(0, 2, 0), pData(1, 2, 0), pData(2, 3, 0)},
	}
	if !sameGraph(out, want) {
		t.Fatalf("got kinds %v edges %v", pKinds(out), out.Edges)
	}
}

func TestGoNodeWrappingSurvives(t *testing.T) {
	pdg := &MiningGraph{
		Nodes: []PdgNode{pn(Go), pCall("k::R::work"), pn(Return)},
		Edges: []PdgEdge{pData(1, 0, 0), pData(1, 2, 0)},
	}
	if !sameGraph(Canonicalize(pdg, RulesAll), pdg) {
		t.Fatal("go wrapping was disturbed")
	}
}
