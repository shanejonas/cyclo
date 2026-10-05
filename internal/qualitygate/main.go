// Command qualitygate enforces cyclo's quality guardrails in CI without
// punishing growth.
//
// A checked-in baseline records each rule's total excess per 1,000 functions
// at some commit. The gate fails when a rule's current rate exceeds its
// baseline rate plus tolerance. Absolute finding counts would trip on any
// growing codebase; rates scale with it. Measuring excess (how far each
// finding overshoots its limit) instead of plain finding counts also catches
// an existing violation getting worse.
//
// Usage:
//
//	cyclo check --format json . > facts.json
//	qualitygate --baseline quality-baseline.json --facts facts.json
//
// Regenerate the baseline after a change legitimately moves the numbers:
//
//	qualitygate --write quality-baseline.json --facts facts.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	baselineVersion = 1
	// defaultTolerance is the relative headroom a rule's rate may grow before
	// the gate trips. It absorbs measurement noise (notably effect
	// attribution) while staying sensitive to real regressions.
	defaultTolerance = 0.05
	perUnit          = 1000.0
	epsilon          = 1e-9
)

type diagnostic struct {
	RuleID  string `json:"rule_id"`
	Name    string `json:"name"`
	Actual  int64  `json:"actual"`
	Limit   int64  `json:"limit"`
	Message string `json:"message"`
}

type checkReport struct {
	Summary struct {
		Functions int `json:"functions"`
	} `json:"summary"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type ruleBaseline struct {
	Limit  int64   `json:"limit"`
	Excess int64   `json:"excess"`
	Rate   float64 `json:"per_1000_functions"`
}

type qualityBaseline struct {
	Version   int                     `json:"version"`
	Generated string                  `json:"generated_at"`
	Commit    string                  `json:"commit"`
	Functions int                     `json:"functions"`
	Tolerance float64                 `json:"tolerance"`
	Rules     map[string]ruleBaseline `json:"rules"`
}

// aggregate folds a report's diagnostics into per-rule excess totals. Excess
// is how far each finding overshoots its limit, so both new findings and
// worsened ones move the number.
func aggregate(report checkReport) (map[string]ruleBaseline, error) {
	rules := make(map[string]ruleBaseline)
	for _, d := range report.Diagnostics {
		rb := rules[d.RuleID]
		if prev, ok := rules[d.RuleID]; ok && prev.Limit != d.Limit {
			return nil, fmt.Errorf("rule %s has inconsistent limits %d and %d", d.RuleID, prev.Limit, d.Limit)
		}
		if excess := d.Actual - d.Limit; excess > 0 {
			rb.Excess += excess
		}
		rb.Limit = d.Limit
		rules[d.RuleID] = rb
	}
	return rules, nil
}

func excessRate(excess int64, functions int) float64 {
	if functions <= 0 {
		return 0
	}
	return float64(excess) / float64(functions) * perUnit
}

type ruleViolation struct {
	rule     string
	rate     float64
	baseline float64
	limit    int64
	worst    []diagnostic
	// improved marks a rate that dropped far enough below the baseline that
	// the ratchet itself is stale: the baseline must be regenerated.
	improved bool
}

// worstOffenders returns the n findings with the largest excess for a rule,
// deterministically ordered.
func worstOffenders(diagnostics []diagnostic, rule string, n int) []diagnostic {
	var matches []diagnostic
	for _, d := range diagnostics {
		if d.RuleID == rule {
			matches = append(matches, d)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		ei, ej := matches[i].Actual-matches[i].Limit, matches[j].Actual-matches[j].Limit
		if ei != ej {
			return ei > ej
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > n {
		matches = matches[:n]
	}
	return matches
}

// checkBaseline compares a report against the baseline. A rule trips when its
// excess rate exceeds the baseline rate plus tolerance. A rule missing from
// the baseline is treated as a zero baseline, so any excess trips it. A
// changed limit means the policy moved out from under the baseline, which is
// a hard error rather than a violation. A rule whose rate improves past the
// tolerance also trips: the baseline is now loose, so the same PR must
// regenerate it (make quality-baseline) to tighten the ratchet.
func checkBaseline(base qualityBaseline, report checkReport) ([]ruleViolation, error) {
	current, err := aggregate(report)
	if err != nil {
		return nil, err
	}
	var violations []ruleViolation
	for rule, cur := range current {
		violation, err := checkRule(rule, cur, base, report)
		if err != nil {
			return nil, err
		}
		if violation != nil {
			violations = append(violations, *violation)
		}
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].rule < violations[j].rule })
	return violations, nil
}

// checkRule compares one rule's excess rate against its baseline. It returns
// a violation when the rate regresses past the tolerance band, or when it
// improves past it — a loose ratchet must be regenerated in the same change.
func checkRule(rule string, cur ruleBaseline, base qualityBaseline, report checkReport) (*ruleViolation, error) {
	baserule, ok := base.Rules[rule]
	if !ok {
		baserule = ruleBaseline{Limit: cur.Limit}
	} else if baserule.Limit != cur.Limit {
		return nil, fmt.Errorf("rule %s limit changed from %d to %d; regenerate the baseline", rule, baserule.Limit, cur.Limit)
	}
	rate := excessRate(cur.Excess, report.Summary.Functions)
	drifted, improved := baselineDrift(rate, baserule.Rate, base.Tolerance)
	if !drifted {
		return nil, nil
	}
	violation := &ruleViolation{rule: rule, rate: rate, baseline: baserule.Rate, limit: cur.Limit, improved: improved}
	if !improved {
		violation.worst = worstOffenders(report.Diagnostics, rule, 5)
	}
	return violation, nil
}

// baselineDrift reports whether a rule's excess rate moved past the tolerance
// band around its baseline, and whether the move was an improvement. A loose
// ratchet trips just like a regression: the same change must regenerate the
// baseline.
func baselineDrift(rate, baselineRate, tolerance float64) (drifted, improved bool) {
	if rate > baselineRate*(1+tolerance)+epsilon {
		return true, false
	}
	if rate < baselineRate*(1-tolerance)-epsilon {
		return true, true
	}
	return false, false
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func writeBaseline(report checkReport, tolerance float64) (qualityBaseline, error) {
	rules, err := aggregate(report)
	if err != nil {
		return qualityBaseline{}, err
	}
	base := qualityBaseline{
		Version:   baselineVersion,
		Generated: time.Now().UTC().Format(time.RFC3339),
		Commit:    gitCommit(),
		Functions: report.Summary.Functions,
		Tolerance: tolerance,
		Rules:     make(map[string]ruleBaseline, len(rules)),
	}
	for rule, rb := range rules {
		rb.Rate = excessRate(rb.Excess, report.Summary.Functions)
		base.Rules[rule] = rb
	}
	return base, nil
}

func readReport(factsPath string) (checkReport, error) {
	var input io.Reader = os.Stdin
	if factsPath != "" {
		file, err := os.Open(factsPath)
		if err != nil {
			return checkReport{}, fmt.Errorf("open facts: %w", err)
		}
		defer file.Close()
		input = file
	}
	var report checkReport
	if err := json.NewDecoder(input).Decode(&report); err != nil {
		return checkReport{}, fmt.Errorf("decode check report: %w", err)
	}
	return report, nil
}

func runWrite(writePath, factsPath string, tolerance float64) int {
	report, err := readReport(factsPath)
	if err != nil {
		return fail(2, "qualitygate: %v", err)
	}
	base, err := writeBaseline(report, tolerance)
	if err != nil {
		return fail(2, "qualitygate: %v", err)
	}
	data, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return fail(2, "qualitygate: encode baseline: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(writePath, data, 0644); err != nil {
		return fail(2, "qualitygate: %v", err)
	}
	fmt.Printf("wrote baseline for %d functions and %d rules to %s\n", base.Functions, len(base.Rules), writePath)
	return 0
}

func runCheck(baselinePath, factsPath string) int {
	report, err := readReport(factsPath)
	if err != nil {
		return fail(2, "qualitygate: %v", err)
	}
	base, err := loadBaseline(baselinePath)
	if err != nil {
		return fail(2, "qualitygate: %v", err)
	}
	violations, err := checkBaseline(base, report)
	if err != nil {
		return fail(2, "qualitygate: %v", err)
	}
	if len(violations) > 0 {
		printViolations(base, violations)
		return 1
	}
	fmt.Printf("quality gate passed: %d rules within baseline across %d functions\n", len(base.Rules), report.Summary.Functions)
	return 0
}

func fail(code int, format string, args ...any) int {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	return code
}

func loadBaseline(path string) (qualityBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return qualityBaseline{}, fmt.Errorf("read baseline: %w", err)
	}
	var base qualityBaseline
	if err := json.Unmarshal(data, &base); err != nil {
		return qualityBaseline{}, fmt.Errorf("decode baseline: %w", err)
	}
	return base, nil
}

func printViolations(base qualityBaseline, violations []ruleViolation) {
	fmt.Printf("quality gate failed: %d rule(s) outside baseline (tolerance %.0f%%):\n", len(violations), base.Tolerance*100)
	for _, v := range violations {
		if v.improved {
			fmt.Printf("  %s: %.2f excess per 1000 functions, baseline %.2f (limit %d) — improved, tighten the ratchet\n", v.rule, v.rate, v.baseline, v.limit)
			continue
		}
		fmt.Printf("  %s: %.2f excess per 1000 functions, baseline %.2f (limit %d)\n", v.rule, v.rate, v.baseline, v.limit)
		for _, w := range v.worst {
			fmt.Printf("    %s: %s\n", w.Name, w.Message)
		}
	}
	fmt.Println("run make quality-baseline to regenerate the baseline")
}

func main() {
	baselinePath := flag.String("baseline", "", "baseline file to check against")
	writePath := flag.String("write", "", "write a new baseline to this path instead of checking")
	factsPath := flag.String("facts", "", "check JSON report (default: stdin)")
	tolerance := flag.Float64("tolerance", defaultTolerance, "relative headroom recorded by --write")
	flag.Parse()

	if (*baselinePath == "") == (*writePath == "") {
		fmt.Fprintln(os.Stderr, "qualitygate: exactly one of --baseline or --write is required")
		os.Exit(2)
	}
	if *tolerance < 0 {
		fmt.Fprintln(os.Stderr, "qualitygate: tolerance must not be negative")
		os.Exit(2)
	}
	if *writePath != "" {
		os.Exit(runWrite(*writePath, *factsPath, *tolerance))
	}
	os.Exit(runCheck(*baselinePath, *factsPath))
}
