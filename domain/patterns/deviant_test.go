package patterns

import "testing"

func TestMineErrorBeliefs(t *testing.T) {
	// 10 call sites for `fetch`: 9 check the error, 1 doesn't.
	var sites []ErrorCheckSite
	for i := 0; i < 9; i++ {
		sites = append(sites, ErrorCheckSite{
			FuncID:  string(rune('a' + i)),
			Callee:  "fetch",
			Checked: true,
		})
	}
	sites = append(sites, ErrorCheckSite{
		FuncID:  "j",
		Callee:  "fetch",
		Checked: false,
	})
	beliefs := MineErrorBeliefs(sites)
	if len(beliefs) != 1 {
		t.Fatalf("expected 1 belief, got %d", len(beliefs))
	}
	if beliefs[0].Callee != "fetch" {
		t.Errorf("belief Callee = %q, want fetch", beliefs[0].Callee)
	}
	if beliefs[0].Confidence < 0.89 {
		t.Errorf("confidence = %f, want >= 0.9", beliefs[0].Confidence)
	}
}

func TestFindBeliefViolations(t *testing.T) {
	sites := []ErrorCheckSite{
		{FuncID: "a", Line: 10, Callee: "fetch", Checked: true},
		{FuncID: "b", Line: 20, Callee: "fetch", Checked: false}, // violation
	}
	beliefs := []CallBelief{
		{Callee: "fetch", Belief: "error checked", Confidence: 0.95, Support: 10},
	}
	violations := FindBeliefViolations(sites, beliefs)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].FuncID != "b" {
		t.Errorf("violation FuncID = %q, want b", violations[0].FuncID)
	}
}
