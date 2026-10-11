package patterns

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func pairFixture() map[string]*MiningGraph {
	graphs := map[string]*MiningGraph{"b": ccTestPdg(8, 100), "a": ccTestPdg(8, 1)}
	for _, graph := range graphs {
		graph.Paper = &PaperCharacteristics{Counts: [9]float64{0, 8, 0, 0, 0, 0, 7, 0, 0}, ReferenceKnown: true}
	}
	return graphs
}

func TestClonePairProfileIdenticalGraphs(t *testing.T) {
	var evidence []ClonePairEvidence
	err := EvaluateClonePairs(CycloPairProfile, pairFixture(), map[string]string{"a": "aaaa", "b": "zzzz"}, func(e ClonePairEvidence) error { evidence = append(evidence, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := []ClonePairEvidence{{Profile: CycloPairProfile, Left: "a", Right: "b", Numerical: 1, Name: 0, Admitted: true, WLKnown: true, Accepted: true, WLMilli: 1000}}
	if len(evidence) == 1 && math.Abs(evidence[0].Numerical-1) < 1e-12 {
		evidence[0].Numerical = 1
	}
	if !reflect.DeepEqual(evidence, want) {
		t.Fatalf("got %+v; want %+v", evidence, want)
	}
}

func TestClonePairProfileGateRejection(t *testing.T) {
	graphs := pairFixture()
	graphs["a"].Paper.Counts = [9]float64{8}
	graphs["b"].Paper.Counts = [9]float64{0, 8}
	var got ClonePairEvidence
	err := EvaluateClonePairs(CycloPairProfile, graphs, map[string]string{"a": "aaaa", "b": "zzzz"}, func(e ClonePairEvidence) error { got = e; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Admitted || got.WLKnown || got.Accepted || got.Numerical != 0 || got.Name != 0 {
		t.Fatalf("unexpected gate evidence: %+v", got)
	}
}

func TestClonePairProfileUnknownFacts(t *testing.T) {
	graphs := pairFixture()
	graphs["a"].Paper.ReferenceKnown = false
	called := false
	err := EvaluateClonePairs(CycloPairProfile, graphs, map[string]string{"a": "a", "b": "b"}, func(ClonePairEvidence) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("unknown required facts must fail before output")
	}
}

func TestClonePairProfileUnavailable(t *testing.T) {
	for _, profile := range []CloneProfile{CompatBaseProfile, "", "unknown"} {
		if err := EvaluateClonePairs(profile, nil, nil, func(ClonePairEvidence) error { return nil }); err == nil {
			t.Fatalf("profile %q must not execute", profile)
		}
	}
}

func TestClonePairProfileSinkError(t *testing.T) {
	stop := errors.New("stop")
	err := EvaluateClonePairs(CycloPairProfile, pairFixture(), map[string]string{"a": "a", "b": "b"}, func(ClonePairEvidence) error { return stop })
	if !errors.Is(err, stop) {
		t.Fatalf("sink error lost: %v", err)
	}
}

func TestCloneDefaultIncludesShortSourceFunctions(t *testing.T) {
	facts := []*MiningFacts{{ID: "short", Line: 10, EndLine: 14, Pdg: ccTestPdg(8, 1)}}
	graphs, _, _ := ccGraphInputs(facts)
	if graphs["short"] == nil {
		t.Fatal("source-size policy must not silently change the default")
	}
}

func TestCloneDefaultKeepsASTBypass(t *testing.T) {
	graphs := pairFixture()
	graphs["a"].Paper.Counts = [9]float64{8}
	graphs["b"].Paper.Counts = [9]float64{0, 8}
	names := map[string]string{"a": "aaaa", "b": "zzzz"}
	if groups := CCGraphClones(graphs, names); len(groups) != 0 {
		t.Fatalf("plain route unexpectedly accepts: %v", groups)
	}
	facts := []*MiningFacts{
		{ID: "a", Name: names["a"], Pdg: graphs["a"], AstTypes: map[string]int{"CallExpr": 8}},
		{ID: "b", Name: names["b"], Pdg: graphs["b"], AstTypes: map[string]int{"CallExpr": 8}},
	}
	if got := defaultCloneGroups(facts); !reflect.DeepEqual(got, [][]string{{"a", "b"}}) {
		t.Fatalf("report default loses AST bypass: %v", got)
	}
}

func TestClonePairProfileNameFallback(t *testing.T) {
	graphs := pairFixture()
	graphs["a"].Paper.Counts = [9]float64{8}
	graphs["b"].Paper.Counts = [9]float64{0, 8}
	var got ClonePairEvidence
	err := EvaluateClonePairs(CycloPairProfile, graphs, map[string]string{"a": "same", "b": "same"}, func(e ClonePairEvidence) error { got = e; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Numerical != 0 || got.Name != 1 || !got.Admitted || !got.Accepted {
		t.Fatalf("name fallback: %+v", got)
	}
}

func TestClonePairProfileWLThreshold(t *testing.T) {
	for _, size := range []int{8, 9} {
		graphs := pairFixture()
		graphs["a"] = ccTestPdg(10, 1)
		graphs["a"].Edges = nil
		graphs["b"] = ccTestPdg(size, 100)
		graphs["b"].Edges = nil
		graphs["a"].Paper = &PaperCharacteristics{Counts: [9]float64{0, 10}, ReferenceKnown: true}
		graphs["b"].Paper = &PaperCharacteristics{Counts: [9]float64{0, float64(size)}, ReferenceKnown: true}
		var got ClonePairEvidence
		err := EvaluateClonePairs(CycloPairProfile, graphs, map[string]string{"a": "a", "b": "b"}, func(e ClonePairEvidence) error { got = e; return nil })
		if err != nil {
			t.Fatal(err)
		}
		if got.WLMilli != uint32(size*100) || got.Accepted != (size == 9) {
			t.Fatalf("size %d: %+v", size, got)
		}
	}
}

func TestClonePairProfileMissingSinkAndName(t *testing.T) {
	if err := EvaluateClonePairs(CycloPairProfile, pairFixture(), nil, nil); err == nil {
		t.Fatal("missing sink succeeds")
	}
	called := false
	err := EvaluateClonePairs(CycloPairProfile, pairFixture(), map[string]string{"a": "a"}, func(ClonePairEvidence) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("missing name must fail before output")
	}
}
