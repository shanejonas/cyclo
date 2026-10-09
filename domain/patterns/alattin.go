package patterns

import "sort"

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
	var rules []DisjunctiveRule
	for a := range countSingles(sets) {
		rules = append(rules, disjunctiveRulesFor(sets, a)...)
	}
	return rules
}

// disjunctiveRulesFor mines {A} -> (B or C) rules for one antecedent.
func disjunctiveRulesFor(sets []CallSet, a string) []DisjunctiveRule {
	co := coOccurring(sets, a)
	cntA := countCoOccur(sets, []string{a})
	var rules []DisjunctiveRule
	for i := 0; i < len(co); i++ {
		for j := i + 1; j < len(co); j++ {
			if r, ok := makeDisjunctiveRule(sets, a, co[i], co[j], cntA); ok {
				rules = append(rules, r)
			}
		}
	}
	return rules
}

// coOccurring returns sorted callees (other than a) that co-occur with a
// at least once. Only these can form alternatives worth checking.
func coOccurring(sets []CallSet, a string) []string {
	var co []string
	for b := range countSingles(sets) {
		if b != a && countCoOccur(sets, []string{a, b}) > 0 {
			co = append(co, b)
		}
	}
	sort.Strings(co)
	return co
}

// makeDisjunctiveRule builds the {A} -> (B or C) rule if it meets the
// thresholds and isn't redundant with the single-consequent rules.
func makeDisjunctiveRule(sets []CallSet, a, b, c string, cntA int) (DisjunctiveRule, bool) {
	if disjunctRedundant(sets, a, b, c, cntA) {
		return DisjunctiveRule{}, false
	}
	cntOr := countDisjunct(sets, a, []string{b, c})
	return disjunctiveRuleFromCounts(len(sets), cntA, cntOr, a, b, c)
}

// disjunctRedundant reports whether {A}->B or {A}->C already meets the
// rule thresholds on its own, making the OR rule redundant.
func disjunctRedundant(sets []CallSet, a, b, c string, cntA int) bool {
	total := len(sets)
	if _, ok := makeRuleFromCounts(total, cntA, countCoOccur(sets, []string{a, b}), []string{a}, b); ok {
		return true
	}
	_, ok := makeRuleFromCounts(total, cntA, countCoOccur(sets, []string{a, c}), []string{a}, c)
	return ok
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
