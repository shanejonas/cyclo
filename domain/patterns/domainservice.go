package patterns

import (
	"fmt"
	"strings"
)

// Domain-service proposals: DDD-inspired detection of legitimate Domain
// Services (Evans). A free function operating on two or more domain types
// without belonging to any of them is stateless coordination, not
// misplaced behavior: it wants to be a Service, not a method.
//
// This complements anemic_model: the anemic detector flags functions
// operating on a methodless struct's fields and suggests moving them
// into methods. But a function touching two types cannot be a method on
// just one. Domain-service detection runs first so those functions are
// suppressed from the anemic suggestions instead of misreported.

// domainServiceScoreMilli is the fixed score for domain-service
// proposals. They are design suggestions with judgment involved, so they
// rank below anemic models.
const domainServiceScoreMilli = 350

// DomainServiceHit is one free function operating on the fields of two
// or more distinct struct types. Produced by the extractor's AST-level
// type analysis.
type DomainServiceHit struct {
	// FuncName is the function name, e.g. "Transfer".
	FuncName string
	// Path is the source file with the function definition.
	Path string
	// Line is the function's first line.
	Line int
	// EndLine is the function's last line.
	EndLine int
	// Types are the distinct struct type names whose fields the
	// function accesses, e.g. ["Account", "Money"].
	Types []string
}

// domainServiceCandidates proposes a Domain Service for every function
// the extractor found operating on multiple domain types. Each candidate
// names the function as its site; the involved types are listed in the
// refactor text. Detection-only: choosing the service name and home is
// design, so FixSpec is nil.
func domainServiceCandidates(hits []DomainServiceHit) []Candidate {
	var out []Candidate
	for _, hit := range hits {
		hit := hit
		if len(hit.Types) < 2 {
			continue
		}
		out = append(out, Candidate{
			Kind:       DomainService,
			ScoreMilli: domainServiceScoreMilli,
			Breakdown: Breakdown{
				Support:       len(hit.Types),
				CoverageMilli: 1000,
			},
			Observation: fmt.Sprintf("func %s operates on %d domain types (%s) without belonging to any of them",
				hit.FuncName, len(hit.Types), strings.Join(hit.Types, ", ")),
			Inference:        "stateless coordination between domain types is a domain service (Evans), not misplaced behavior",
			PossibleRefactor: fmt.Sprintf("extract %s into a %sService type; do not move it into a method on %s", hit.FuncName, hit.FuncName, hit.Types[0]),
			Sites: []Site{{
				Path:    hit.Path,
				Line:    hit.Line,
				EndLine: hit.EndLine,
				ID:      hit.FuncName,
				Name:    hit.FuncName,
			}},
			FixSpec: nil,
		})
	}
	return out
}

// serviceFuncNames returns the set of function names identified as
// domain services, for suppressing anemic-model false positives.
func serviceFuncNames(hits []DomainServiceHit) map[string]bool {
	out := make(map[string]bool, len(hits))
	for _, h := range hits {
		out[h.FuncName] = true
	}
	return out
}

// filterServiceFuncs removes domain-service functions from an anemic
// hit's func list. A function operating on two or more types cannot be
// a method on just one, so the anemic "move into a method" suggestion
// does not apply to it.
func filterServiceFuncs(funcs []string, services map[string]bool) []string {
	if len(services) == 0 {
		return funcs
	}
	out := make([]string, 0, len(funcs))
	for _, f := range funcs {
		if !services[f] {
			out = append(out, f)
		}
	}
	return out
}
