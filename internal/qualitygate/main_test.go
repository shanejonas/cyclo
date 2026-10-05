package main

import (
	"strings"
	"testing"
)

func testReport(functions int, findings ...diagnostic) checkReport {
	report := checkReport{}
	report.Summary.Functions = functions
	report.Diagnostics = findings
	return report
}

func finding(rule, name string, actual, limit int64) diagnostic {
	return diagnostic{RuleID: rule, Name: name, Actual: actual, Limit: limit}
}

func mustAggregate(t *testing.T, report checkReport) map[string]ruleBaseline {
	t.Helper()
	rules, err := aggregate(report)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	return rules
}

func TestAggregateSumsExcess(t *testing.T) {
	rules := mustAggregate(t, testReport(100,
		finding("fn_params", "a", 6, 4), // excess 2
		finding("fn_params", "b", 5, 4), // excess 1
	))
	if got := rules["fn_params"].Excess; got != 3 {
		t.Fatalf("excess = %d, want 3", got)
	}
	if got := rules["fn_params"].Limit; got != 4 {
		t.Fatalf("limit = %d, want 4", got)
	}
}

func TestAggregateRejectsInconsistentLimits(t *testing.T) {
	_, err := aggregate(testReport(100,
		finding("fn_params", "a", 6, 4),
		finding("fn_params", "b", 6, 5),
	))
	if err == nil || !strings.Contains(err.Error(), "inconsistent limits") {
		t.Fatalf("expected inconsistent limits error, got %v", err)
	}
}

func baselineFor(t *testing.T, report checkReport) qualityBaseline {
	t.Helper()
	base, err := writeBaseline(report, defaultTolerance)
	if err != nil {
		t.Fatalf("writeBaseline: %v", err)
	}
	return base
}

func TestCheckBaselinePassesOnIdenticalReport(t *testing.T) {
	report := testReport(100, finding("fn_params", "a", 6, 4))
	violations, err := checkBaseline(baselineFor(t, report), report)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestCheckBaselineTripsOnNewFinding(t *testing.T) {
	base := baselineFor(t, testReport(100, finding("fn_params", "a", 6, 4)))
	worse := testReport(100,
		finding("fn_params", "a", 6, 4),
		finding("fn_params", "b", 5, 4),
	)
	violations, err := checkBaseline(base, worse)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 1 || violations[0].rule != "fn_params" {
		t.Fatalf("expected one fn_params violation, got %v", violations)
	}
}

func TestCheckBaselineTripsOnWorsenedFinding(t *testing.T) {
	base := baselineFor(t, testReport(100, finding("fn_params", "a", 6, 4)))
	worse := testReport(100, finding("fn_params", "a", 9, 4)) // excess 2 -> 5
	violations, err := checkBaseline(base, worse)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected a violation for worsened excess, got %v", violations)
	}
}

func TestCheckBaselineTripsOnImprovement(t *testing.T) {
	// A past improvement (side_effect_density accuracy work) landed without
	// refreshing the baseline, leaving the ratchet loose. The gate must force
	// the refresh into the same PR.
	base := baselineFor(t, testReport(100, finding("side_effect_density", "a", 2500, 500)))
	better := testReport(100, finding("side_effect_density", "a", 1500, 500)) // excess 2000 -> 1000
	violations, err := checkBaseline(base, better)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 1 || !violations[0].improved || violations[0].rule != "side_effect_density" {
		t.Fatalf("expected an improvement violation, got %v", violations)
	}
}

func TestCheckBaselineIgnoresNoise(t *testing.T) {
	// Small movements inside the tolerance band pass either way.
	base := baselineFor(t, testReport(1000, finding("fn_params", "a", 24, 4)))
	quieter := testReport(1000, finding("fn_params", "a", 23, 4)) // rate 20 -> 19
	violations, err := checkBaseline(base, quieter)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations inside tolerance, got %v", violations)
	}
}

func TestCheckBaselineScalesWithGrowth(t *testing.T) {
	// Doubling the codebase with proportional findings keeps the rate flat.
	base := baselineFor(t, testReport(100,
		finding("fn_params", "a", 6, 4),
		finding("fn_params", "b", 6, 4),
	))
	grown := testReport(200,
		finding("fn_params", "a", 6, 4),
		finding("fn_params", "b", 6, 4),
		finding("fn_params", "c", 6, 4),
		finding("fn_params", "d", 6, 4),
	)
	violations, err := checkBaseline(base, grown)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("proportional growth should pass, got %v", violations)
	}
}

func TestCheckBaselineTripsOnUnbaselinedRule(t *testing.T) {
	base := baselineFor(t, testReport(100, finding("fn_params", "a", 6, 4)))
	withNewRule := testReport(100,
		finding("fn_params", "a", 6, 4),
		finding("mutated_targets", "a", 5, 3),
	)
	violations, err := checkBaseline(base, withNewRule)
	if err != nil {
		t.Fatalf("checkBaseline: %v", err)
	}
	if len(violations) != 1 || violations[0].rule != "mutated_targets" {
		t.Fatalf("expected a mutated_targets violation, got %v", violations)
	}
}

func TestCheckBaselineRejectsPolicyChange(t *testing.T) {
	base := baselineFor(t, testReport(100, finding("fn_params", "a", 6, 4)))
	changed := testReport(100, finding("fn_params", "a", 7, 6))
	_, err := checkBaseline(base, changed)
	if err == nil || !strings.Contains(err.Error(), "regenerate the baseline") {
		t.Fatalf("expected policy-change error, got %v", err)
	}
}

func TestWorstOffendersOrderedByExcess(t *testing.T) {
	diagnostics := []diagnostic{
		finding("fn_params", "small", 5, 4),
		finding("fn_params", "big", 9, 4),
		finding("fn_params", "mid", 6, 4),
	}
	worst := worstOffenders(diagnostics, "fn_params", 2)
	if len(worst) != 2 || worst[0].Name != "big" || worst[1].Name != "mid" {
		t.Fatalf("unexpected ordering: %v", worst)
	}
}
