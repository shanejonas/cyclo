package quality

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

// Evaluate never mutates facts or config. It rejects conflicting duplicates
// instead of allowing package/test variant order to determine the result.
func Evaluate(facts []Function, config Config) (Report, error) {
	if err := config.Validate(); err != nil {
		return Report{}, err
	}
	functions, err := canonicalFacts(facts)
	if err != nil {
		return Report{}, err
	}
	results := []FunctionResult{}
	diagnostics := []Diagnostic{}
	sites := stackSites(functions)
	for idx, function := range functions {
		result, findings := evaluateFunction(function, config)
		planFindings(functions, sites, idx, findings)
		results = append(results, result)
		diagnostics = append(diagnostics, findings...)
	}
	slices.SortStableFunc(diagnostics, func(a, b Diagnostic) int {
		if order := compareLocation(a.Location, b.Location); order != 0 {
			return order
		}
		return cmp.Compare(a.RuleID, b.RuleID)
	})
	return Report{
		SchemaVersion: 1,
		Functions:     results,
		Diagnostics:   diagnostics,
		Summary:       summarize(results),
		FixGroups:     buildFixGroups(functions, diagnostics, config),
	}, nil
}

func canonicalFacts(facts []Function) ([]Function, error) {
	result := slices.Clone(facts)
	slices.SortStableFunc(result, func(a, b Function) int { return compareFunction(a.Location, b.Location) })
	unique := make([]Function, 0, len(result))
	for _, fact := range result {
		if err := validateFact(fact); err != nil {
			return nil, err
		}
		if len(unique) == 0 || compareFunction(unique[len(unique)-1].Location, fact.Location) != 0 {
			unique = append(unique, fact)
			continue
		}
		if !reflect.DeepEqual(unique[len(unique)-1], fact) {
			return nil, fmt.Errorf("conflicting facts for %s:%d:%d %s", fact.Path, fact.Line, fact.Column, fact.Name)
		}
	}
	return unique, nil
}

func compareLocation(a, b Location) int {
	if order := cmp.Compare(a.Path, b.Path); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Line, b.Line); order != 0 {
		return order
	}
	return cmp.Compare(a.Column, b.Column)
}

func compareFunction(a, b Location) int {
	if order := compareLocation(a, b); order != 0 {
		return order
	}
	return cmp.Compare(a.Name, b.Name)
}

// ruleCheck keeps a measurement, its policy, and supporting evidence together.
type ruleCheck struct {
	id        string
	actual    int64
	rule      Rule
	effects   []Effect
	mutations []Mutation
}

func evaluateFunction(f Function, c Config) (FunctionResult, []Diagnostic) {
	result := functionEffects(f, c)
	targets := mutationTargets(f.Mutations, c.Granularity)
	result.Mutations, result.MutatedTargets = len(f.Mutations), len(targets)
	allowed, suppressionError := suppression(f.PrecedingLine)
	diagnostics := []Diagnostic{}
	if suppressionError != "" {
		diagnostics = append(diagnostics, Diagnostic{Location: f.Location, RuleID: "invalid_suppression", Actual: 1, Limit: 0, Message: suppressionError})
	}
	checks := []ruleCheck{
		{id: "fn_length", actual: int64(f.CodeLines), rule: c.FnLength},
		{id: "fn_params", actual: parameterBudget(f, c), rule: c.FnParams},
		{id: "mutated_targets", actual: int64(len(targets)), rule: c.MutatedTargets},
	}
	for _, check := range checks {
		diagnostics = appendRule(diagnostics, f.Location, check, allowed)
	}
	for _, target := range targets {
		check := ruleCheck{id: "mutation_per_target", actual: int64(len(target)), rule: c.MutationPerTarget, mutations: target}
		diagnostics = appendRule(diagnostics, f.Location, check, allowed)
	}
	if f.Statements >= c.MinStatements {
		if diagnostic, ok := result.densityDiagnostic(int64(f.Statements), c.SideEffectDensity, c.Weights, allowed); ok {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	return result, diagnostics
}

func parameterBudget(f Function, c Config) int64 {
	if c.CountSelf && f.HasSelf {
		return int64(f.Params) + 1
	}
	return int64(f.Params)
}

func appendRule(ds []Diagnostic, loc Location, check ruleCheck, allowed []string) []Diagnostic {
	if !check.rule.Enabled || check.actual <= check.rule.Max || slices.Contains(allowed, check.id) {
		return ds
	}
	return append(ds, Diagnostic{
		Location: loc, RuleID: check.id, Actual: check.actual, Limit: check.rule.Max,
		Message: fmt.Sprintf("%s: %d exceeds %d", check.id, check.actual, check.rule.Max),
		Effects: check.effects, Mutations: check.mutations,
	})
}

// densityDiagnostic builds the side_effect_density finding for this result,
// or false when the rule does not fire. The message shows the finding's
// arithmetic so it explains itself instead of just naming a number.
func (r FunctionResult) densityDiagnostic(statements int64, rule Rule, weights Weights, allowed []string) (Diagnostic, bool) {
	if !rule.Enabled || r.DensityMilli <= rule.Max || slices.Contains(allowed, "side_effect_density") {
		return Diagnostic{}, false
	}
	breakdown := summarizeDensity(r.Effects, weights)
	return Diagnostic{
		Location: r.Location, RuleID: "side_effect_density",
		Actual: r.DensityMilli, Limit: rule.Max,
		Message:     breakdown.message(r.DensityMilli, rule.Max, statements),
		Effects:     r.Effects,
		Weight:      breakdown.weight,
		Statements:  statements,
		KindWeights: breakdown.weights(),
	}, true
}

type kindWeight struct {
	kind   Kind
	weight int64
}

// densitySummary totals effect weight by kind for one finding.
type densitySummary struct {
	weight int64
	kinds  []kindWeight
}

// summarizeDensity totals effect weight by kind, heaviest first.
func summarizeDensity(effects []Effect, weights Weights) densitySummary {
	byKind := map[Kind]int64{}
	var total int64
	for _, effect := range effects {
		w := weights.value(effect.Kind)
		if w == 0 {
			continue
		}
		byKind[effect.Kind] += w
		total += w
	}
	kinds := make([]kindWeight, 0, len(byKind))
	for kind, w := range byKind {
		kinds = append(kinds, kindWeight{kind, w})
	}
	slices.SortFunc(kinds, func(a, b kindWeight) int {
		if a.weight != b.weight {
			return cmp.Compare(b.weight, a.weight)
		}
		return cmp.Compare(a.kind, b.kind)
	})
	return densitySummary{weight: total, kinds: kinds}
}

func (s densitySummary) message(actual, limit, statements int64) string {
	parts := make([]string, len(s.kinds))
	for i, kw := range s.kinds {
		parts[i] = fmt.Sprintf("%s %d", kw.kind, kw.weight)
	}
	return fmt.Sprintf("side_effect_density %d > %d: weight %d over %d statements (%s)",
		actual, limit, s.weight, statements, strings.Join(parts, ", "))
}

func (s densitySummary) weights() map[string]int64 {
	weights := make(map[string]int64, len(s.kinds))
	for _, kw := range s.kinds {
		weights[string(kw.kind)] = kw.weight
	}
	return weights
}

func mutationTargets(mutations []Mutation, granularity string) [][]Mutation {
	groups := make(map[string][]Mutation)
	for _, mutation := range mutations {
		key := mutation.Root + "@" + mutation.RootID
		if granularity == "field" {
			key += "." + mutation.FieldPath
		}
		groups[key] = append(groups[key], mutation)
	}
	result := make([][]Mutation, len(groups))
	for index, key := range slices.Sorted(maps.Keys(groups)) {
		result[index] = groups[key]
	}
	return result
}

func functionEffects(f Function, c Config) FunctionResult {
	effects := append([]Effect{}, f.Effects...)
	effects = append(effects, mutationEffects(f.Mutations)...)
	calls, unclassified := localCalls(f, c.Prefixes)
	effects = append(effects, calls...)
	slices.SortStableFunc(effects, compareEffects)
	weight, complete := effectWeight(effects, c.Weights)
	return FunctionResult{
		Location:          f.Location,
		Effects:           effects,
		UnclassifiedCalls: unclassified,
		Complete:          complete,
		DensityMilli:      1000 * weight / int64(max(f.Statements, 1)),
	}
}

func mutationEffects(mutations []Mutation) []Effect {
	effects := []Effect{}
	for _, mutation := range mutations {
		if mutation.Provenance == Local {
			continue
		}
		kind := MutationEffect
		if mutation.Provenance == Unknown {
			kind = UnknownEffect
		}
		label := mutation.Root
		if mutation.FieldPath != "" {
			label += "." + mutation.FieldPath
		}
		effects = append(effects, Effect{Kind: kind, Detail: label, Line: mutation.Line})
	}
	return effects
}

func compareEffects(a, b Effect) int {
	if order := cmp.Compare(a.Line, b.Line); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Kind, b.Kind); order != 0 {
		return order
	}
	return cmp.Compare(a.Detail, b.Detail)
}

func effectWeight(effects []Effect, weights Weights) (int64, bool) {
	var weight int64
	complete := true
	for _, effect := range effects {
		weight += weights.value(effect.Kind)
		if effect.Kind == UnknownEffect {
			complete = false
		}
	}
	return weight, complete
}

// NormalizeCallee removes balanced Go or Rust generic arguments.
func NormalizeCallee(callee string) string {
	if strings.ContainsAny(callee, "[]<>") || !utf8.ValidString(callee) {
		callee = withoutGenericArguments(callee)
	}
	return strings.ReplaceAll(callee, "::::", "::")
}

func withoutGenericArguments(callee string) string {
	var result strings.Builder
	depth := 0
	for _, char := range callee {
		switch char {
		case '[', '<':
			depth++
		case ']', '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				result.WriteRune(char)
			}
		}
	}
	return result.String()
}

func summarize(functions []FunctionResult) Summary {
	// Summing raw densities can overflow for large hand-built facts. Divide
	// first, retaining remainders so the mean still uses exact integer math.
	count := int64(max(len(functions), 1))
	var quotient, remainder, maxDensity int64
	incomplete := 0
	for _, function := range functions {
		quotient += function.DensityMilli / count
		remainder += function.DensityMilli % count
		maxDensity = max(maxDensity, function.DensityMilli)
		if !function.Complete {
			incomplete++
		}
	}
	return Summary{
		Functions: len(functions), Effects: effectCounts(functions),
		IncompleteFunctions: incomplete, MaxDensityMilli: maxDensity,
		MeanDensityMilli: quotient + remainder/count,
	}
}

func effectCounts(functions []FunctionResult) []EffectCount {
	counts := make(map[Kind]int, len(kinds))
	for _, function := range functions {
		for _, effect := range function.Effects {
			counts[effect.Kind]++
		}
	}
	result := make([]EffectCount, len(kinds))
	for index, kind := range kinds {
		result[index] = EffectCount{kind, counts[kind]}
	}
	return result
}
