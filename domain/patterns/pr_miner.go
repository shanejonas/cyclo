package patterns

import (
	"fmt"
	"sort"
)

// PR-Miner (Li & Zhou, FSE 2005):
// Automatically extract implicit programming rules and detect violations
// in large software code. Uses frequent itemset mining over
// function/variable/type co-occurrences — zero annotations needed.
//
// Example: "functions that call `lock` also call `unlock`" is mined from
// the codebase itself. A function that calls `lock` but not `unlock`
// violates the rule — that's a bug.
//
// For cyclo: a `mined_rule` pattern kind. The items are function calls;
// the rules are "if you call A and B, you usually also call C".
// This is the self-supervised version of every hand-written linter rule.

// minSupport is the minimum fraction of functions that must contain an
// itemset for it to be a rule. Lower = more rules but noisier.
const minSupport = 0.1

// minConfidence is the minimum fraction of functions with the antecedent
// that also have the consequent.
const minConfidence = 0.9

// CallSet is the set of functions called by one function.
type CallSet struct {
	FuncID string
	Calls  map[string]bool
}

// MinedRule is an implicit programming rule: if antecedent, then consequent.
type MinedRuleDef struct {
	// Antecedent is the set of calls that imply the consequent.
	Antecedent []string
	// Consequent is the call that should also be present.
	Consequent string
	// Support is the fraction of functions with antecedent+consequent.
	Support float64
	// Confidence is P(consequent | antecedent).
	Confidence float64
}

// RuleViolation is a function that violates a mined rule.
type RuleViolation struct {
	FuncID string
	Rule   MinedRuleDef
}

// MineRules extracts implicit call-co-occurrence rules from function call sets.
func MineRules(sets []CallSet) []MinedRuleDef {
	if len(sets) == 0 {
		return nil
	}
	var rules []MinedRuleDef
	rules = append(rules, mineSingleAntecedent(sets)...)
	rules = append(rules, mineDoubleAntecedent(sets)...)
	return rules
}

// mineSingleAntecedent mines rules of form {A} -> B.
func mineSingleAntecedent(sets []CallSet) []MinedRuleDef {
	single := countSingles(sets)
	var rules []MinedRuleDef
	for a, cntA := range single {
		for b := range single {
			if a == b {
				continue
			}
			if r, ok := makeRule(sets, []string{a}, b, cntA); ok {
				rules = append(rules, r)
			}
		}
	}
	return rules
}

// mineDoubleAntecedent mines rules of form {A,B} -> C.
func mineDoubleAntecedent(sets []CallSet) []MinedRuleDef {
	triples := countTriples(sets)
	var rules []MinedRuleDef
	for key, cntABC := range triples {
		parts := parseTripleKey(key)
		if len(parts) != 3 {
			continue
		}
		a, b, c := parts[0], parts[1], parts[2]
		cntAB := countCoOccur(sets, []string{a, b})
		if r, ok := makeRuleFromCounts(len(sets), cntAB, cntABC, []string{a, b}, c); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

// makeRule creates a rule if support and confidence thresholds are met.
// cntA is the count of the antecedent, cntAB is the count of antecedent+consequent.
func makeRule(sets []CallSet, antecedent []string, consequent string, cntA int) (MinedRuleDef, bool) {
	cntAB := countCoOccur(sets, append(antecedent, consequent))
	return makeRuleFromCounts(len(sets), cntA, cntAB, antecedent, consequent)
}

// makeRuleFromCounts creates a rule from precomputed counts.
func makeRuleFromCounts(total, cntA, cntAB int, antecedent []string, consequent string) (MinedRuleDef, bool) {
	if cntAB == 0 || cntA == 0 {
		return MinedRuleDef{}, false
	}
	support := float64(cntAB) / float64(total)
	confidence := float64(cntAB) / float64(cntA)
	if support < minSupport || confidence < minConfidence {
		return MinedRuleDef{}, false
	}
	return MinedRuleDef{
		Antecedent: antecedent,
		Consequent: consequent,
		Support:    support,
		Confidence: confidence,
	}, true
}

// FindViolations finds functions that have the antecedent but not the consequent.
func FindViolations(sets []CallSet, rules []MinedRuleDef) []RuleViolation {
	var out []RuleViolation
	for _, rule := range rules {
		out = append(out, violationsForRule(sets, rule)...)
	}
	return out
}

// violationsForRule finds functions violating one rule.
func violationsForRule(sets []CallSet, rule MinedRuleDef) []RuleViolation {
	var out []RuleViolation
	for _, s := range sets {
		if hasAntecedent(s, rule.Antecedent) && !s.Calls[rule.Consequent] {
			out = append(out, RuleViolation{FuncID: s.FuncID, Rule: rule})
		}
	}
	return out
}

// hasAntecedent reports whether the call set contains all antecedent items.
func hasAntecedent(s CallSet, antecedent []string) bool {
	for _, a := range antecedent {
		if !s.Calls[a] {
			return false
		}
	}
	return true
}

func countSingles(sets []CallSet) map[string]int {
	counts := map[string]int{}
	for _, s := range sets {
		for c := range s.Calls {
			counts[c]++
		}
	}
	return counts
}

func countCoOccur(sets []CallSet, items []string) int {
	count := 0
	for _, s := range sets {
		hasAll := true
		for _, item := range items {
			if !s.Calls[item] {
				hasAll = false
				break
			}
		}
		if hasAll {
			count++
		}
	}
	return count
}

func countTriples(sets []CallSet) map[string]int {
	counts := map[string]int{}
	for _, s := range sets {
		var calls []string
		for c := range s.Calls {
			calls = append(calls, c)
		}
		sort.Strings(calls)
		for i := 0; i < len(calls); i++ {
			for j := i + 1; j < len(calls); j++ {
				for k := j + 1; k < len(calls); k++ {
					key := calls[i] + "\x00" + calls[j] + "\x00" + calls[k]
					counts[key]++
				}
			}
		}
	}
	return counts
}

func parseTripleKey(key string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}

// MinedRuleCandidates converts violations to pattern candidates.
func MinedRuleCandidates(violations []RuleViolation, facts []*FuncFacts) []Candidate {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, v := range violations {
		f := factByID[v.FuncID]
		if f == nil {
			continue
		}
		ant := ""
		for i, a := range v.Rule.Antecedent {
			if i > 0 {
				ant += ", "
			}
			ant += "`" + a + "`"
		}
		out = append(out, Candidate{
			Kind:        MinedRule,
			ScoreMilli:  750, // High: mined rules are repo-specific.
			Observation: fmt.Sprintf("calls %s but not `%s`", ant, v.Rule.Consequent),
			Inference: fmt.Sprintf(
				"%.0f%% of functions that call %s also call `%s`",
				v.Rule.Confidence*100, ant, v.Rule.Consequent,
			),
			PossibleRefactor: fmt.Sprintf("add the missing `%s` call", v.Rule.Consequent),
			Sites: []Site{
				{Path: f.Path, Line: f.Line, Name: f.Name},
			},
		})
	}
	return out
}

// BuildCallSets constructs CallSets from FuncFacts by extracting called
// function names from PDG Call nodes.
func BuildCallSets(facts []*FuncFacts) []CallSet {
	var out []CallSet
	for _, f := range facts {
		if calls := extractCalls(f); len(calls) > 0 {
			out = append(out, CallSet{FuncID: f.ID, Calls: calls})
		}
	}
	return out
}

// extractCalls returns the set of callee IDs from a function's PDG.
func extractCalls(f *FuncFacts) map[string]bool {
	calls := map[string]bool{}
	if f.Pdg == nil {
		return calls
	}
	for _, n := range f.Pdg.Nodes {
		if n.Kind == Call && n.CalleeID != "" {
			calls[n.CalleeID] = true
		}
	}
	return calls
}
