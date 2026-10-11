package patterns

import (
	"fmt"
	"sort"
)

// Alattin (Thummalapenta & Xie, ASE 2009):
// Classic belief miners (PR-Miner, Engler) find X=>Y and flag ¬Y as bugs.
// But API preconditions are usually "one of these checks, not all."
// Alattin mines *alternative* patterns — e.g. for a method call, the
// precondition might be "check arg for null OR check return for error"
// (disjunction, not conjunction) — and flags code that uses the method
// with *neither* alternative checked. Cut false positives ~28%.
//
// For cyclo: disjunctive rules over call sets. PR-Miner mines "if you
// call A, you also call B"; Alattin mines "if you call A, you call B OR C".
// The OR rule is only interesting when neither single rule meets the
// thresholds on its own — that's exactly the case classic miners miss.

// DisjunctiveRule is an implicit programming rule with a disjunctive
// consequent: if antecedent, then at least one of alternatives.
type DisjunctiveRule struct {
	// Antecedent is the set of calls that imply the alternatives.
	Antecedent []string
	// Alternatives is the disjunction: at least one must be present.
	Alternatives []string
	// Support is the fraction of functions with antecedent+(B or C).
	Support float64
	// Confidence is P(B or C | antecedent).
	Confidence float64
}

// AlternativeViolation is a function that has the antecedent but none of
// the alternatives — the disjunctive precondition is unsatisfied.
type AlternativeViolation struct {
	FuncID string
	Rule   DisjunctiveRule
}

// MineDisjunctiveRules extracts "if A then (B or C)" rules from function
// call sets. A rule fires when P(B or C | A) >= minConfidence and support
// >= minSupport, and neither {A}->B nor {A}->C meets the thresholds on its
// own (otherwise the OR adds nothing — that's PR-Miner's territory).
func MineDisjunctiveRules(sets []CallSet) []DisjunctiveRule {
	if len(sets) == 0 {
		return nil
	}
	p := buildPostings(sets)
	var rules []DisjunctiveRule
	for a, list := range p.lists {
		if len(list) < minCountForSupport(p.total) {
			continue
		}
		rules = append(rules, disjunctiveRulesFor(p, sets, a)...)
	}
	return rules
}

// alternativePostings indexes only functions containing the antecedent.
// Alternatives that already imply a single-consequent rule are redundant.
func alternativePostings(p *callPostings, sets []CallSet, a string) map[string][]int {
	co := map[string][]int{}
	for _, i := range p.lists[a] {
		for b := range sets[i].Calls {
			if b != a {
				co[b] = append(co[b], i)
			}
		}
	}
	for b, list := range co {
		if _, ok := makeRuleFromCounts(p.total, len(p.lists[a]), len(list), []string{a}, b); ok {
			delete(co, b)
		}
	}
	return co
}

// disjunctiveRulesFor counts unions of antecedent-restricted postings.
func disjunctiveRulesFor(p *callPostings, sets []CallSet, a string) []DisjunctiveRule {
	co := alternativePostings(p, sets, a)
	names := make([]string, 0, len(co))
	for b := range co {
		names = append(names, b)
	}
	sort.Strings(names)
	var rules []DisjunctiveRule
	for i, b := range names {
		for _, c := range names[i+1:] {
			count := len(co[b]) + len(co[c]) - intersectSize(co[b], co[c])
			if r, ok := disjunctiveRuleFromCounts(p.total, len(p.lists[a]), count, a, b, c); ok {
				rules = append(rules, r)
			}
		}
	}
	return rules
}

// disjunctiveRuleFromCounts builds the rule from precomputed counts.
func disjunctiveRuleFromCounts(total, cntA, cntOr int, a, b, c string) (DisjunctiveRule, bool) {
	if cntOr == 0 || cntA == 0 {
		return DisjunctiveRule{}, false
	}
	support := float64(cntOr) / float64(total)
	confidence := float64(cntOr) / float64(cntA)
	if support < minSupport || confidence < minConfidence {
		return DisjunctiveRule{}, false
	}
	return DisjunctiveRule{
		Antecedent:   []string{a},
		Alternatives: []string{b, c},
		Support:      support,
		Confidence:   confidence,
	}, true
}

// countDisjunct counts sets containing a and at least one of alts.
func countDisjunct(sets []CallSet, a string, alts []string) int {
	count := 0
	for _, s := range sets {
		if hasDisjunct(s, a, alts) {
			count++
		}
	}
	return count
}

// hasDisjunct reports whether the set has a and at least one alternative.
func hasDisjunct(s CallSet, a string, alts []string) bool {
	if !s.Calls[a] {
		return false
	}
	return hasAnyAlternative(s, alts)
}

// hasAnyAlternative reports whether the set contains any alternative.
func hasAnyAlternative(s CallSet, alts []string) bool {
	for _, alt := range alts {
		if s.Calls[alt] {
			return true
		}
	}
	return false
}

// FindAlternativeViolations finds functions that have the antecedent but
// none of the alternatives — the disjunctive precondition is unsatisfied.
func FindAlternativeViolations(sets []CallSet, rules []DisjunctiveRule) []AlternativeViolation {
	var out []AlternativeViolation
	for _, rule := range rules {
		out = append(out, alternativeViolationsFor(sets, rule)...)
	}
	return out
}

// alternativeViolationsFor finds functions violating one disjunctive rule.
func alternativeViolationsFor(sets []CallSet, rule DisjunctiveRule) []AlternativeViolation {
	var out []AlternativeViolation
	for _, s := range sets {
		if hasAntecedent(s, rule.Antecedent) && !hasAnyAlternative(s, rule.Alternatives) {
			out = append(out, AlternativeViolation{FuncID: s.FuncID, Rule: rule})
		}
	}
	return out
}

// AlattinRuleCandidates converts disjunctive-rule violations to candidates.
func AlattinRuleCandidates(violations []AlternativeViolation, facts []*MiningFacts) []Candidate {
	factByID := map[string]*MiningFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, v := range violations {
		f := factByID[v.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:        AlattinRule,
			ScoreMilli:  750, // High: mined rules are repo-specific.
			Observation: fmt.Sprintf("calls %s but neither %s", joinBackticked(v.Rule.Antecedent), joinBacktickedOr(v.Rule.Alternatives)),
			Inference: fmt.Sprintf(
				"%.0f%% of functions that call %s also call %s",
				v.Rule.Confidence*100, joinBackticked(v.Rule.Antecedent), joinBacktickedOr(v.Rule.Alternatives),
			),
			PossibleRefactor: fmt.Sprintf("add one of the missing alternatives: %s", joinBacktickedOr(v.Rule.Alternatives)),
			Sites: []Site{
				{Path: f.Path, Line: f.Line, Name: f.Name},
			},
		})
	}
	return out
}

// joinBackticked renders items as "`a`, `b`".
func joinBackticked(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return joinWith(quoted, ", ")
}

// joinBacktickedOr renders items as "`a` or `b`".
func joinBacktickedOr(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return joinWith(quoted, " or ")
}

// joinWith joins parts with sep.
func joinWith(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
