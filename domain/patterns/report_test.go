package patterns

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func intPtr(n int) *int { return &n }

func testFact(id, name, sigKey, selfTy string) *FuncFacts {
	return &FuncFacts{
		ID:     id,
		Name:   name,
		Path:   "src/lib.go",
		Line:   7,
		SigKey: sigKey,
		SelfTy: selfTy,
	}
}

func testGroup(key string, score uint32) SigGroup {
	return SigGroup{
		Key:              key,
		Members:          []Member{},
		Traited:          []Member{},
		SelfTypes:        2,
		SameName:         false,
		SpecificityMilli: score,
		ScoreMilli:       score,
	}
}

func TestBuildMinScoreAndTop(t *testing.T) {
	groups := []SigGroup{testGroup("a", 900), testGroup("b", 500), testGroup("c", 100)}
	if got := Build(groups, 400, nil); len(got.SignatureGroups) != 2 {
		t.Fatalf("min_score 400: got %d groups, want 2", len(got.SignatureGroups))
	}
	if got := Build(groups, 0, intPtr(1)); len(got.SignatureGroups) != 1 || got.SignatureGroups[0].Key != "a" {
		t.Fatalf("top 1: got %+v, want [a]", got.SignatureGroups)
	}
	if got := Build(groups, 0, nil); len(got.Candidates) != 0 || len(got.Suppressed) != 0 {
		t.Fatalf("Build must start candidates/suppressed empty: %+v", got)
	}
}

func TestTextListsHeaderAndSites(t *testing.T) {
	g := testGroup("fn(&Self) -> String", 1500)
	g.Members = append(g.Members, Member{
		Path: "src/lib.rs", Line: 7, ID: "k::Dog::bark", Name: "Dog::bark", SelfTy: "k::Dog",
	})
	out := Text(&PatternsReport{SignatureGroups: []SigGroup{g}})
	if !strings.HasPrefix(out, "1 signature groups") {
		t.Fatalf("text should start with group header, got:\n%s", out)
	}
	for _, want := range []string{"score 1.50", "src/lib.rs:7  Dog::bark", "0 candidates"} {
		if !strings.Contains(out, want) {
			t.Errorf("text missing %q:\n%s", want, out)
		}
	}
}

func TestTextRendersCandidateAndSuppressed(t *testing.T) {
	report := &PatternsReport{
		Candidates: []Candidate{{
			Kind:             CandidateKind("trait_method"),
			ScoreMilli:       1500,
			Breakdown:        Breakdown{Support: 2, LiftMilli: 0, Holes: 1, CoverageMilli: 1000},
			Observation:      "M0 = Dog::bark | Cat::meow",
			Inference:        "introduce a trait",
			PossibleRefactor: "fn method(&self) -> String",
			Sites:            []Site{{Path: "src/emit.go", Line: 10, ID: "pkg.emitDog", Name: "emitDog"}},
			Definitions:      []Site{{Path: "src/dog.go", Line: 3, ID: "pkg.Dog.Bark", Name: "Dog.Bark"}},
			CounterEvidence:  []string{"Cat::meow has extra logging"},
		}},
		Suppressed: []Suppressed{{
			Reason: "already abstracted by trait Speaker",
			Sites:  []Site{{Path: "src/dog.go", Line: 3, ID: "pkg.Dog.Speak", Name: "Dog.Speak"}},
		}},
	}
	out := Text(report)
	for _, want := range []string{
		"#1 trait_method  score 1.50  [support 2, lift 0.00, holes 1, coverage 1.00]",
		"  observation: M0 = Dog::bark | Cat::meow",
		"  inference: introduce a trait",
		"  possible refactor: fn method(&self) -> String",
		"  sites:",
		"    src/emit.go:10  emitDog",
		"  definitions:",
		"    src/dog.go:3  Dog.Bark",
		"  counter evidence:",
		"    - Cat::meow has extra logging",
		"1 suppressed (already abstracted)",
		"  already abstracted by trait Speaker (src/dog.go:3  Dog.Speak)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text missing %q:\n%s", want, out)
		}
	}
}

func TestSuppressedListedSeparately(t *testing.T) {
	// Suppressed entries are never filtered by min_score: Run passes
	// mined.Suppressed through untouched, and rendering always shows them.
	report := &PatternsReport{
		Suppressed: []Suppressed{{
			Reason: "already abstracted",
			Sites:  []Site{{Path: "src/a.go", Line: 1, ID: "pkg.A", Name: "A"}},
		}},
	}
	out := Text(report)
	if !strings.Contains(out, "1 suppressed (already abstracted)") {
		t.Fatalf("suppressed section missing:\n%s", out)
	}
	js, err := JSON(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(js), &decoded); err != nil {
		t.Fatal(err)
	}
	sup, ok := decoded["suppressed"].([]any)
	if !ok || len(sup) != 1 {
		t.Fatalf("JSON suppressed not a 1-list: %s", js)
	}
}

func TestJSONRendersSnakeCaseKeys(t *testing.T) {
	report := &PatternsReport{
		SignatureGroups: []SigGroup{testGroup("fn() string", 800)},
	}
	js, err := JSON(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(js), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"signature_groups", "candidates", "suppressed"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("JSON missing key %q: %s", key, js)
		}
	}
}

func TestRunDedupesFacts(t *testing.T) {
	facts := []*FuncFacts{
		testFact("pkg.Dog.Bark", "Dog.Bark", "fn() string", "Dog"),
		testFact("pkg.Dog.Growl", "Dog.Growl", "fn() string", "Dog"),
	}
	doubled := append(append([]*FuncFacts{}, facts...), facts...)
	once := Run(facts, Options{})
	twice := Run(doubled, Options{})
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("doubled facts changed the report:\nonce: %+v\ntwice: %+v", once, twice)
	}
	seen := map[string]bool{}
	for _, g := range twice.SignatureGroups {
		for _, m := range g.Members {
			if seen[m.ID] {
				t.Fatalf("duplicate site id %q after merge", m.ID)
			}
			seen[m.ID] = true
		}
	}
}

func TestRunHideImplementsOracle(t *testing.T) {
	facts := []*FuncFacts{
		{ID: "pkg.Dog.Speak", Name: "Dog.Speak", Path: "src/dog.go", Line: 3,
			SigKey: "fn() string", SelfTy: "Dog", Implements: true},
		{ID: "pkg.Cat.Speak", Name: "Cat.Speak", Path: "src/cat.go", Line: 5,
			SigKey: "fn() string", SelfTy: "Cat"},
	}
	withHide := Run(facts, Options{HideImplements: []string{"Speak"}})
	if len(withHide.SignatureGroups) != 1 {
		t.Fatalf("hidden implements should form one group, got %d", len(withHide.SignatureGroups))
	}
	if len(withHide.SignatureGroups[0].Members) != 2 {
		t.Fatalf("group should hold both inherent methods, got %+v",
			withHide.SignatureGroups[0].Members)
	}
	withoutHide := Run(facts, Options{})
	if len(withoutHide.SignatureGroups) != 0 {
		t.Fatalf("unhidden implements should leave one inherent type, got %+v",
			withoutHide.SignatureGroups)
	}
	star := Run(facts, Options{HideImplements: []string{"*"}})
	if len(star.SignatureGroups) != 1 {
		t.Fatalf(`"*" should hide all implements links`)
	}
}

func TestMergeFactsDedupesByID(t *testing.T) {
	a := testFact("pkg.a", "a", "fn()", "A")
	b := testFact("pkg.b", "b", "fn()", "B")
	merged := mergeFacts([]*FuncFacts{b, a, b, nil})
	var ids []string
	for _, f := range merged {
		ids = append(ids, factID(f))
	}
	want := []string{"", "pkg.a", "pkg.b"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("mergeFacts ids = %v, want %v", ids, want)
	}
}

func TestHideImplementsCopiesFacts(t *testing.T) {
	f := &FuncFacts{ID: "pkg.Dog.Speak", Implements: true}
	out := hideImplements([]*FuncFacts{f}, []string{"Speak"})
	if out[0].Implements {
		t.Fatal("implements link should be hidden")
	}
	if !f.Implements {
		t.Fatal("hideImplements must not mutate the caller's facts")
	}
	if got := hideImplements([]*FuncFacts{f}, nil); got[0].Implements != true {
		t.Fatal("empty patterns must hide nothing")
	}
}

func TestKeepCandidatesMinScoreAndTop(t *testing.T) {
	cands := []Candidate{
		{Kind: CandidateKind("trait_method"), ScoreMilli: 900},
		{Kind: CandidateKind("parameterize"), ScoreMilli: 500},
		{Kind: CandidateKind("fn_param"), ScoreMilli: 100},
	}
	if got := keepCandidates(cands, 400, nil); len(got) != 2 {
		t.Fatalf("min_score 400: got %d candidates, want 2", len(got))
	}
	if got := keepCandidates(cands, 0, intPtr(1)); len(got) != 1 || got[0].ScoreMilli != 900 {
		t.Fatalf("top 1: got %+v, want the 900-score candidate", got)
	}
}

func TestDefaultParamsZeroValue(t *testing.T) {
	if got := defaultParams(Params{}); !reflect.DeepEqual(got, DefaultParams()) {
		t.Fatalf("zero Params should mean DefaultParams(), got %+v", got)
	}
	custom := Params{ThresholdMilli: 1001}
	if got := defaultParams(custom); !reflect.DeepEqual(got, custom) {
		t.Fatalf("explicit Params must pass through, got %+v", got)
	}
}
