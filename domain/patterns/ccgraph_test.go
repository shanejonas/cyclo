package patterns

import "testing"

func TestJaroWinklerIdentical(t *testing.T) {
	if got := jaroWinkler("getUser", "getUser"); got != 1.0 {
		t.Fatalf("identical strings should score 1.0, got %f", got)
	}
}

func TestJaroWinklerEmpty(t *testing.T) {
	if got := jaroWinkler("", ""); got != 1.0 {
		t.Fatalf("two empty strings should score 1.0, got %f", got)
	}
	if got := jaroWinkler("abc", ""); got != 0.0 {
		t.Fatalf("empty vs non-empty should score 0.0, got %f", got)
	}
}

func TestJaroWinklerSimilar(t *testing.T) {
	// "getUser" vs "getUsers" share a long prefix: should be high.
	got := jaroWinkler("getUser", "getUsers")
	if got < 0.9 {
		t.Fatalf("similar names should score >= 0.9, got %f", got)
	}
}

func TestJaroWinklerDifferent(t *testing.T) {
	got := jaroWinkler("abcdef", "xyz")
	if got > 0.5 {
		t.Fatalf("different strings should score low, got %f", got)
	}
}

func TestJaroWinklerThreshold(t *testing.T) {
	// Sanity: names that differ by a suffix pass the 0.5 Stage 2 bar,
	// unrelated names don't.
	if jaroWinkler("fetchData", "fetchDatum") < ccStage2NameThreshold {
		t.Fatal("fetchData/fetchDatum should pass Stage 2")
	}
	if jaroWinkler("fetchData", "renderWidget") >= ccStage2NameThreshold {
		t.Fatal("fetchData/renderWidget should fail Stage 2")
	}
}

// ccTestPdg builds a simple linear PDG with n nodes for CCGraph tests.
func ccTestPdg(n int, lineBase int) *MiningGraph {
	nodes := make([]PdgNode, n)
	for i := range nodes {
		nodes[i] = PdgNode{Kind: Op, Line: lineBase + i, Detail: "add:int"}
	}
	var edges []PdgEdge
	for i := 0; i+1 < n; i++ {
		edges = append(edges, PdgEdge{From: i, To: i + 1, Kind: Data})
	}
	return &MiningGraph{Nodes: nodes, Edges: edges}
}

func TestCCGraphClonesFindsSimilar(t *testing.T) {
	pdgs := map[string]*MiningGraph{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(20, 100),
		"f3": ccTestPdg(5, 200), // too small / different shape
	}
	names := map[string]string{
		"f1": "processData",
		"f2": "processDatum",
		"f3": "renderWidget",
	}
	groups := CCGraphClones(pdgs, names)
	if len(groups) != 1 {
		t.Fatalf("expected 1 clone group, got %d: %v", len(groups), groups)
	}
	got := map[string]bool{}
	for _, id := range groups[0] {
		got[id] = true
	}
	if !got["f1"] || !got["f2"] || got["f3"] {
		t.Fatalf("expected group {f1 f2}, got %v", groups[0])
	}
}

func TestCCGraphNumericalMatchBypassesNameFallback(t *testing.T) {
	// Algorithm 1 admits numerical matches before the name fallback.
	pdgs := map[string]*MiningGraph{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(20, 100),
	}
	names := map[string]string{
		"f1": "aaaaaaa",
		"f2": "zzzzzzz",
	}
	groups := CCGraphClones(pdgs, names)
	if len(groups) != 1 {
		t.Fatalf("numerical match incorrectly requires a name match: %v", groups)
	}
}

func TestQualifiedNumericalMatchBypassesNameFallback(t *testing.T) {
	pdgs := map[string]*MiningGraph{"a": ccTestPdg(20, 1), "b": ccTestPdg(20, 100)}
	names := map[string]string{
		"a": "github.com/example/project/pkg.Widget.aaaaaaa",
		"b": "github.com/example/project/pkg.Widget.zzzzzzz",
	}
	if got := CCGraphClones(pdgs, names); len(got) != 1 {
		t.Fatalf("numerical match lost to different names: %v", got)
	}
	matcher := newNameMatcher(ccNameRunes([]string{names["a"], names["b"]}))
	runes := ccNameRunes([]string{names["a"], names["b"]})
	if matcher.similar(runes[0], runes[1]) {
		t.Fatal("optimized matcher admits unrelated qualified names")
	}
}

func TestQualifiedNamesMatchAcrossPackagesAndReceivers(t *testing.T) {
	pdgs := map[string]*MiningGraph{"a": ccTestPdg(20, 1), "b": ccTestPdg(20, 100)}
	names := map[string]string{"a": "example.org/a.Reader.fetchData", "b": "other.net/b.Writer.fetchDatum"}
	if got := CCGraphClones(pdgs, names); len(got) != 1 {
		t.Fatalf("similar method names lost across packages: %v", got)
	}
}

func TestASTBypassStillAdmitsDifferentQualifiedNames(t *testing.T) {
	pdgs := map[string]*MiningGraph{"a": ccTestPdg(20, 1), "b": ccTestPdg(20, 100)}
	names := map[string]string{"a": "example.org/pkg.A.aaaaaaa", "b": "example.org/pkg.B.zzzzzzz"}
	shapes := map[string]map[string]int{"a": {"Return": 1, "Call": 2}, "b": {"Return": 1, "Call": 2}}
	if got := CCGraphClonesWithAST(pdgs, names, shapes); len(got) != 1 {
		t.Fatalf("AST bypass lost candidates: %v", got)
	}
}

func TestCCGraphClonesEmpty(t *testing.T) {
	if got := CCGraphClones(nil, nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	if got := CCGraphClones(map[string]*MiningGraph{"f1": ccTestPdg(20, 1)}, nil); len(got) != 0 {
		t.Fatalf("expected no groups for single function, got %v", got)
	}
}

func TestCCGraphClonesDeterministic(t *testing.T) {
	pdgs := map[string]*MiningGraph{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(20, 100),
	}
	names := map[string]string{"f1": "doThing", "f2": "doThings"}
	a := CCGraphClones(pdgs, names)
	b := CCGraphClones(pdgs, names)
	if len(a) != len(b) {
		t.Fatal("CCGraphClones is not deterministic")
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Fatal("CCGraphClones is not deterministic")
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Fatal("CCGraphClones is not deterministic")
			}
		}
	}
}
