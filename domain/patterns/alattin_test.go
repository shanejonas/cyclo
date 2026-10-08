package patterns

import "testing"

func TestMineDisjunctiveRules(t *testing.T) {
	// 10 functions call `lock`: 5 also call `unlock`, 4 also call `cleanup`,
	// 1 calls `lock` alone. Neither single rule reaches 90% confidence, but
	// {lock} -> (unlock or cleanup) does: 9/10 = 90%.
	var sets []CallSet
	for i := 0; i < 5; i++ {
		sets = append(sets, CallSet{
			FuncID: string(rune('a' + i)),
			Calls:  map[string]bool{"lock": true, "unlock": true},
		})
	}
	for i := 0; i < 4; i++ {
		sets = append(sets, CallSet{
			FuncID: string(rune('f' + i)),
			Calls:  map[string]bool{"lock": true, "cleanup": true},
		})
	}
	sets = append(sets, CallSet{
		FuncID: "j",
		Calls:  map[string]bool{"lock": true},
	})
	rules := MineDisjunctiveRules(sets)
	if len(rules) != 1 {
		t.Fatalf("expected 1 disjunctive rule, got %d: %v", len(rules), rules)
	}
	r := rules[0]
	if len(r.Antecedent) != 1 || r.Antecedent[0] != "lock" {
		t.Errorf("antecedent = %v, want [lock]", r.Antecedent)
	}
	if !hasString(r.Alternatives, "unlock") || !hasString(r.Alternatives, "cleanup") {
		t.Errorf("alternatives = %v, want [unlock cleanup]", r.Alternatives)
	}
	if r.Confidence < 0.89 {
		t.Errorf("confidence = %f, want >= 0.9", r.Confidence)
	}
	if r.Support < 0.89 {
		t.Errorf("support = %f, want >= 0.9", r.Support)
	}
}

func TestFindAlternativeViolations(t *testing.T) {
	rule := DisjunctiveRule{
		Antecedent:   []string{"lock"},
		Alternatives: []string{"unlock", "cleanup"},
		Confidence:   1.0,
	}
	sets := []CallSet{
		{FuncID: "a", Calls: map[string]bool{"lock": true, "unlock": true}}, // ok: has alternative
		{FuncID: "b", Calls: map[string]bool{"lock": true}},                 // violation: neither alternative
		{FuncID: "c", Calls: map[string]bool{"unlock": true}},               // ok: no antecedent
	}
	violations := FindAlternativeViolations(sets, []DisjunctiveRule{rule})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].FuncID != "b" {
		t.Errorf("violation FuncID = %q, want b", violations[0].FuncID)
	}
}

func TestNoRedundantDisjunctiveRule(t *testing.T) {
	// Every function calls lock, unlock, and cleanup. {lock}->unlock already
	// meets the thresholds on its own, so the OR rule is redundant and must
	// not be emitted.
	var sets []CallSet
	for i := 0; i < 10; i++ {
		sets = append(sets, CallSet{
			FuncID: string(rune('a' + i)),
			Calls:  map[string]bool{"lock": true, "unlock": true, "cleanup": true},
		})
	}
	rules := MineDisjunctiveRules(sets)
	if len(rules) != 0 {
		t.Errorf("expected no disjunctive rules (redundant), got %v", rules)
	}
}

func TestMineDisjunctiveRulesEmpty(t *testing.T) {
	if rules := MineDisjunctiveRules(nil); rules != nil {
		t.Errorf("expected nil for empty input, got %v", rules)
	}
}

func hasString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
