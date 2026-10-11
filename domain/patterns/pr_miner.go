package patterns

import (
	"fmt"
	"math"
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
	p := buildPostings(sets)
	var rules []MinedRuleDef
	rules = append(rules, mineSingleAntecedent(p)...)
	rules = append(rules, mineDoubleAntecedent(p, sets)...)
	return rules
}

// callPostings is an inverted index from call name to the sorted list of
// function indices containing that call. Built once per MineRules call so
// co-occurrence counts become postings-list intersections instead of
// full scans over all functions.
type callPostings struct {
	lists map[string][]int
	total int
}

// buildPostings indexes which functions contain each call.
// Indices are appended in increasing order, so every list stays sorted.
func buildPostings(sets []CallSet) *callPostings {
	p := &callPostings{lists: make(map[string][]int), total: len(sets)}
	for i, s := range sets {
		for c := range s.Calls {
			p.lists[c] = append(p.lists[c], i)
		}
	}
	return p
}

// minCountForSupport returns the smallest integer count meeting minSupport.
// Pairs below this count can never produce a rule, so they are pruned
// before the (more expensive) intersection.
func minCountForSupport(total int) int {
	return int(math.Ceil(minSupport * float64(total)))
}

// intersectSize returns |a ∩ b| for sorted index lists via two-pointer merge.
func intersectSize(a, b []int) int {
	i, j, n := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			n++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return n
}

// intersectLists returns a ∩ b for sorted index lists, preserving order.
func intersectLists(a, b []int) []int {
	var out []int
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
}

// sortedContains reports whether sorted list contains x via binary search.
func sortedContains(list []int, x int) bool {
	lo, hi := 0, len(list)
	for lo < hi {
		mid := (lo + hi) / 2
		if list[mid] < x {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(list) && list[lo] == x
}

// mineSingleAntecedent mines rules of form {A} -> B using the postings index.
// A pair is skipped without intersecting when either side alone is too rare
// to meet minSupport, since co-occurrence cannot exceed either side's count.
func mineSingleAntecedent(p *callPostings) []MinedRuleDef {
	minCnt := minCountForSupport(p.total)
	var rules []MinedRuleDef
	for a, listA := range p.lists {
		if len(listA) < minCnt {
			continue
		}
		rules = append(rules, singleConsequents(p, a, listA, minCnt)...)
	}
	return rules
}

// singleConsequents mines {a} -> b rules for one fixed antecedent a.
func singleConsequents(p *callPostings, a string, listA []int, minCnt int) []MinedRuleDef {
	var rules []MinedRuleDef
	for b, listB := range p.lists {
		if a == b || len(listB) < minCnt {
			continue
		}
		cntAB := intersectSize(listA, listB)
		if r, ok := makeRuleFromCounts(p.total, len(listA), cntAB, []string{a}, b); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

// mineDoubleAntecedent mines rules of form {A,B} -> C.
// cntABC comes from the triple enumeration; cntAB is a postings intersection
// instead of a full scan. Triples below minSupport skip the intersection.
func mineDoubleAntecedent(p *callPostings, sets []CallSet) []MinedRuleDef {
	minCnt := minCountForSupport(p.total)
	names, indices := frequentCallIndices(p)
	triples := countTriples(sets, indices)
	var rules []MinedRuleDef
	for key, cntABC := range triples {
		if cntABC < minCnt {
			continue
		}
		a, b, c := names[key[0]], names[key[1]], names[key[2]]
		cntAB := intersectSize(p.lists[a], p.lists[b])
		if r, ok := makeRuleFromCounts(len(sets), cntAB, cntABC, []string{a, b}, c); ok {
			rules = append(rules, r)
		}
	}
	return rules
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
	p := buildPostings(sets)
	var out []RuleViolation
	for _, rule := range rules {
		out = append(out, violationsForRule(p, sets, rule)...)
	}
	return out
}

// violationsForRule finds functions violating one rule via the index.
// The antecedent intersection is walked in function order, so output order
// matches the original full-scan implementation.
func violationsForRule(p *callPostings, sets []CallSet, rule MinedRuleDef) []RuleViolation {
	ant := antecedentPostings(p, rule.Antecedent)
	if len(ant) == 0 {
		return nil
	}
	cons := p.lists[rule.Consequent]
	var out []RuleViolation
	for _, idx := range ant {
		if !sortedContains(cons, idx) {
			out = append(out, RuleViolation{FuncID: sets[idx].FuncID, Rule: rule})
		}
	}
	return out
}

// antecedentPostings returns the sorted function indices containing every
// antecedent call. An empty antecedent matches all functions.
func antecedentPostings(p *callPostings, antecedent []string) []int {
	if len(antecedent) == 0 {
		all := make([]int, p.total)
		for i := range all {
			all[i] = i
		}
		return all
	}
	out := p.lists[antecedent[0]]
	for _, a := range antecedent[1:] {
		out = intersectLists(out, p.lists[a])
		if len(out) == 0 {
			break
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

// frequentCallIndices assigns sorted integer IDs only to calls that can
// meet support. A triple cannot occur more often than any of its members.
func frequentCallIndices(p *callPostings) ([]string, map[string]int) {
	var names []string
	for name, list := range p.lists {
		if len(list) >= minCountForSupport(p.total) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	indices := make(map[string]int, len(names))
	for i, name := range names {
		indices[name] = i
	}
	return names, indices
}

func countTriples(sets []CallSet, indices map[string]int) map[[3]int]int {
	counts := map[[3]int]int{}
	for _, set := range sets {
		addTriples(indexedCalls(set, indices), counts)
	}
	return counts
}

func indexedCalls(set CallSet, indices map[string]int) []int {
	var calls []int
	for name := range set.Calls {
		if i, ok := indices[name]; ok {
			calls = append(calls, i)
		}
	}
	sort.Ints(calls)
	return calls
}

func addTriples(calls []int, counts map[[3]int]int) {
	for i := 0; i < len(calls); i++ {
		for j := i + 1; j < len(calls); j++ {
			for k := j + 1; k < len(calls); k++ {
				counts[[3]int{calls[i], calls[j], calls[k]}]++
			}
		}
	}
}

// MinedRuleCandidates converts violations to pattern candidates.
func MinedRuleCandidates(violations []RuleViolation, facts []*MiningFacts) []Candidate {
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

// BuildCallSets constructs CallSets from MiningFacts by extracting called
// function names from PDG Call nodes.
func BuildCallSets(facts []*MiningFacts) []CallSet {
	var out []CallSet
	for _, f := range facts {
		if calls := extractCalls(f); len(calls) > 0 {
			out = append(out, CallSet{FuncID: f.ID, Calls: calls})
		}
	}
	return out
}

// extractCalls returns the set of callee IDs from a function's PDG.
func extractCalls(f *MiningFacts) map[string]bool {
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
