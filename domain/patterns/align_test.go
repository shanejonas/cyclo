package patterns

import (
	"reflect"
	"strconv"
	"testing"
)

func dataEdge(from, to, pos int) PdgEdge {
	return PdgEdge{From: from, To: to, Kind: Data, ArgPos: pos}
}

func ctrlEdge(from, to, pos int) PdgEdge {
	return PdgEdge{From: from, To: to, Kind: Ctrl, ArgPos: pos}
}

func runAlign(a, b *Pdg) Alignment {
	return Align(a, NewWl(a), b, NewWl(b))
}

// permute shifts every node index by shift (mod n), like rstyle's
// pdgtest::permuted.
func permute(pdg *Pdg, shift int) *Pdg {
	n := len(pdg.Nodes)
	nodes := make([]PdgNode, n)
	for old, node := range pdg.Nodes {
		nodes[(old+shift)%n] = node
	}
	edges := make([]PdgEdge, len(pdg.Edges))
	for i, e := range pdg.Edges {
		edges[i] = PdgEdge{
			From:   (e.From + shift) % n,
			To:     (e.To + shift) % n,
			Kind:   e.Kind,
			ArgPos: e.ArgPos,
		}
	}
	return &Pdg{Nodes: nodes, Edges: edges}
}

func holeKinds(al Alignment) [][3]string {
	var out [][3]string
	for _, h := range al.Holes {
		out = append(out, [3]string{string(h.Kind), h.A, h.B})
	}
	return out
}

func TestHoleVarNaming(t *testing.T) {
	cases := []struct {
		kind HoleKind
		want byte
	}{
		{HoleType, 'T'},
		{HoleMethod, 'M'},
		{HoleFreeFn, 'F'},
		{HoleLiteral, 'L'},
		{HoleField, 'D'},
		{HoleOp, 'O'},
	}
	for i, c := range cases {
		if got := c.kind.Prefix(); got != c.want {
			t.Errorf("case %d: Prefix() = %c, want %c", i, got, c.want)
		}
		wantVar := string([]byte{c.want}) + strconv.Itoa(i)
		if got := HoleVar(c.kind, i); got != wantVar {
			t.Errorf("case %d: HoleVar = %q, want %q", i, got, wantVar)
		}
	}
}

// cmpGraph is loop { lit <op> } -> case, mirroring the Rust op-hole test.
func cmpGraph(op string) *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			{Kind: Loop},
			{Kind: Lit, LitKind: "int", Detail: "3"},
			{Kind: Op, Detail: "cmp:" + op},
			{Kind: Case},
		},
		Edges: []PdgEdge{
			dataEdge(0, 2, 0),
			dataEdge(1, 2, 1),
			dataEdge(2, 3, 0),
		},
	}
}

func TestCanonicalComparisonOperatorsDifferAsOpHole(t *testing.T) {
	al := runAlign(cmpGraph("<"), cmpGraph("=="))
	if al.CoverageMilli != 1000 {
		t.Errorf("coverage = %d, want 1000", al.CoverageMilli)
	}
	want := [][3]string{{"op", "<", "=="}}
	if got := holeKinds(al); !reflect.DeepEqual(got, want) {
		t.Errorf("holes = %v, want %v", got, want)
	}
	if al.Holes[0].Var != "O0" {
		t.Errorf("hole var = %q, want O0", al.Holes[0].Var)
	}
	if got := runAlign(cmpGraph("<"), cmpGraph("<")).Holes; len(got) != 0 {
		t.Errorf("same operator holes = %v, want none", got)
	}
}

// emitLoop is the Go analogue of rstyle's pdgtest::emit_loop: a loop pushing
// x.method() into out. SigClass is identical across the pair, so only the
// callee id holes.
func methodLoop(ty, method string) *Pdg {
	param := func(tyClass string) PdgNode { return PdgNode{Kind: Param, TyClass: tyClass} }
	return &Pdg{
		Nodes: []PdgNode{
			param("[]_"),       // 0
			param("*[]string"), // 1
			{Kind: Iterate},    // 2
			{Kind: Call, CalleeID: "example.com/vec.Vec.Push", SigClass: "fn(*_, _) -> ()"},                // 3
			{Kind: Call, CalleeID: "example.com/shapes." + ty + "." + method, SigClass: "fn(_) -> string"}, // 4
			{Kind: Return}, // 5
		},
		Edges: []PdgEdge{
			dataEdge(0, 2, 0),
			dataEdge(1, 3, 0),
			dataEdge(2, 4, 0),
			dataEdge(4, 3, 1),
			ctrlEdge(2, 3, 0),
		},
	}
}

func TestMethodCallYieldsMethodHole(t *testing.T) {
	al := runAlign(methodLoop("Dog", "Bark"), methodLoop("Cat", "Meow"))
	if len(al.Pairs) != 6 {
		t.Fatalf("pairs = %d, want 6", len(al.Pairs))
	}
	if al.CoverageMilli != 1000 {
		t.Errorf("coverage = %d, want 1000", al.CoverageMilli)
	}
	want := [][3]string{{"method", "example.com/shapes.Dog.Bark", "example.com/shapes.Cat.Meow"}}
	if got := holeKinds(al); !reflect.DeepEqual(got, want) {
		t.Errorf("holes = %v, want %v", got, want)
	}
	if al.Holes[0].Var != "M0" {
		t.Errorf("hole var = %q, want M0", al.Holes[0].Var)
	}
	if got := runAlign(methodLoop("Dog", "Bark"), methodLoop("Dog", "Bark")).Holes; len(got) != 0 {
		t.Errorf("identical graphs holes = %v, want none", got)
	}
}

// callGraph is a single call of callee fed by one param.
func callGraph(callee string) *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "int"},
			{Kind: Call, CalleeID: callee, SigClass: "fn(int) -> int", TyClass: "int"},
		},
		Edges: []PdgEdge{dataEdge(0, 1, 0)},
	}
}

func TestFreeFnHole(t *testing.T) {
	al := runAlign(callGraph("example.com/p.f"), callGraph("example.com/p.g"))
	want := [][3]string{{"free_fn", "example.com/p.f", "example.com/p.g"}}
	if got := holeKinds(al); !reflect.DeepEqual(got, want) {
		t.Fatalf("holes = %v, want %v", got, want)
	}
	if al.Holes[0].Var != "F0" {
		t.Errorf("hole var = %q, want F0", al.Holes[0].Var)
	}
	if al.CoverageMilli != 1000 {
		t.Errorf("coverage = %d, want 1000", al.CoverageMilli)
	}
}

func TestMethodVsFreeFnKind(t *testing.T) {
	// Method vs method: the "pkg.Type.Name" id (two dots past the slash)
	// marks a receiver.
	al := runAlign(callGraph("example.com/p.T.f"), callGraph("example.com/p.T.g"))
	if len(al.Holes) != 1 || al.Holes[0].Kind != HoleMethod {
		t.Errorf("method/method holes = %v, want one method hole", al.Holes)
	}
	// Method vs free function: either side having a receiver makes it a
	// Method hole, mirroring Rust's is_method(a) || is_method(b).
	al = runAlign(callGraph("example.com/p.T.f"), callGraph("example.com/p.g"))
	if len(al.Holes) != 1 || al.Holes[0].Kind != HoleMethod {
		t.Errorf("method/freefn holes = %v, want one method hole", al.Holes)
	}
}

func TestLiteralHole(t *testing.T) {
	lit := func(detail string) *Pdg {
		return &Pdg{
			Nodes: []PdgNode{
				{Kind: Lit, LitKind: "string", Detail: detail},
				{Kind: Return},
			},
			Edges: []PdgEdge{dataEdge(0, 1, 0)},
		}
	}
	al := runAlign(lit("hello"), lit("world"))
	want := [][3]string{{"literal", "hello", "world"}}
	if got := holeKinds(al); !reflect.DeepEqual(got, want) {
		t.Fatalf("holes = %v, want %v", got, want)
	}
	if al.Holes[0].Var != "L0" {
		t.Errorf("hole var = %q, want L0", al.Holes[0].Var)
	}
	if al.CoverageMilli != 1000 {
		t.Errorf("coverage = %d, want 1000", al.CoverageMilli)
	}
	if got := runAlign(lit("hello"), lit("hello")).Holes; len(got) != 0 {
		t.Errorf("same literal holes = %v, want none", got)
	}
}

// fieldGraph is x.<field> where the field's type class differs: the Go
// analogue of Rust's adt_diffs, which the port reduces to a TyClass
// inequality.
func fieldGraph(fieldTy string) *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "T"},
			{Kind: Field, Detail: "Name", TyClass: fieldTy},
		},
		Edges: []PdgEdge{dataEdge(0, 1, 0)},
	}
}

func TestFieldTypeHole(t *testing.T) {
	al := runAlign(fieldGraph("string"), fieldGraph("int"))
	want := [][3]string{{"type", "string", "int"}}
	if got := holeKinds(al); !reflect.DeepEqual(got, want) {
		t.Fatalf("holes = %v, want %v", got, want)
	}
	if al.Holes[0].Var != "T0" {
		t.Errorf("hole var = %q, want T0", al.Holes[0].Var)
	}
	if al.CoverageMilli != 1000 {
		t.Errorf("coverage = %d, want 1000", al.CoverageMilli)
	}
	if got := runAlign(fieldGraph("string"), fieldGraph("string")).Holes; len(got) != 0 {
		t.Errorf("same type holes = %v, want none", got)
	}
}

func TestRepeatedPairReusesHoleVar(t *testing.T) {
	two := func(t1, t2 string) *Pdg {
		return &Pdg{
			Nodes: []PdgNode{
				{Kind: Param, TyClass: "T"},
				{Kind: Field, Detail: "A", TyClass: t1},
				{Kind: Field, Detail: "B", TyClass: t2},
			},
			Edges: []PdgEdge{dataEdge(0, 1, 0), dataEdge(0, 2, 0)},
		}
	}
	al := runAlign(two("string", "string"), two("int", "int"))
	if len(al.Holes) != 1 {
		t.Fatalf("holes = %v, want exactly one shared type hole", al.Holes)
	}
	h := al.Holes[0]
	if h.Var != "T0" || h.A != "string" || h.B != "int" {
		t.Errorf("hole = %+v, want T0 = string | int", h)
	}
	wantSites := [][2]int{{1, 1}, {2, 2}}
	if !reflect.DeepEqual(h.Sites, wantSites) {
		t.Errorf("sites = %v, want %v", h.Sites, wantSites)
	}
}

func TestCoverageMilli(t *testing.T) {
	full := cmpGraph("<")
	short := &Pdg{
		Nodes: []PdgNode{
			{Kind: Loop},
			{Kind: Lit, LitKind: "int", Detail: "3"},
			{Kind: Op, Detail: "cmp:<"},
		},
		Edges: []PdgEdge{dataEdge(0, 2, 0), dataEdge(1, 2, 1)},
	}
	for _, tc := range []struct {
		name string
		a, b *Pdg
		want uint32
	}{
		{"identical", full, cmpGraph("<"), 1000},
		{"smaller_b", full, short, 750},
		{"smaller_a", short, full, 750},
	} {
		if got := runAlign(tc.a, tc.b).CoverageMilli; got != tc.want {
			t.Errorf("%s: coverage = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSymmetricGraphAlignmentDeterministic(t *testing.T) {
	push := PdgNode{Kind: Call, CalleeID: "example.com/vec.Vec.Push", SigClass: "fn(int) -> ()"}
	twin := &Pdg{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "int"},
			{Kind: Param, TyClass: "int"},
			push,
			push,
		},
		Edges: []PdgEdge{dataEdge(0, 2, 0), dataEdge(1, 3, 0)},
	}
	flipped := permute(twin, 1)
	first := runAlign(twin, flipped)
	if len(first.Pairs) != 4 {
		t.Fatalf("pairs = %d, want 4", len(first.Pairs))
	}
	if len(first.Holes) != 0 {
		t.Errorf("holes = %v, want none", first.Holes)
	}
	if second := runAlign(twin, flipped); !reflect.DeepEqual(first.Pairs, second.Pairs) {
		t.Errorf("nondeterministic: %v vs %v", first.Pairs, second.Pairs)
	}
	// The greedy pairing must already be edge-consistent.
	wa, wb := NewWl(twin), NewWl(flipped)
	c := newCtx(twin, wa, flipped, wb)
	m := newMatching(4, 4)
	for _, p := range first.Pairs {
		m.link(p[0], p[1])
	}
	if got := len(prune(c, m).pairs()); got != 4 {
		t.Errorf("prune kept %d pairs, want 4 (edge-consistent)", got)
	}
}

func TestEdgeInconsistentMatchRejected(t *testing.T) {
	push := PdgNode{Kind: Call, CalleeID: "example.com/vec.Vec.Push", SigClass: "fn() -> ()"}
	linked := &Pdg{
		Nodes: []PdgNode{push, push},
		Edges: []PdgEdge{dataEdge(0, 1, 0)},
	}
	loose := &Pdg{Nodes: []PdgNode{push, push}}
	c := newCtx(linked, NewWl(linked), loose, NewWl(loose))
	both := newMatching(2, 2)
	both.link(0, 0)
	both.link(1, 1)
	if kept := prune(c, both).pairs(); len(kept) != 1 {
		t.Errorf("prune kept %d pairs, want 1 (one clashing pair must go)", len(kept))
	}
	single := newMatching(2, 2)
	single.link(0, 0)
	if kept := prune(c, single).pairs(); len(kept) != 1 {
		t.Errorf("prune kept %d pairs of single link, want 1", len(kept))
	}
}

func TestDifferingSigClassStillPairsAsHole(t *testing.T) {
	a := &Pdg{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "int"},
			{Kind: Call, CalleeID: "example.com/p.f", SigClass: "fn(int) -> string", TyClass: "string"},
		},
		Edges: []PdgEdge{dataEdge(0, 1, 0)},
	}
	b := &Pdg{
		Nodes: []PdgNode{
			{Kind: Param, TyClass: "int"},
			{Kind: Call, CalleeID: "example.com/p.f", SigClass: "fn(int) -> int", TyClass: "int"},
		},
		Edges: []PdgEdge{dataEdge(0, 1, 0)},
	}
	al := runAlign(a, b)
	if len(al.Pairs) != 2 {
		t.Fatalf("pairs = %d, want 2", len(al.Pairs))
	}
	want := [][3]string{{"type", "string", "int"}}
	if got := holeKinds(al); !reflect.DeepEqual(got, want) {
		t.Errorf("holes = %v, want %v", got, want)
	}
}

func TestHoleOrderingAndNumbering(t *testing.T) {
	multi := func(fieldTy, methodCallee, freeCallee string) *Pdg {
		return &Pdg{
			Nodes: []PdgNode{
				{Kind: Param, TyClass: "T"},
				{Kind: Field, Detail: "Name", TyClass: fieldTy},
				{Kind: Call, CalleeID: methodCallee, SigClass: "fn(T) -> ()"},
				{Kind: Call, CalleeID: freeCallee, SigClass: "fn() -> ()"},
			},
			Edges: []PdgEdge{dataEdge(0, 1, 0), dataEdge(1, 2, 0), dataEdge(2, 3, 0)},
		}
	}
	a := multi("string", "example.com/p.T.F", "example.com/p.h")
	b := multi("int", "example.com/p.T.G", "example.com/p.i")
	al := runAlign(a, b)
	if al.CoverageMilli != 1000 {
		t.Fatalf("coverage = %d, want 1000", al.CoverageMilli)
	}
	// Holes order by first site, then kind; each kind numbered from zero.
	var got [][2]string
	for _, h := range al.Holes {
		got = append(got, [2]string{h.Var, string(h.Kind)})
	}
	want := [][2]string{{"T0", "type"}, {"M0", "method"}, {"F0", "free_fn"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("holes = %v, want %v", got, want)
	}
}

func TestEveryVariantFullyAligns(t *testing.T) {
	a, b := methodLoop("Dog", "Bark"), methodLoop("Cat", "Meow")
	variants := []Variant{Baseline, LabelsOnly, NoFinalPrune, CompleteChecked, CoarseToFine, Combined}
	for _, v := range variants {
		al, stages := AlignVariant(a, NewWl(a), b, NewWl(b), v)
		if al.CoverageMilli != 1000 {
			t.Errorf("variant %d: coverage = %d, want 1000", v, al.CoverageMilli)
		}
		if len(al.Holes) != 1 {
			t.Errorf("variant %d: holes = %d, want 1", v, len(al.Holes))
		}
		if len(stages) == 0 {
			t.Errorf("variant %d: no stages reported", v)
		}
	}
}

func TestAlignStagedReportsPipeline(t *testing.T) {
	al, stages := AlignStaged(methodLoop("Dog", "Bark"), NewWl(methodLoop("Dog", "Bark")),
		methodLoop("Cat", "Meow"), NewWl(methodLoop("Cat", "Meow")))
	wantNames := []string{"seed", "settle", "prune", "labels", "prune_labels", "complete", "final"}
	if len(stages) != len(wantNames) {
		t.Fatalf("stages = %d, want %d", len(stages), len(wantNames))
	}
	for i, name := range wantNames {
		if stages[i].Name != name {
			t.Errorf("stage %d = %q, want %q", i, stages[i].Name, name)
		}
	}
	if !reflect.DeepEqual(al.Pairs, stages[len(stages)-1].Pairs) {
		t.Errorf("alignment pairs != final stage pairs")
	}
}
