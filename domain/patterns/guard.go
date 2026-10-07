package patterns

import "fmt"

// Guard-clause proposals: single-function findings from the extractor's
// AST-level inverted-conditional detection. Unlike the mined candidates,
// these need no clustering: one function, one site, fixed score.

// guardClauseScoreMilli is the fixed score for guard-clause proposals.
// They are real, actionable simplifications, but they carry no cluster
// support signal, so they rank below strong structural parallels.
const guardClauseScoreMilli = 500

// guardCandidates proposes a guard clause for every inverted conditional
// the extractor found. Each candidate names the containing function as its
// site (full body range, so range-based filters like --changed work); the
// if-statement line is in the observation for precise location.
func guardCandidates(facts []*FuncFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		for _, hit := range f.GuardClauses {
			f := f
			hit := hit
			out = append(out, Candidate{
				Kind:       GuardClause,
				ScoreMilli: guardClauseScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation:      fmt.Sprintf("the else branch at line %d is an early return while a %d-statement happy path is nested in the if body", hit.Line, hit.BodyStmts),
				Inference:        "the condition is inverted",
				PossibleRefactor: "invert the condition and return early, letting the happy path flow at top level",
				Sites: []Site{{
					Path:    f.Path,
					Line:    f.Line,
					EndLine: f.EndLine,
					ID:      f.ID,
					Name:    f.Name,
				}},
			})
		}
	}
	return out
}
