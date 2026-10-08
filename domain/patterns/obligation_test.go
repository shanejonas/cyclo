package patterns

import "testing"

// pdg builds a PDG from nodes and edges for obligation tests.
func testObligationPdg(nodes []PdgNode, edges []PdgEdge) *Pdg {
	return &Pdg{Nodes: nodes, Edges: edges}
}

func TestObligationDeferCloseNoViolation(t *testing.T) {
	// f, _ := os.Open(name); defer f.Close()
	pdg := testObligationPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "os.Open", Line: 10},          // 0
			{Kind: Let, Line: 10},                                // 1: f, err :=
			{Kind: Call, CalleeID: "(*os.File).Close", Line: 12}, // 2: f.Close()
			{Kind: Defer, Line: 12},                              // 3: defer
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 1, To: 2, Kind: Data, ArgPos: 0}, // receiver f
			{From: 2, To: 3, Kind: Data, ArgPos: 0}, // defer wraps call
		},
	)
	facts := []*FuncFacts{{ID: "pkg.F", Pdg: pdg}}
	if v := FindObligationViolations(facts); len(v) != 0 {
		t.Errorf("expected no violations, got %v", v)
	}
}

func TestObligationMissingDeferIsViolation(t *testing.T) {
	// f, _ := os.Open(name) with no defer
	pdg := testObligationPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "os.Open", Line: 10}, // 0
			{Kind: Let, Line: 10},                       // 1
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
		},
	)
	facts := []*FuncFacts{{ID: "pkg.F", Pdg: pdg}}
	violations := FindObligationViolations(facts)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	v := violations[0]
	if v.AcquireCall != "os.Open" || v.Resource != "file" {
		t.Errorf("unexpected violation: %+v", v)
	}
	if v.FuncID != "pkg.F" || v.Line != 10 {
		t.Errorf("unexpected location: %+v", v)
	}
}

func TestObligationNonResourceIgnored(t *testing.T) {
	// fmt.Println("hi") is not a resource acquisition.
	pdg := testObligationPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "fmt.Println", Line: 10}, // 0
		},
		nil,
	)
	facts := []*FuncFacts{{ID: "pkg.F", Pdg: pdg}}
	if v := FindObligationViolations(facts); len(v) != 0 {
		t.Errorf("expected no violations, got %v", v)
	}
}

func TestObligationMutexDeferUnlockNoViolation(t *testing.T) {
	// mu.Lock(); defer mu.Unlock() where mu is a parameter.
	pdg := testObligationPdg(
		[]PdgNode{
			{Kind: Param, Line: 5},                                  // 0: mu
			{Kind: Call, CalleeID: "(*sync.Mutex).Lock", Line: 5},   // 1
			{Kind: Call, CalleeID: "(*sync.Mutex).Unlock", Line: 6}, // 2
			{Kind: Defer, Line: 6},                                  // 3
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0}, // receiver mu
			{From: 0, To: 2, Kind: Data, ArgPos: 0}, // receiver mu
			{From: 2, To: 3, Kind: Data, ArgPos: 0}, // defer wraps call
		},
	)
	facts := []*FuncFacts{{ID: "pkg.F", Pdg: pdg}}
	if v := FindObligationViolations(facts); len(v) != 0 {
		t.Errorf("expected no violations, got %v", v)
	}
}

func TestObligationMutexMissingUnlockIsViolation(t *testing.T) {
	// mu.Lock() with no deferred Unlock.
	pdg := testObligationPdg(
		[]PdgNode{
			{Kind: Param, Line: 5},                                // 0: mu
			{Kind: Call, CalleeID: "(*sync.Mutex).Lock", Line: 5}, // 1
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
		},
	)
	facts := []*FuncFacts{{ID: "pkg.F", Pdg: pdg}}
	violations := FindObligationViolations(facts)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Resource != "mutex" {
		t.Errorf("unexpected resource: %+v", violations[0])
	}
}

func TestObligationUnrelatedDeferNoMatch(t *testing.T) {
	// f, _ := os.Open(name); defer fmt.Println("done") — the defer does
	// not release the file, so it is still a violation.
	pdg := testObligationPdg(
		[]PdgNode{
			{Kind: Call, CalleeID: "os.Open", Line: 10},     // 0
			{Kind: Let, Line: 10},                           // 1
			{Kind: Call, CalleeID: "fmt.Println", Line: 11}, // 2
			{Kind: Defer, Line: 11},                         // 3
		},
		[]PdgEdge{
			{From: 0, To: 1, Kind: Data, ArgPos: 0},
			{From: 2, To: 3, Kind: Data, ArgPos: 0},
		},
	)
	facts := []*FuncFacts{{ID: "pkg.F", Pdg: pdg}}
	if v := FindObligationViolations(facts); len(v) != 1 {
		t.Errorf("expected 1 violation, got %v", v)
	}
}
