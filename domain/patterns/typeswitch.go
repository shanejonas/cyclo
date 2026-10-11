package patterns

import (
	"fmt"
	"strings"
)

// Type-switch proposals: single-function findings from the extractor's
// AST-level type-switch detection. Unlike the mined candidates, these need
// no clustering: one function, one site, fixed score.

// typeSwitchScoreMilli is the fixed score for type-switch proposals.
// They are real, actionable simplifications (a type switch calling the
// same method in every arm is an interface waiting to happen), but they
// carry no cluster support signal, so they rank below guard clauses.
const typeSwitchScoreMilli = 450

// typeSwitchCandidates proposes an interface method call for every type
// switch the extractor found whose arms all call the same method on the
// case-bound value. Each candidate names the containing function as its
// site (full body range, so range-based filters like --changed work); the
// switch-statement line is in the observation and the FixSpec for precise
// location.
func typeSwitchCandidates(facts []*MiningFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		for _, hit := range f.TypeSwitches {
			f := f
			hit := hit
			out = append(out, Candidate{
				Kind:       TypeSwitch,
				ScoreMilli: typeSwitchScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation: fmt.Sprintf("the type switch at line %d calls %s() in every arm (%s)",
					hit.Line, hit.Method, strings.Join(hit.Types, ", ")),
				Inference:        "every arm calls the same method on the case-bound value",
				PossibleRefactor: fmt.Sprintf("replace the switch with a direct method call on %s via an interface", hit.Expr),
				Sites: []Site{{
					Path:    f.Path,
					Line:    f.Line,
					EndLine: f.EndLine,
					ID:      f.ID,
					Name:    f.Name,
				}},
				FixSpec: &FixSpec{
					Kind:    TypeSwitch,
					File:    f.Path,
					Line:    hit.Line,
					EndLine: hit.Line,
					Params: map[string]string{
						"bound":  hit.Bound,
						"expr":   hit.Expr,
						"method": hit.Method,
						"types":  strings.Join(hit.Types, ","),
						"args":   hit.Args,
					},
				},
			})
		}
	}
	return out
}
