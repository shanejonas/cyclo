package patterns

import "testing"

// testTaintPdg builds a PDG from nodes and edges for taint tests.
func testTaintPdg(nodes []PdgNode, edges []PdgEdge) *MiningGraph {
	return &MiningGraph{Nodes: nodes, Edges: edges}
}

func testTaintFacts(pdg *MiningGraph) []*MiningFacts {
	return []*MiningFacts{{ID: "pkg.Handler", Name: "Handler", Path: "a.go", Pdg: pdg}}
}

func TestTaintQueryToExecIsFlagged(t *testing.T) {
	// x := r.URL.Query().Get("q"); db.Exec("SELECT ... " + x)
	pdg := testTaintPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "(*url.Values).Get", Line: 10}, // 0: source
			{Kind: Let, Line: 10},                              // 1: x :=
			{Kind: Op, Detail: "add:string", Line: 11},         // 2: "..." + x
			{Kind: Call, CalleeID: "(*sql.DB).Exec", Line: 11}, // 3: sink
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 1},
			{From: 2, To: 3, Kind: Data, ArgPos: 1}, // query arg
		},
	)
	flows := FindTaintFlows(testTaintFacts(pdg))
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	fl := flows[0]
	if fl.Risk != "SQL injection" {
		t.Errorf("unexpected risk: %q", fl.Risk)
	}
	if fl.SourceLine != 10 || fl.SinkLine != 11 {
		t.Errorf("unexpected lines: %+v", fl)
	}
}

func TestTaintSanitizedNotFlagged(t *testing.T) {
	// x := r.URL.Query().Get("q"); safe := template.HTMLEscapeString(x)
	// tmpl.Execute(w, safe)
	pdg := testTaintPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "(*url.Values).Get", Line: 10}, // 0: source
			{Kind: Let, Line: 10}, // 1: x :=
			{Kind: Call, CalleeID: "template.HTMLEscapeString", Line: 11}, // 2: sanitizer
			{Kind: Let, Line: 11}, // 3: safe :=
			{Kind: Call, CalleeID: "(*template.Template).Execute", Line: 12}, // 4: sink
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 0},
			{From: 2, To: 3, Kind: Data, ArgPos: 0},
			{From: 3, To: 4, Kind: Data, ArgPos: 2}, // data arg
		},
	)
	if flows := FindTaintFlows(testTaintFacts(pdg)); len(flows) != 0 {
		t.Errorf("expected no flows (sanitized), got %d", len(flows))
	}
}

func TestTaintCleanDataNotFlagged(t *testing.T) {
	// db.Exec("SELECT * FROM users") — no tainted input.
	pdg := testTaintPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "(*sql.DB).Exec", Line: 10}, // 0: sink, clean
		},
		nil,
	)
	if flows := FindTaintFlows(testTaintFacts(pdg)); len(flows) != 0 {
		t.Errorf("expected no flows (clean), got %d", len(flows))
	}
}

func TestTaintParameterizedQueryNotFlagged(t *testing.T) {
	// x := r.URL.Query().Get("id"); db.Exec("... WHERE id = ?", x)
	// Tainted data in ArgPos 2 (parameter) is the safe pattern — only
	// ArgPos 1 (query string) is dangerous.
	pdg := testTaintPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "(*url.Values).Get", Line: 10}, // 0: source
			{Kind: Let, Line: 10},                              // 1: x :=
			{Kind: Call, CalleeID: "(*sql.DB).Exec", Line: 11}, // 2: sink
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 2}, // parameter, not query
		},
	)
	if flows := FindTaintFlows(testTaintFacts(pdg)); len(flows) != 0 {
		t.Errorf("expected no flows (parameterized), got %d", len(flows))
	}
}

func TestTaintExecCommandFlagged(t *testing.T) {
	// name := os.Getenv("TOOL"); exec.Command(name)
	pdg := testTaintPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "os.Getenv", Line: 10},    // 0: source
			{Kind: Let, Line: 10},                            // 1: name :=
			{Kind: Call, CalleeID: "exec.Command", Line: 11}, // 2: sink
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 0},
		},
	)
	flows := FindTaintFlows(testTaintFacts(pdg))
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].Risk != "command injection" {
		t.Errorf("unexpected risk: %q", flows[0].Risk)
	}
}

func TestTaintCandidatesFormat(t *testing.T) {
	pdg := testTaintPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "os.Getenv", Line: 10},
			{Kind: Let, Line: 10},
			{Kind: Call, CalleeID: "os.Open", Line: 11},
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 0},
		},
	)
	facts := testTaintFacts(pdg)
	flows := FindTaintFlows(facts)
	cands := TaintFlowCandidates(flows, facts)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != TaintFlowKind {
		t.Errorf("unexpected kind: %q", c.Kind)
	}
	if len(c.Sites) != 2 {
		t.Errorf("expected 2 sites (source + sink), got %d", len(c.Sites))
	}
	if c.FixSpec != nil {
		t.Error("detection-only: FixSpec should be nil")
	}
}
