package quality

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestHelperResolutionPreservesPolicyAndCallsiteEvidence(t *testing.T) {
	f := fact("project.caller", 1)
	f.Calls = []Call{{Callee: "project.helper", Local: true, Line: 2}, {Callee: "project.helper", Local: true, Line: 3}}
	f.Helpers = []Helper{{Name: "project.helper", Calls: []Call{{Callee: "fmt.Println", Line: 20}}}}
	before, _ := json.Marshal(f)
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if !result.Complete || result.UnclassifiedCalls != 0 || len(result.Effects) != 2 {
		t.Fatalf("resolution: %+v", result)
	}
	for index, effect := range result.Effects {
		if effect.Kind != IO || effect.Line != index+2 {
			t.Fatalf("callsite evidence: %+v", result.Effects)
		}
	}
	config := DefaultConfig()
	config.Prefixes = append(config.Prefixes, Prefix{Path: "project.helper", Kind: None})
	result = evaluate(t, f, config).Functions[0]
	if len(result.Effects) != 0 {
		t.Fatalf("explicit override ignored: %+v", result)
	}
	after, _ := json.Marshal(f)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("helper data mutated")
	}
}

func TestHelperResolutionKeepsMissingAndDynamicBodiesUnknown(t *testing.T) {
	for _, call := range []Call{
		{Callee: "project.missing", Local: true, Line: 2},
		{Callee: "project.helper", Local: true, Dynamic: true, Line: 2},
		{Callee: "project.helper", Line: 2},
	} {
		f := fact("caller", 1)
		f.Calls = []Call{call}
		f.Helpers = []Helper{{Name: "project.helper"}}
		result := evaluate(t, f, DefaultConfig()).Functions[0]
		if result.Complete || result.UnclassifiedCalls != 1 || len(result.Effects) != 1 || result.Effects[0].Kind != UnknownEffect {
			t.Fatalf("uncertainty erased: %+v", result)
		}
	}
}

func TestHelperSummaryLimitsStayUnknown(t *testing.T) {
	f := fact("caller", 1)
	f.Calls = []Call{{Callee: "helper0", Local: true, Line: 2}}
	for index := 0; index <= localDepthLimit; index++ {
		h := Helper{Name: fmt.Sprintf("helper%d", index)}
		if index < localDepthLimit {
			h.Calls = []Call{{Callee: fmt.Sprintf("helper%d", index+1), Local: true, Line: 3}}
		}
		f.Helpers = append(f.Helpers, h)
	}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if result.Complete || result.UnclassifiedCalls == 0 {
		t.Fatalf("depth limit pretends pure: %+v", result)
	}
	f.Helpers = []Helper{{Name: "helper0"}}
	for index := 0; index <= localEffectLimit; index++ {
		f.Helpers[0].Effects = append(f.Helpers[0].Effects, Effect{Kind: IO, Detail: "write", Line: 3})
	}
	result = evaluate(t, f, DefaultConfig()).Functions[0]
	if result.Complete || result.UnclassifiedCalls == 0 {
		t.Fatalf("effect limit pretends pure: %+v", result)
	}
	if len(result.Effects) != localEffectLimit || result.Effects[0].Kind != IO {
		t.Fatal("effect limit discarded established IO evidence")
	}
}

func TestHelperFactsRejectInvalidAndConflictingBodies(t *testing.T) {
	for _, helpers := range [][]Helper{
		{{Name: ""}},
		{{Name: "caller"}},
		{{Name: "helper"}, {Name: "helper"}},
		{{Name: "helper", Mutations: []Mutation{{Root: "x", Line: 2, Provenance: "invalid"}}}},
		{{Name: "helper", Effects: []Effect{{Kind: "invalid", Line: 2}}}},
		{{Name: "helper", Calls: []Call{{Callee: "callback", Line: 0}}}},
	} {
		f := fact("caller", 1)
		f.Helpers = helpers
		if _, err := Evaluate([]Function{f}, DefaultConfig()); err == nil {
			t.Fatalf("invalid helper facts accepted: %+v", helpers)
		}
	}
}

func TestGenericHelperNamesShareLookupAndPolicy(t *testing.T) {
	f := fact("project.caller", 1)
	f.Calls = []Call{{Callee: "project.helper[map[string][]int]", Local: true, Line: 2}}
	f.Helpers = []Helper{{Name: "project.helper[T]", Effects: []Effect{{Kind: IO, Detail: "write", Line: 8}}}}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if !result.Complete || len(result.Effects) != 1 || result.Effects[0].Kind != IO || result.Effects[0].Line != 2 {
		t.Fatalf("generic helper lookup: %+v", result)
	}
	if result.Effects[0].Detail != "project.helper[map[string][]int] → write" {
		t.Fatalf("diagnostic loses original callee: %+v", result.Effects)
	}
	config := DefaultConfig()
	config.Prefixes = append(config.Prefixes, Prefix{Path: "project.helper", Kind: None})
	result = evaluate(t, f, config).Functions[0]
	if !result.Complete || len(result.Effects) != 0 || result.UnclassifiedCalls != 0 {
		t.Fatalf("generic policy override: %+v", result)
	}
}

func TestCombinedCallLimitPreservesEarlierEvidence(t *testing.T) {
	f := fact("caller", 1)
	f.Calls = []Call{
		{Callee: "helper", Local: true, Line: 2},
		{Callee: "helper", Local: true, Line: 3},
		{Callee: "fmt.Println", Line: 4},
	}
	effects := make([]Effect, localEffectLimit/2+1)
	for index := range effects {
		effects[index] = Effect{Kind: IO, Detail: "write", Line: 8}
	}
	f.Helpers = []Helper{{Name: "helper", Effects: effects}}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if result.Complete || result.UnclassifiedCalls != 1 || len(result.Effects) != len(effects)+1 {
		t.Fatalf("combined call limit: %+v", result)
	}
	for _, effect := range result.Effects[:len(effects)] {
		if effect.Kind != IO || effect.Line != 2 {
			t.Fatalf("earlier call evidence lost: %+v", effect)
		}
	}
	last := result.Effects[len(effects)]
	if last.Kind != UnknownEffect || last.Line != 3 || last.Detail != "helper (local summary limit)" {
		t.Fatalf("limit evidence: %+v", last)
	}
}

func TestCallLimitReservesRoomForUnknownEvidence(t *testing.T) {
	f := fact("caller", 1)
	for range localEffectLimit {
		f.Calls = append(f.Calls, Call{Callee: "fmt.Println", Line: 2})
	}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if !result.Complete || len(result.Effects) != localEffectLimit {
		t.Fatalf("exact limit: complete=%v, effects=%d", result.Complete, len(result.Effects))
	}
	f.Calls = append(f.Calls, Call{Callee: "fmt.Println", Line: 3})
	result = evaluate(t, f, DefaultConfig()).Functions[0]
	if result.Complete || result.UnclassifiedCalls != 1 || len(result.Effects) != localEffectLimit {
		t.Fatalf("overflow: complete=%v, unclassified=%d, effects=%d", result.Complete, result.UnclassifiedCalls, len(result.Effects))
	}
	for _, effect := range result.Effects[:localEffectLimit-1] {
		if effect.Kind != IO || effect.Line != 2 {
			t.Fatalf("known evidence lost: %+v", effect)
		}
	}
	last := result.Effects[localEffectLimit-1]
	if last.Kind != UnknownEffect || last.Line != 3 || last.Detail != "fmt.Println (local summary limit)" {
		t.Fatalf("overflow evidence: %+v", last)
	}
}

func TestRecursiveHelperKeepsKnownEffects(t *testing.T) {
	f := fact("caller", 1)
	f.Calls = []Call{{Callee: "helper", Local: true, Line: 2}}
	f.Helpers = []Helper{{Name: "helper", Calls: []Call{{Callee: "helper", Local: true, Line: 8}}, Effects: []Effect{{Kind: IO, Detail: "write", Line: 9}}}}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if result.Complete || result.UnclassifiedCalls != 1 || len(result.Effects) != 2 {
		t.Fatalf("recursive helper evidence: %+v", result)
	}
	kinds := map[Kind]bool{}
	for _, effect := range result.Effects {
		kinds[effect.Kind] = true
		if effect.Line != 2 {
			t.Fatalf("recursive callsite: %+v", effect)
		}
	}
	if !kinds[IO] || !kinds[UnknownEffect] {
		t.Fatalf("recursion erases established effects: %+v", result.Effects)
	}
}

func TestInheritedEffectsNameImmediateHelper(t *testing.T) {
	f := fact("caller", 1)
	f.Calls = []Call{{Callee: "helper", Local: true, Line: 2}, {Callee: "fmt.Println", Line: 3}}
	f.Helpers = []Helper{{Name: "helper", Calls: []Call{{Callee: "os.Write", Line: 20}}}}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if len(result.Effects) != 2 {
		t.Fatalf("effects: %+v", result.Effects)
	}
	for _, effect := range result.Effects {
		if effect.Line == 2 && effect.Via != "helper" {
			t.Fatalf("inherited effect missing via: %+v", effect)
		}
		if effect.Line == 3 && effect.Via != "" {
			t.Fatalf("direct effect should not name via: %+v", effect)
		}
	}
}

func TestNestedHelperViaNamesImmediateCallee(t *testing.T) {
	f := fact("caller", 1)
	f.Calls = []Call{{Callee: "helper", Local: true, Line: 2}}
	f.Helpers = []Helper{
		{Name: "helper", Calls: []Call{{Callee: "inner", Local: true, Line: 20}}},
		{Name: "inner", Calls: []Call{{Callee: "os.Write", Line: 30}}},
	}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if len(result.Effects) != 1 {
		t.Fatalf("effects: %+v", result.Effects)
	}
	effect := result.Effects[0]
	if effect.Via != "helper" {
		t.Fatalf("via should be the immediate callee: %+v", effect)
	}
	if effect.Detail != "helper → inner → os.Write" {
		t.Fatalf("chain detail lost: %+v", effect)
	}
}

func TestViaUsesShortPackageQualifiedName(t *testing.T) {
	f := fact("caller", 1)
	f.Calls = []Call{{Callee: "example.com/mod/pkg.helper", Local: true, Line: 2}}
	f.Helpers = []Helper{{Name: "example.com/mod/pkg.helper", Calls: []Call{{Callee: "os.Write", Line: 20}}}}
	result := evaluate(t, f, DefaultConfig()).Functions[0]
	if len(result.Effects) != 1 || result.Effects[0].Via != "pkg.helper" {
		t.Fatalf("via not shortened: %+v", result.Effects)
	}
}
