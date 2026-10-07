package patterns

import (
	"fmt"
	"sort"
	"strings"
)

// DDD structural patterns: aggregates, repositories, factories (Evans).
// Aggregates and repositories are detection-only: choosing an aggregate
// root or extracting a repository is design judgment. Factories have a
// mechanical transform.

// aggregateScoreMilli is the fixed score for co-modified entity types.
// Detection-only: the aggregate root is a design decision.
const aggregateScoreMilli = 400

// repositoryScoreMilli is the fixed score for db calls in business logic.
// Detection-only: moving to a repository is design.
const repositoryScoreMilli = 400

// factoryScoreMilli is the fixed score for complex struct literals built
// in multiple functions. The factory extraction is mechanical.
const factoryScoreMilli = 450

// factoryMinFields is the minimum struct-literal field count to propose a
// factory. Must match the extractor's threshold.
const factoryMinFields = 5

// factoryMaxFields caps the struct field count: a factory with more params
// would violate the fn_params quality gate, so don't propose it. With the
// min at 5 this keeps the pattern dormant until param bundling is designed.
const factoryMaxFields = 4

// aggregateCandidates finds pairs of struct types mutated together in 2+
// functions. Such types likely belong under one aggregate root.
func aggregateCandidates(facts []*FuncFacts) []Candidate {
	pairFuncs := coModifiedPairs(facts)
	var out []Candidate
	for k, funcs := range pairFuncs {
		if len(funcs) < 2 {
			continue
		}
		out = append(out, aggregateCandidate(k, facts, funcs))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Breakdown.Support != out[j].Breakdown.Support {
			return out[i].Breakdown.Support > out[j].Breakdown.Support
		}
		return out[i].Observation < out[j].Observation
	})
	return out
}

// coModifiedPairs maps each co-modified type pair to the functions that
// modify both types.
func coModifiedPairs(facts []*FuncFacts) map[[2]string]map[string]bool {
	seen := map[[2]string]map[string]bool{}
	for _, f := range facts {
		types := distinctTypes(f.AggregateMods)
		for i := 0; i < len(types); i++ {
			for j := i + 1; j < len(types); j++ {
				k := [2]string{types[i], types[j]}
				if seen[k] == nil {
					seen[k] = map[string]bool{}
				}
				seen[k][f.ID] = true
			}
		}
	}
	return seen
}

// aggregateCandidate builds the detection-only candidate for a type pair.
func aggregateCandidate(k [2]string, facts []*FuncFacts, funcs map[string]bool) Candidate {
	names := make([]string, 0, len(funcs))
	for id := range funcs {
		names = append(names, id)
	}
	sort.Strings(names)
	return Candidate{
		Kind:       Aggregate,
		ScoreMilli: aggregateScoreMilli,
		Breakdown: Breakdown{
			Support:       len(funcs),
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("%s and %s are modified together in %d functions", k[0], k[1], len(funcs)),
		Inference:        "types changed in the same transaction likely belong to one aggregate",
		PossibleRefactor: fmt.Sprintf("consider making %s the aggregate root for %s", k[0], k[1]),
		Sites:            candidateSites(facts, names),
		// No FixSpec: choosing the aggregate root is design judgment.
		FixSpec: nil,
	}
}

// distinctTypes returns the sorted unique type names from mod hits.
func distinctTypes(hits []AggregateModHit) []string {
	set := map[string]bool{}
	for _, h := range hits {
		if h.TypeName != "" {
			set[h.TypeName] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// candidateSites builds Sites for the named function IDs.
func candidateSites(facts []*FuncFacts, ids []string) []Site {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Site
	for _, f := range facts {
		if want[f.ID] {
			out = append(out, Site{
				Path:    f.Path,
				Line:    f.Line,
				EndLine: f.EndLine,
				ID:      f.ID,
				Name:    f.Name,
			})
		}
	}
	return out
}

// repositoryCandidates flags functions making direct db calls outside
// repository files. One candidate per function.
func repositoryCandidates(facts []*FuncFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		if len(f.DbCalls) == 0 {
			continue
		}
		calls := make([]string, 0, len(f.DbCalls))
		seen := map[string]bool{}
		for _, d := range f.DbCalls {
			if !seen[d.Call] {
				seen[d.Call] = true
				calls = append(calls, d.Call)
			}
		}
		sort.Strings(calls)
		out = append(out, Candidate{
			Kind:       Repository,
			ScoreMilli: repositoryScoreMilli,
			Breakdown: Breakdown{
				Support:       len(f.DbCalls),
				CoverageMilli: 1000,
			},
			Observation:      fmt.Sprintf("%s makes %d direct db call(s): %s", f.Name, len(f.DbCalls), strings.Join(calls, ", ")),
			Inference:        "database access in business logic bypasses the repository boundary",
			PossibleRefactor: "move db access behind a Repository interface",
			Sites: []Site{{
				Path:    f.Path,
				Line:    f.Line,
				EndLine: f.EndLine,
				ID:      f.ID,
				Name:    f.Name,
			}},
			// No FixSpec: repository extraction is design.
			FixSpec: nil,
		})
	}
	return out
}

// factoryCandidates finds struct types with complex literals (5+ fields)
// built with construction logic in 2+ functions. Plain field assignment
// doesn't qualify — the factory must encapsulate validation, defaults,
// or error handling to be worth the indirection.
func factoryCandidates(facts []*FuncFacts) []Candidate {
	byType, firstLine := groupFactoryLits(facts)
	var out []Candidate
	for k, funcs := range byType {
		if len(funcs) < 2 {
			continue
		}
		out = append(out, factoryCandidate(k, facts, funcs, firstLine[k]))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Breakdown.Support != out[j].Breakdown.Support {
			return out[i].Breakdown.Support > out[j].Breakdown.Support
		}
		return out[i].Observation < out[j].Observation
	})
	return out
}

// factoryTypeKey identifies a struct type by name and declaration file.
type factoryTypeKey struct {
	name string
	file string
}

// groupFactoryLits groups factory hits by struct type, tracking the
// functions that build each type and the first literal line. Only hits
// with construction logic are counted — plain field assignment is skipped.
func groupFactoryLits(facts []*FuncFacts) (map[factoryTypeKey]map[string]bool, map[factoryTypeKey]int) {
	byType := map[factoryTypeKey]map[string]bool{}
	firstLine := map[factoryTypeKey]int{}
	for _, f := range facts {
		for _, h := range f.FactoryLits {
			if !factoryHitQualifies(h) {
				continue
			}
			k := factoryTypeKey{h.TypeName, h.DeclFile}
			if byType[k] == nil {
				byType[k] = map[string]bool{}
				firstLine[k] = h.Line
			}
			byType[k][f.ID] = true
		}
	}
	return byType, firstLine
}

// factoryHitQualifies reports whether a factory hit has the field count
// and construction logic to warrant a factory. Structs with more than 4
// fields are skipped: the factory would need more than 4 params, violating
// the fn_params quality gate.
func factoryHitQualifies(h FactoryHit) bool {
	return h.TypeName != "" &&
		h.NumFields >= factoryMinFields &&
		h.NumFields <= factoryMaxFields &&
		h.HasLogic
}

// factoryCandidate builds the fixable candidate for a struct type.
func factoryCandidate(k factoryTypeKey, facts []*FuncFacts, funcs map[string]bool, line int) Candidate {
	names := make([]string, 0, len(funcs))
	for id := range funcs {
		names = append(names, id)
	}
	sort.Strings(names)
	return Candidate{
		Kind:       Factory,
		ScoreMilli: factoryScoreMilli,
		Breakdown: Breakdown{
			Support:       len(funcs),
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("type %s is built with %d+ fields and construction logic in %d functions", k.name, factoryMinFields, len(funcs)),
		Inference:        "complex construction with validation/defaults scattered across callers wants a factory",
		PossibleRefactor: fmt.Sprintf("extract a New%s factory function", k.name),
		Sites:            candidateSites(facts, names),
		FixSpec: &FixSpec{
			Kind:    Factory,
			File:    k.file,
			Line:    line,
			EndLine: line,
			Params: map[string]string{
				"type": k.name,
			},
		},
	}
}

// specificationScoreMilli is the fixed score for repeated business rules.
// The Specification extraction is mechanical.
const specificationScoreMilli = 450

// specificationCandidates finds boolean business rules (2+ conditions)
// repeated in 2+ functions. Such rules want a Specification type with
// IsSatisfiedBy (Evans).
func specificationCandidates(facts []*FuncFacts) []Candidate {
	byRule, first := groupSpecRules(facts)
	var out []Candidate
	for k, funcs := range byRule {
		if len(funcs) < 2 {
			continue
		}
		out = append(out, specificationCandidate(k, facts, funcs, first[k]))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Breakdown.Support != out[j].Breakdown.Support {
			return out[i].Breakdown.Support > out[j].Breakdown.Support
		}
		return out[i].Observation < out[j].Observation
	})
	return out
}

// specRuleKey identifies a business rule by its normalized key.
type specRuleKey struct {
	key      string
	typeName string
}

// groupSpecRules groups specification hits by rule key, tracking the
// functions containing each rule and the first hit for fixer context.
func groupSpecRules(facts []*FuncFacts) (map[specRuleKey]map[string]bool, map[specRuleKey]SpecificationHit) {
	byRule := map[specRuleKey]map[string]bool{}
	first := map[specRuleKey]SpecificationHit{}
	for _, f := range facts {
		for _, h := range f.SpecRules {
			if h.RuleKey == "" || h.TypeName == "" {
				continue
			}
			k := specRuleKey{h.RuleKey, h.TypeName}
			if byRule[k] == nil {
				byRule[k] = map[string]bool{}
				first[k] = h
			}
			byRule[k][f.ID] = true
		}
	}
	return byRule, first
}

// specificationCandidate builds the fixable candidate for a business rule.
func specificationCandidate(k specRuleKey, facts []*FuncFacts, funcs map[string]bool, hit SpecificationHit) Candidate {
	names := make([]string, 0, len(funcs))
	for id := range funcs {
		names = append(names, id)
	}
	sort.Strings(names)
	specName := k.typeName + "Specification"
	return Candidate{
		Kind:       Specification,
		ScoreMilli: specificationScoreMilli,
		Breakdown: Breakdown{
			Support:       len(funcs),
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("business rule on %s repeated in %d functions", k.typeName, len(funcs)),
		Inference:        "a boolean business rule scattered across callers wants a Specification",
		PossibleRefactor: fmt.Sprintf("extract a %s with IsSatisfiedBy", specName),
		Sites:            candidateSites(facts, names),
		FixSpec: &FixSpec{
			Kind:    Specification,
			File:    "",
			Line:    hit.Line,
			EndLine: hit.Line,
			Params: map[string]string{
				"type":     k.typeName,
				"varname":  hit.VarName,
				"cond":     hit.CondText,
				"rulekey":  k.key,
			},
		},
	}
}
