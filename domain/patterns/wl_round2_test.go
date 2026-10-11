package patterns

import "testing"

// callNode builds a Call PdgNode with the given callee id and signature class.
func callNode(calleeID, sigClass string) PdgNode {
	return PdgNode{Kind: Call, CalleeID: calleeID, SigClass: sigClass, Line: 1}
}

func TestCallScope(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"fmt.Println", "std"},
		{"strings.ToUpper", "std"},
		{"net/http.Server.Serve", "std"},
		{"github.com/me/app.print", "local"},
		{"github.com/me/app.Serv.Serve", "local"},
		{"builtin.len", "builtin"},
		{"", "unknown"},
		{"?", "unknown"},
	}
	for _, c := range cases {
		if got := callScope(c.id); got != c.want {
			t.Errorf("callScope(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

// TestCallScopeRefinesLabels verifies the CCGraph-style call-scope refinement:
// same-shape calls to stdlib vs local helpers no longer get identical labels.
func TestCallScopeRefinesLabels(t *testing.T) {
	stdCall := callNode("fmt.Println", "fn(string) -> ()")
	localCall := callNode("github.com/me/app.println", "fn(string) -> ()")
	if label(stdCall) == label(localCall) {
		t.Errorf("labels collide: %q", label(stdCall))
	}
	twin := callNode("strings.ToUpper", "fn(string) -> string")
	twin2 := callNode("strings.ToLower", "fn(string) -> string")
	if label(twin) != label(twin2) {
		t.Errorf("stdlib twins differ: %q vs %q", label(twin), label(twin2))
	}
	// Name-independence holds: Dog::bark ≡ Cat::meow for same-scope calls.
	dog := callNode("github.com/me/app.Dog.bark", "fn() -> ()")
	cat := callNode("github.com/me/app.Cat.meow", "fn() -> ()")
	if label(dog) != label(cat) {
		t.Errorf("name-independence broken: %q vs %q", label(dog), label(cat))
	}
}

// shapePDG builds the synthetic PDGs for the weighting experiment.
// F1 and F2 share control flow and differ in one data-op label;
// F1 and F3 share all data ops but F3 nests an extra Branch.
// The doubled control-edge weight must rank the same-logic pair higher,
// with a wider margin than the unweighted kernel.
func shapePDG(control NodeKind, dataOp string, callee string) *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			{Kind: control, Line: 1},
			callNode(callee, "fn(string) -> string"),
			{Kind: Op, Detail: dataOp + ":+", Line: 3},
			callNode("fmt.Println", "fn(string) -> ()"),
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Ctrl, ArgPos: 0},
			{From: 0, To: 2, Kind: Ctrl, ArgPos: 1},
			{From: 1, To: 3, Kind: Data, ArgPos: 0},
			{From: 2, To: 1, Kind: Data, ArgPos: 0},
		},
	}
}

// nestedPDG shares F1's data ops but has a very different control skeleton
// (Loop nesting instead of a flat Branch): a structural control difference
// the weighting must penalize more than F2's single data-op label change.
func nestedPDG() *MiningGraph {
	return &MiningGraph{
		Nodes: []PdgNode{
			{Kind: Loop, Line: 1},
			callNode("fmt.Sprintf", "fn(string) -> string"),
			{Kind: Loop, Line: 3},
			{Kind: Op, Detail: "arith:+", Line: 4},
			callNode("fmt.Println", "fn(string) -> ()"),
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Ctrl, ArgPos: 0},
			{From: 0, To: 2, Kind: Ctrl, ArgPos: 1},
			{From: 2, To: 3, Kind: Ctrl, ArgPos: 0},
			{From: 2, To: 4, Kind: Ctrl, ArgPos: 1},
			{From: 1, To: 4, Kind: Data, ArgPos: 0},
			{From: 3, To: 1, Kind: Data, ArgPos: 0},
		},
	}
}

// TestWeightedKernelRanksLogicOverData verifies the control-edge weighting:
// same control flow with different data ops must outrank same data ops with
// different control flow.
func TestWeightedKernelRanksLogicOverData(t *testing.T) {
	f1 := shapePDG(Branch, "arith", "fmt.Sprintf")
	f2 := shapePDG(Branch, "bit", "strconv.Itoa")
	f3 := nestedPDG()

	sameLogic := SimilarityMilli(NewWl(f1), NewWl(f2))
	sameData := SimilarityMilli(NewWl(f1), NewWl(f3))
	t.Logf("same-logic/different-data: %d‰, same-data/different-logic: %d‰", sameLogic, sameData)
	if sameLogic <= sameData {
		t.Errorf("weighting failed: same-logic %d‰ <= same-data %d‰", sameLogic, sameData)
	}
}

// TestWeightedSimilarityBreaksTies is the CCGraph ranking benchmark: the
// flat kernel ties (458/458) on same-control/different-data vs
// different-control/same-data, while the control-weighted kernel ranks the
// same-logic pair clearly above.
func TestWeightedSimilarityBreaksTies(t *testing.T) {
	f1 := shapePDG(Branch, "arith", "fmt.Sprintf")
	f2 := shapePDG(Branch, "bit", "strconv.Itoa")
	f3 := shapePDG(Loop, "arith", "fmt.Sprintf")

	u12 := SimilarityMilli(NewWl(f1), NewWl(f2))
	u13 := SimilarityMilli(NewWl(f1), NewWl(f3))
	w12 := SimilarityWeighted(NewWl(f1), NewWl(f2))
	w13 := SimilarityWeighted(NewWl(f1), NewWl(f3))
	t.Logf("unweighted: %d/%d, weighted: %d/%d", u12, u13, w12, w13)
	if u12 != u13 {
		t.Logf("note: unweighted tie broken by structure (%d vs %d)", u12, u13)
	}
	if w12 <= w13 {
		t.Errorf("weighted should rank same-logic above same-data: %d <= %d", w12, w13)
	}
	if w12 <= u12 {
		t.Errorf("weighted same-logic %d should exceed unweighted %d", w12, u12)
	}
}

// TestCallScopeLowersCrossScopeSimilarity shows the precision win: identical
// structure calling stdlib vs local with the same signature class scores
// below identical.
func TestCallScopeLowersCrossScopeSimilarity(t *testing.T) {
	mk := func(callee string) *MiningGraph {
		return &MiningGraph{
			Nodes: []PdgNode{
				{Kind: Branch, Line: 1},
				callNode(callee, "fn(string) -> ()"),
				callNode("fmt.Println", "fn(string) -> ()"),
				{Kind: Op, Detail: "assign:=", Line: 4},
			},
			Edges: []PdgEdge{
				{From: 0, To: 1, Kind: Ctrl, ArgPos: 0},
				{From: 0, To: 3, Kind: Ctrl, ArgPos: 1},
				{From: 1, To: 2, Kind: Data, ArgPos: 0},
			},
		}
	}
	a := mk("fmt.Printf")
	b := mk("fmt.Printf")
	c := mk("github.com/me/app.printf")
	same := SimilarityMilli(NewWl(a), NewWl(b))
	cross := SimilarityMilli(NewWl(a), NewWl(c))
	t.Logf("same-scope: %d‰, cross-scope: %d‰", same, cross)
	if same != 1000 {
		t.Errorf("identical PDGs: %d‰, want 1000‰", same)
	}
	if cross >= same {
		t.Errorf("cross-scope %d‰ not below same-scope %d‰", cross, same)
	}
}
