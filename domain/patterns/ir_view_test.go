package patterns

import (
	"reflect"
	"testing"

	"github.com/shanejonas/cyclo/domain/pdg"
)

func TestMiningViewRemapsNativeNodesAndKeepsParallelEdges(t *testing.T) {
	pool := pdg.NewBuilder()
	param := pool.MatchLabel(pdg.MatchLabel{Kind: pool.Text(string(Param)), TypeClass: pool.Text("int")})
	call := pool.MatchLabel(pdg.MatchLabel{Kind: pool.Text(string(Call)), Callee: pool.Text("example.f"), Effects: EffectIO})
	g := &pdg.Graph{Tables: pool.Tables, Nodes: []pdg.Node{{MatchLabel: param, Line: 3}, {Category: pdg.FormalOutput}, {MatchLabel: call, Line: 5}}, Edges: []pdg.Edge{
		{Source: 1, Target: 3, Kind: pdg.Data, Position: 1},
		{Source: 1, Target: 3, Kind: pdg.Data, Position: 2},
		{Source: 3, Target: 3, Kind: pdg.ControlEdge, Position: 1},
		{Source: 2, Target: 3, Kind: pdg.Data},
		{Source: 1, Target: 3, Kind: pdg.Execution},
	}}
	view := MiningView(g)
	if len(view.Nodes) != 2 || len(view.Edges) != 4 {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.Edges[0].To != 1 || view.Edges[1].ArgPos != 1 || view.Edges[2].From != view.Edges[2].To {
		t.Fatal("reference or parallel-edge loss")
	}
	if view.Nodes[1].Effects != EffectIO || view.Nodes[1].CalleeID != "example.f" || view.Nodes[1].Line != 5 {
		t.Fatal("matching metadata loss")
	}
	view.Nodes[0].TyClass = "changed"
	view.Edges[0].To = 0
	if pool.Tables.Text(pool.Tables.MatchLabels[param-1].TypeClass) != "int" || g.Edges[0].Target != 3 {
		t.Fatal("view changes canonical storage")
	}
}

func TestRunUsesCanonicalIRAndPreservesFindingMetadata(t *testing.T) {
	facts := []*FuncFacts{
		{ID: "example.Dog.Bark", Name: "Bark", SelfTy: "Dog", SigKey: "fn(int)", Path: "dog.go", Line: 3, GuardClauses: []GuardClauseHit{{Line: 4, BodyStmts: 3}}},
		{ID: "example.Cat.Meow", Name: "Meow", SelfTy: "Cat", SigKey: "fn(int)", Path: "cat.go", Line: 8, Params: []ParamInfo{{Name: "age", Type: "int"}}},
	}
	pool := pdg.NewBuilder()
	label := pool.MatchLabel(pdg.MatchLabel{Kind: pool.Text(string(Param)), TypeClass: pool.Text("int")})
	for _, f := range facts {
		f.Pdg = &pdg.Graph{Tables: pool.Tables, Nodes: []pdg.Node{{MatchLabel: label}}}
	}
	views := []*MiningFacts{miningFacts(facts[0]), miningFacts(facts[1])}
	want := RunMining(views, Options{})
	got := Run(facts, Options{})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical Run differs: got=%+v want=%+v", got, want)
	}
	if len(got.SignatureGroups) == 0 {
		t.Fatal("fixture does not exercise signature mining")
	}
}
