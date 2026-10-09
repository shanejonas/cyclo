package quality

import "testing"

func TestDDDAggregateRule(t *testing.T) {
	f := fact("checkout", 1)
	f.DDD = []DDDViolation{
		{RuleID: "aggregate", Line: 2, Detail: "Order"},
		{RuleID: "aggregate", Line: 3, Detail: "OrderLine"},
	}
	c := DefaultConfig()
	report := evaluate(t, f, c)
	found := false
	for _, d := range report.Diagnostics {
		if d.RuleID == "aggregate" {
			found = true
			if d.Actual != 2 || d.Limit != 1 {
				t.Errorf("diagnostic = %+v, want actual 2 limit 1", d)
			}
		}
	}
	if !found {
		t.Error("expected aggregate diagnostic for 2 mutated types")
	}
}

func TestDDDAggregateRuleSingleTypeOK(t *testing.T) {
	f := fact("update", 1)
	f.DDD = []DDDViolation{
		{RuleID: "aggregate", Line: 2, Detail: "Order"},
	}
	c := DefaultConfig()
	report := evaluate(t, f, c)
	for _, d := range report.Diagnostics {
		if d.RuleID == "aggregate" {
			t.Errorf("unexpected aggregate diagnostic for single type: %+v", d)
		}
	}
}

func TestDDDRepositoryRule(t *testing.T) {
	f := fact("checkout", 1)
	f.DDD = []DDDViolation{
		{RuleID: "repository", Line: 5, Detail: "direct db call db.Query in business logic"},
	}
	c := DefaultConfig()
	report := evaluate(t, f, c)
	found := false
	for _, d := range report.Diagnostics {
		if d.RuleID == "repository" {
			found = true
		}
	}
	if !found {
		t.Error("expected repository diagnostic")
	}
}

func TestDDDMutableIdentityRule(t *testing.T) {
	f := fact("promote", 1)
	f.DDD = []DDDViolation{
		{RuleID: "mutable_identity", Line: 8, Detail: "assigns .ID outside a constructor"},
	}
	c := DefaultConfig()
	report := evaluate(t, f, c)
	found := false
	for _, d := range report.Diagnostics {
		if d.RuleID == "mutable_identity" {
			found = true
		}
	}
	if !found {
		t.Error("expected mutable_identity diagnostic")
	}
}

func TestDDDRulesSuppressible(t *testing.T) {
	f := fact("checkout", 1)
	f.PrecedingLine = "// cyclo-allow(aggregate, repository, mutable_identity): deliberate"
	f.DDD = []DDDViolation{
		{RuleID: "aggregate", Line: 2, Detail: "Order"},
		{RuleID: "aggregate", Line: 3, Detail: "OrderLine"},
		{RuleID: "repository", Line: 5, Detail: "db.Query"},
		{RuleID: "mutable_identity", Line: 8, Detail: ".ID"},
	}
	c := DefaultConfig()
	report := evaluate(t, f, c)
	for _, d := range report.Diagnostics {
		switch d.RuleID {
		case "aggregate", "repository", "mutable_identity":
			t.Errorf("suppressed rule %q still fired: %+v", d.RuleID, d)
		}
	}
}
