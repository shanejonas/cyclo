package patterns

import "fmt"

// Enum-dispatch proposals: single-function findings from the extractor's
// AST-level value-switch detection. Unlike the mined candidates, these
// need no clustering: one function, one switch, fixed score.

// enumDispatchScoreMilli is the fixed score for enum-dispatch proposals.
// They are real, actionable simplifications, but they carry no cluster
// support signal, so they rank below strong structural parallels.
const enumDispatchScoreMilli = 500

// enumDispatchCandidates proposes a dispatch table for every enum-value
// switch the extractor found. Each candidate names the containing function
// as its site (full body range, so range-based filters like --changed
// work); the switch line is in the observation and the FixSpec for
// precise location.
func enumDispatchCandidates(facts []*FuncFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		for _, hit := range f.EnumDispatches {
			f := f
			hit := hit
			out = append(out, Candidate{
				Kind:       EnumDispatch,
				ScoreMilli: enumDispatchScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation:      fmt.Sprintf("the switch at line %d dispatches on an enum value with %d cases calling zero-arg functions", hit.Line, hit.NumCases),
				Inference:        "each case calls a different function selected by the enum value",
				PossibleRefactor: "replace the switch with a map[EnumType]func() dispatch table and a single lookup",
				Sites: []Site{{
					Path:    f.Path,
					Line:    f.Line,
					EndLine: f.EndLine,
					ID:      f.ID,
					Name:    f.Name,
				}},
				FixSpec: &FixSpec{
					Kind:    EnumDispatch,
					File:    f.Path,
					Line:    hit.Line,
					EndLine: hit.Line,
					Params: map[string]string{
						"switch_line": fmt.Sprintf("%d", hit.Line),
					},
				},
			})
		}
	}
	return out
}
