package patterns

import (
	"fmt"
)

// Group mining: CCGraph finds groups of semantically similar functions,
// then PR-Miner mines call-co-occurrence rules within each group. The
// differences between similar functions reveal the interesting patterns
// and violations — e.g., 19 HTTP handlers that all send a response and
// 1 that forgot to.
//
// This is the "reverse" of whole-repo mining: instead of diluting patterns
// across diverse code, we mine within coherent similarity groups where the
// base pattern is strong and violations stand out.

// minGroupMineSize is the minimum CCGraph group size for mining. Smaller
// groups don't have enough data for the 10% support threshold to be
// meaningful.
const minGroupMineSize = 10

// SimilarityGroupResult holds the PR-Miner results for one CCGraph similarity
// group.
type SimilarityGroupResult struct {
	// GroupIDs are the function IDs in this similarity group.
	GroupIDs []string
	// Representative is a sample function name for display.
	Representative string
	// Rules are the mined "if A then B" rules for this group.
	Rules []MinedRuleDef
	// Violations are functions in the group that break a rule.
	Violations []RuleViolation
}

// MineGroups runs CCGraph to find similarity groups, then mines PR-Miner
// rules within each group of 10+ functions.
func MineSimilarityGroups(facts []*FuncFacts) []SimilarityGroupResult {
	pdgs, names := ccGraphInputs(facts)
	groups := CCGraphClones(pdgs, names)
	byID := factsByID(facts)

	var out []SimilarityGroupResult
	for _, group := range groups {
		if len(group) < minGroupMineSize {
			continue
		}
		if result := mineOneGroup(group, names, byID); result != nil {
			out = append(out, *result)
		}
	}
	return out
}

// factsByID indexes facts by function ID for group lookup.
func factsByID(facts []*FuncFacts) map[string]*FuncFacts {
	byID := make(map[string]*FuncFacts, len(facts))
	for _, f := range facts {
		if f != nil {
			byID[f.ID] = f
		}
	}
	return byID
}

// mineOneGroup extracts call sets for a similarity group, mines rules,
// and finds violations. Returns nil if no rules were found.
func mineOneGroup(group []string, names map[string]string, byID map[string]*FuncFacts) *SimilarityGroupResult {
	facts := make([]*FuncFacts, 0, len(group))
	for _, id := range group {
		if f := byID[id]; f != nil {
			facts = append(facts, f)
		}
	}
	if len(facts) < minGroupMineSize {
		return nil
	}
	sets := BuildCallSets(facts)
	rules := MineRules(sets)
	if len(rules) == 0 {
		return nil
	}
	violations := FindViolations(sets, rules)
	return &SimilarityGroupResult{
		GroupIDs:       group,
		Representative: groupRepresentative(group, names),
		Rules:          rules,
		Violations:     violations,
	}
}

// groupRepresentative picks a display name for the group: the most common
// function name prefix, or the first name if no clear winner.
func groupRepresentative(group []string, names map[string]string) string {
	if len(group) == 0 {
		return ""
	}
	// Use the first function's name as the representative.
	if name := names[group[0]]; name != "" {
		return name
	}
	return group[0]
}

// minedRuleGroupCandidates converts group mining results to candidates.
// Each violation becomes a candidate with group context.
func minedRuleGroupCandidates(prepared []*FuncFacts) []Candidate {
	byID := factsByID(prepared)
	var out []Candidate
	for _, result := range MineSimilarityGroups(prepared) {
		out = append(out, groupCandidates(result, byID)...)
	}
	return out
}

// groupCandidates converts one group's violations to candidates with
// group context in the inference text.
func groupCandidates(result SimilarityGroupResult, byID map[string]*FuncFacts) []Candidate {
	var out []Candidate
	for _, v := range result.Violations {
		f := byID[v.FuncID]
		if f == nil {
			continue
		}
		ant := formatAntecedent(v.Rule.Antecedent)
		out = append(out, Candidate{
			Kind:        MinedRule,
			ScoreMilli:  750,
			Observation: fmt.Sprintf("calls %s but not `%s`", ant, v.Rule.Consequent),
			Inference: fmt.Sprintf(
				"in similarity group of %d functions (e.g., %s): %.0f%% of functions that call %s also call `%s`",
				len(result.GroupIDs),
				result.Representative,
				v.Rule.Confidence*100,
				ant,
				v.Rule.Consequent,
			),
			PossibleRefactor: fmt.Sprintf("add the missing `%s` call", v.Rule.Consequent),
			Sites: []Site{
				{Path: f.Path, Line: f.Line, Name: f.Name},
			},
		})
	}
	return out
}

// formatAntecedent formats rule antecedent calls as backtick-quoted,
// comma-separated names.
func formatAntecedent(antecedent []string) string {
	ant := ""
	for i, a := range antecedent {
		if i > 0 {
			ant += ", "
		}
		ant += "`" + a + "`"
	}
	return ant
}
