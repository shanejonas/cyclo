package patterns

import (
	"fmt"
	"strings"
)

// Anemic-model proposals: DDD-inspired detection of the anemic domain
// model anti-pattern. A struct with data but no behavior, while external
// functions do all the work on it, wants its behavior moved into methods.
//
// Unlike the mined candidates, these need no PDG clustering: the signal
// is type-level (struct definitions vs method sets vs field-accessing
// functions), so the detection is a direct pass over the extractor's
// AnemicModelHits. Fixed score, like guard clauses and value objects.

// anemicModelScoreMilli is the fixed score for anemic-model proposals.
// They are design suggestions with more judgment involved than value
// objects, so they rank below both guard clauses and value objects.
const anemicModelScoreMilli = 400

// minAnemicFuncs is the minimum functions operating on a methodless
// struct for it to count as anemic.
const minAnemicFuncs = 3

// AnemicModelHit is one methodless struct with functions operating on
// its fields. Produced by the extractor's AST-level type analysis
// (Go-specific extension, not in rstyle's FnFacts).
type AnemicModelHit struct {
	// TypeName is the struct type name, e.g. "Order".
	TypeName string
	// Path is the source file with the struct definition.
	Path string
	// Line is the struct definition's first line.
	Line int
	// EndLine is the struct definition's last line.
	EndLine int
	// Funcs are the names of functions that take the struct as a
	// parameter and access its fields.
	Funcs []string
}

// MissingIdentityHit is a struct used as an entity (stored, compared,
// passed around) that has no identity field.
type MissingIdentityHit struct {
	// TypeName is the struct type name.
	TypeName string
	// Path is the source file with the struct definition.
	Path string
	// Line is the struct definition's first line.
	Line int
	// EndLine is the struct definition's last line.
	EndLine int
	// UseCount is how many entity-like uses were found.
	UseCount int
}

// anemicModelCandidates proposes moving behavior into methods for every
// anemic struct the extractor found. Each candidate names the struct
// definition as its site (full range, so --changed filtering works);
// the operating functions are listed in the refactor text.
//
// serviceHits are domain-service functions to suppress: a function
// operating on two or more types cannot be a method on just one, so the
// anemic suggestion does not apply to it. Filtering may drop a struct
// below the threshold, removing the candidate entirely.
func anemicModelCandidates(hits []AnemicModelHit, serviceHits []DomainServiceHit) []Candidate {
	services := serviceFuncNames(serviceHits)
	var out []Candidate
	for _, hit := range hits {
		hit := hit
		funcs := filterServiceFuncs(hit.Funcs, services)
		if len(funcs) < minAnemicFuncs {
			continue
		}
		out = append(out, Candidate{
			Kind:       AnemicModel,
			ScoreMilli: anemicModelScoreMilli,
			Breakdown: Breakdown{
				Support:       len(funcs),
				CoverageMilli: 1000,
			},
			Observation:      fmt.Sprintf("type %s has no methods, but %d functions operate on its fields", hit.TypeName, len(funcs)),
			Inference:        "behavior lives outside the type — this is an anemic domain model",
			PossibleRefactor: fmt.Sprintf("move %s into methods on *%s", strings.Join(funcs, ", "), hit.TypeName),
			Sites: []Site{{
				Path:    hit.Path,
				Line:    hit.Line,
				EndLine: hit.EndLine,
				ID:      hit.TypeName,
				Name:    hit.TypeName,
			}},
			FixSpec: &FixSpec{
				Kind:    AnemicModel,
				File:    hit.Path,
				Line:    hit.Line,
				EndLine: hit.EndLine,
				Params: map[string]string{
					"type_name": hit.TypeName,
				},
			},
		})
	}
	return out
}
