package gopatterns

import (
	"context"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func loadShapes(t *testing.T) []FuncPdg {
	t.Helper()
	ex, err := Extract(context.Background(), "testdata/shapes", []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	return ex.Funcs
}

func findPdg(t *testing.T, pdgs []FuncPdg, name string) FuncPdg {
	t.Helper()
	for _, f := range pdgs {
		if strings.HasSuffix(f.Name, "."+name) {
			return f
		}
	}
	t.Fatalf("function %s not extracted", name)
	return FuncPdg{}
}

func countKind(pdg *patterns.Pdg, kind patterns.NodeKind) int {
	n := 0
	for _, node := range pdg.Nodes {
		if node.Kind == kind {
			n++
		}
	}
	return n
}

func hasEdge(pdg *patterns.Pdg, fromKind, toKind patterns.NodeKind, edgeKind patterns.EdgeKind) bool {
	index := map[int]patterns.NodeKind{}
	for i, n := range pdg.Nodes {
		index[i] = n.Kind
	}
	for _, e := range pdg.Edges {
		if index[e.From] == fromKind && index[e.To] == toKind && e.Kind == edgeKind {
			return true
		}
	}
	return false
}

func TestStraightLineStructure(t *testing.T) {
	pdg := findPdg(t, loadShapes(t), "CreateUser").Pdg
	// 3 params, 2 branches (validations), 2 errors.New calls, 1 Let
	// (destructured Exec), 1 Try, 3 returns.
	if got := countKind(&pdg, patterns.Param); got != 3 {
		t.Errorf("params = %d, want 3", got)
	}
	if got := countKind(&pdg, patterns.Branch); got != 2 {
		t.Errorf("branches = %d, want 2", got)
	}
	if got := countKind(&pdg, patterns.Try); got != 1 {
		t.Errorf("try = %d, want 1", got)
	}
	if got := countKind(&pdg, patterns.Let); got != 1 {
		t.Errorf("let = %d, want 1", got)
	}
	// The Exec call carries its stable callee id and signature class.
	found := false
	for _, n := range pdg.Nodes {
		if n.Kind == patterns.Call && n.CalleeID == "example.com/shapes.DB.Exec" {
			found = true
			if n.SigClass != "fn(*_, string, ...string) -> (int64, error)" {
				t.Errorf("Exec sig = %q", n.SigClass)
			}
		}
	}
	if !found {
		t.Error("DB.Exec call missing callee id")
	}
	// errors.New resolves through the package qualifier.
	found = false
	for _, n := range pdg.Nodes {
		if n.Kind == patterns.Call && n.CalleeID == "errors.New" {
			found = true
		}
	}
	if !found {
		t.Error("errors.New call missing callee id")
	}
	// Branch controls its arm; Try controls its return.
	if !hasEdge(&pdg, patterns.Branch, patterns.Return, patterns.Ctrl) {
		t.Error("missing Branch -> Return ctrl edge")
	}
	if !hasEdge(&pdg, patterns.Try, patterns.Return, patterns.Ctrl) {
		t.Error("missing Try -> Return ctrl edge")
	}
}

func TestLoopCanonicalization(t *testing.T) {
	pdgs := loadShapes(t)
	forLoop := findPdg(t, pdgs, "SumFor").Pdg
	rangeLoop := findPdg(t, pdgs, "SumRange").Pdg
	// Both spellings become Iterate.
	if got := countKind(&forLoop, patterns.Iterate); got != 1 {
		t.Errorf("SumFor iterate = %d, want 1", got)
	}
	if got := countKind(&rangeLoop, patterns.Iterate); got != 1 {
		t.Errorf("SumRange iterate = %d, want 1", got)
	}
	if got := countKind(&forLoop, patterns.Loop); got != 0 {
		t.Errorf("SumFor loop = %d, want 0", got)
	}
	// Condition loop stays a Loop.
	whileLoop := findPdg(t, pdgs, "SumWhile").Pdg
	if got := countKind(&whileLoop, patterns.Loop); got != 1 {
		t.Errorf("SumWhile loop = %d, want 1", got)
	}
	if got := countKind(&whileLoop, patterns.Iterate); got != 0 {
		t.Errorf("SumWhile iterate = %d, want 0", got)
	}
}

func TestDeferAndGoNodes(t *testing.T) {
	pdgs := loadShapes(t)
	deferDemo := findPdg(t, pdgs, "DeferDemo").Pdg
	if got := countKind(&deferDemo, patterns.Defer); got != 1 {
		t.Errorf("defer nodes = %d, want 1", got)
	}
	if !hasEdge(&deferDemo, patterns.Call, patterns.Defer, patterns.Data) {
		t.Error("missing Call -> Defer data edge")
	}
	goDemo := findPdg(t, pdgs, "GoDemo").Pdg
	if got := countKind(&goDemo, patterns.Go); got != 1 {
		t.Errorf("go nodes = %d, want 1", got)
	}
	if !hasEdge(&goDemo, patterns.Call, patterns.Go, patterns.Data) {
		t.Error("missing Call -> Go data edge")
	}
}

func TestClosureOpaque(t *testing.T) {
	pdg := findPdg(t, loadShapes(t), "ClosureDemo").Pdg
	if got := countKind(&pdg, patterns.Closure); got != 1 {
		t.Errorf("closure nodes = %d, want 1", got)
	}
}

func TestMatchNodes(t *testing.T) {
	pdgs := loadShapes(t)
	switchDemo := findPdg(t, pdgs, "SwitchDemo").Pdg
	if got := countKind(&switchDemo, patterns.Match); got != 1 {
		t.Errorf("switch match nodes = %d, want 1", got)
	}
	typeSwitch := findPdg(t, pdgs, "TypeSwitchDemo").Pdg
	if got := countKind(&typeSwitch, patterns.Match); got != 1 {
		t.Errorf("type switch match nodes = %d, want 1", got)
	}
}

func TestMultiReturn(t *testing.T) {
	pdg := findPdg(t, loadShapes(t), "MultiReturn").Pdg
	// One return per return statement (2).
	if got := countKind(&pdg, patterns.Return); got != 2 {
		t.Errorf("return nodes = %d, want 2", got)
	}
}

func TestMethodReceiverAtArgZero(t *testing.T) {
	pdg := findPdg(t, loadShapes(t), "MethodDemo").Pdg
	callIdx := -1
	for i, n := range pdg.Nodes {
		if n.Kind == patterns.Call && n.CalleeID == "example.com/shapes.DB.Exec" {
			callIdx = i
		}
	}
	if callIdx < 0 {
		t.Fatal("DB.Exec call not found")
	}
	for _, e := range pdg.Edges {
		if e.To == callIdx && e.Kind == patterns.Data && e.ArgPos == 0 {
			if recv := pdg.Nodes[e.From]; recv.Kind != patterns.Param {
				t.Errorf("receiver at arg 0 is %s, want param", recv.Kind)
			}
			return
		}
	}
	t.Error("no arg-0 edge into DB.Exec call")
}
