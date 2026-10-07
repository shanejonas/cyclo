package patterns

// Guard-clause proposals: single-function findings from the extractor's
// AST-level inverted-conditional detection. Unlike the mined candidates,
// these need no clustering: one function, one site, fixed score.

// guardClauseScoreMilli is the fixed score for guard-clause proposals.
// They are real, actionable simplifications, but they carry no cluster
// support signal, so they rank below strong structural parallels.
const guardClauseScoreMilli = 500

// guardCandidates proposes a guard clause for every inverted conditional
// the extractor found. Each candidate names the if-statement line as its
// site; the site's EndLine is the function's closing line so range-based
// filters (e.g. --changed) treat the whole function body as relevant.
func guardCandidates(facts []*FuncFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		for _, line := range f.GuardClauses {
			f := f
			out = append(out, Candidate{
				Kind:       GuardClause,
				ScoreMilli: guardClauseScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation:      "the else branch is an early return while the happy path is nested in the if body",
				Inference:        "the condition is inverted",
				PossibleRefactor: "invert the condition and return early, letting the happy path flow at top level",
				Sites: []Site{{
					Path:    f.Path,
					Line:    line,
					EndLine: f.EndLine,
					ID:      f.ID,
					Name:    f.Name,
				}},
			})
		}
	}
	return out
}
