package patterns

import (
	"reflect"
	"testing"
)

func dispatchNode(kind NodeKind) PdgNode { return PdgNode{Kind: kind, Line: 42} }

func dispatchData(from, to, argPos int) PdgEdge {
	return PdgEdge{From: from, To: to, Kind: Data, ArgPos: argPos}
}

func dispatchCtrl(from, to, argPos int) PdgEdge {
	return PdgEdge{From: from, To: to, Kind: Ctrl, ArgPos: argPos}
}

func dispatchMethod(id, sigClass string) PdgNode {
	return PdgNode{Kind: Call, CalleeID: id, SigClass: sigClass, Line: 43}
}

// typeSwitch builds the PDG of:
//
//	switch v := x.(type) {
//	case Dog:
//		v.Bark() // arm 0
//	case Cat:
//		v.<second>() // arm 1, sig class secondSig
//	}
//
// The Match node (1) is the producer of the case-bound value: each arm's
// call receives it as its receiver (data edge at arg position 0).
func typeSwitch(secondID, secondSig string) *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			dispatchNode(Param),
			dispatchNode(Match),
			dispatchMethod("example.com/p.Dog.Bark", "fn() -> string"),
			dispatchMethod(secondID, secondSig),
			dispatchNode(Return),
		},
		Edges: []PdgEdge{
			dispatchData(0, 1, 0),
			dispatchCtrl(1, 2, 0),
			dispatchCtrl(1, 3, 1),
			dispatchData(1, 2, 0),
			dispatchData(1, 3, 0),
			dispatchData(1, 4, 0),
		},
	}
}

func TestFindDispatchAnalogousMethodsAreADispatch(t *testing.T) {
	got := FindDispatch(typeSwitch("example.com/p.Cat.Meow", "fn() -> string"))
	want := []Dispatch{{
		Line:  42,
		Class: "fn() -> string",
		Arms: []Arm{
			{Index: 0, CalleeID: "example.com/p.Dog.Bark"},
			{Index: 1, CalleeID: "example.com/p.Cat.Meow"},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindDispatch() = %+v, want %+v", got, want)
	}
}

func TestFindDispatchSameCalleeInEveryArmIsNot(t *testing.T) {
	pdg := typeSwitch("example.com/p.Dog.Bark", "fn() -> string")
	if got := FindDispatch(pdg); len(got) != 0 {
		t.Errorf("FindDispatch() = %+v, want no dispatches", got)
	}
}

func TestFindDispatchUnrelatedCallsAreNot(t *testing.T) {
	// The arms call different-shaped methods; no signature class has two
	// distinct callees.
	pdg := typeSwitch("example.com/p.Cat.Purr", "fn(int) -> string")
	if got := FindDispatch(pdg); len(got) != 0 {
		t.Errorf("FindDispatch() = %+v, want no dispatches", got)
	}
}

func TestFindDispatchCallNotOnBoundValueIsNot(t *testing.T) {
	// Arm 1's call is control-dependent on the switch but does not take the
	// bound value as its receiver (no data edge from the Match node at arg
	// position 0): only one arm calls a method on the bound variable.
	pdg := typeSwitch("example.com/p.Cat.Meow", "fn() -> string")
	var edges []PdgEdge
	for _, e := range pdg.Edges {
		if e == (PdgEdge{From: 1, To: 3, Kind: Data, ArgPos: 0}) {
			continue
		}
		edges = append(edges, e)
	}
	pdg.Edges = edges
	if got := FindDispatch(pdg); len(got) != 0 {
		t.Errorf("FindDispatch() = %+v, want no dispatches", got)
	}
}

func TestFindDispatchSingleCaseSwitchIsNot(t *testing.T) {
	// One arm cannot yield two distinct callees.
	pdg := &Pdg{
		Nodes: []PdgNode{
			dispatchNode(Param),
			dispatchNode(Match),
			dispatchMethod("example.com/p.Dog.Bark", "fn() -> string"),
		},
		Edges: []PdgEdge{
			dispatchData(0, 1, 0),
			dispatchCtrl(1, 2, 0),
			dispatchData(1, 2, 0),
		},
	}
	if got := FindDispatch(pdg); len(got) != 0 {
		t.Errorf("FindDispatch() = %+v, want no dispatches", got)
	}
}

func TestFindDispatchDefaultOnlyIsNot(t *testing.T) {
	// A switch with only a default clause: the default is just another arm
	// index, so a single default arm cannot form a dispatch.
	pdg := &Pdg{
		Nodes: []PdgNode{
			dispatchNode(Param),
			dispatchNode(Match),
			dispatchMethod("example.com/p.Dog.Bark", "fn() -> string"),
		},
		Edges: []PdgEdge{
			dispatchData(0, 1, 0),
			dispatchCtrl(1, 2, 0), // arm 0 is the default clause
			dispatchData(1, 2, 0),
		},
	}
	if got := FindDispatch(pdg); len(got) != 0 {
		t.Errorf("FindDispatch() = %+v, want no dispatches", got)
	}
}

func TestFindDispatchNilPdg(t *testing.T) {
	if got := FindDispatch(nil); len(got) != 0 {
		t.Errorf("FindDispatch(nil) = %+v, want no dispatches", got)
	}
}
