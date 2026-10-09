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

// minCohortCallDensity is the minimum average calls per function for a
// cohort to be mined. Groups below this are boilerplate (getters,
// setters) with no call variance worth mining.
const minCohortCallDensity = 5

// defaultCohortThresholdMilli is the default WL similarity threshold for
// cohort detection, in thousandths. Lower than the paper's 900 clone
// threshold: cohorts are functions solving similar problems, not
// near-duplicate clones.
const defaultCohortThresholdMilli = 650

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

// MineSimilarityGroups runs CCGraph to find similarity cohorts at the
// given WL match threshold (in thousandths; 900 is the paper's clone
// threshold, 650 finds looser cohorts), then mines PR-Miner rules within
// each cohort of 10+ functions with sufficient call density.
func MineSimilarityGroups(facts []*FuncFacts, thresholdMilli uint32) []SimilarityGroupResult {
	pdgs, names := ccGraphInputs(facts)
	groups := CCGraphCohorts(pdgs, names, thresholdMilli)
	byID := factsByID(facts)

	var out []SimilarityGroupResult
	for _, group := range groups {
		if len(group) < minGroupMineSize {
			continue
		}
		if result := mineOneGroup(group, names, byID, thresholdMilli); result != nil {
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

// mineOneGroup extracts call sets for a similarity cohort, skips it when
// call density is too low (boilerplate), mines rules, and finds
// violations. Returns nil if no rules were found.
func mineOneGroup(group []string, names map[string]string, byID map[string]*FuncFacts, thresholdMilli uint32) *SimilarityGroupResult {
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
	if !meetsCallDensity(sets) {
		return nil
	}
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

// meetsCallDensity reports whether a cohort's call sets average at least
// minCohortCallDensity calls per function. Trivial getters and setters
// fall below this and carry no minable variance.
func meetsCallDensity(sets []CallSet) bool {
	if len(sets) == 0 {
		return false
	}
	total := 0
	for _, s := range sets {
		total += len(s.Calls)
	}
	return total/len(sets) >= minCohortCallDensity
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
// Each violation becomes a candidate with cohort context. thresholdMilli
// is the WL match threshold used for cohort detection.
func minedRuleGroupCandidates(prepared []*FuncFacts, thresholdMilli uint32) []Candidate {
	byID := factsByID(prepared)
	var out []Candidate
	for _, result := range MineSimilarityGroups(prepared, thresholdMilli) {
		out = append(out, groupCandidates(result, byID, thresholdMilli)...)
	}
	return out
}

// groupNoun returns "cohort" for looser-than-clone thresholds and
// "similarity group" for the paper's clone threshold.
func groupNoun(thresholdMilli uint32) string {
	if thresholdMilli < ccMatchThreshold {
		return "cohort"
	}
	return "similarity group"
}

// groupCandidates converts one cohort's violations to candidates with
// cohort context in the inference text.
func groupCandidates(result SimilarityGroupResult, byID map[string]*FuncFacts, thresholdMilli uint32) []Candidate {
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
				"in %s of %d functions (e.g., %s): %.0f%% of functions that call %s also call `%s`",
				groupNoun(thresholdMilli),
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
