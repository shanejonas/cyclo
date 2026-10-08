package patterns

import "testing"

func TestMineRulesBasic(t *testing.T) {
	// 10 functions: 9 call {lock, unlock}, 1 calls {lock} only.
	// Should mine {lock} -> unlock with 90% confidence.
	var sets []CallSet
	for i := 0; i < 9; i++ {
		sets = append(sets, CallSet{
			FuncID: string(rune('a' + i)),
			Calls:  map[string]bool{"lock": true, "unlock": true},
		})
	}
	sets = append(sets, CallSet{
		FuncID: "j",
		Calls:  map[string]bool{"lock": true},
	})
	rules := MineRules(sets)
	found := false
	for _, r := range rules {
		if len(r.Antecedent) == 1 && r.Antecedent[0] == "lock" && r.Consequent == "unlock" {
			found = true
			if r.Confidence < 0.89 {
				t.Errorf("confidence = %f, want >= 0.9", r.Confidence)
			}
		}
	}
	if !found {
		t.Errorf("expected rule {lock} -> unlock, got %v", rules)
	}
}

func TestFindViolations(t *testing.T) {
	sets := []CallSet{
		{FuncID: "a", Calls: map[string]bool{"lock": true, "unlock": true}},
		{FuncID: "b", Calls: map[string]bool{"lock": true}}, // violation
	}
	rules := []MinedRuleDef{
		{Antecedent: []string{"lock"}, Consequent: "unlock", Confidence: 0.95},
	}
	violations := FindViolations(sets, rules)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].FuncID != "b" {
		t.Errorf("violation FuncID = %q, want b", violations[0].FuncID)
	}
}
