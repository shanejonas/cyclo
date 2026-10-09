package patterns

import (
	"fmt"
	"sort"
	"strings"
)

// Value-object proposals: DDD-inspired detection of data clumps. When the
// same primitive params travel together across functions, they describe a
// single domain concept that wants to be an immutable value object.
//
// Unlike the mined candidates, these need no PDG clustering: the signal is
// parameter co-occurrence, so the detection is a direct post-pass over
// FuncFacts. Fixed score, like guard clauses.

// valueObjectScoreMilli is the fixed score for value-object proposals.
// They are suggestions, not corrections, so they rank below guard clauses.
const valueObjectScoreMilli = 450

// minClumpSize is the minimum params in a data clump.
const minClumpSize = 2

// minClumpFuncs is the minimum functions sharing a clump.
const minClumpFuncs = 3

// paramKey joins a param's name and type for set operations.
func paramKey(p ParamInfo) string {
	return p.Name + ":" + p.Type
}

// clump is a set of param keys shared by a group of functions.
type clump struct {
	keys  map[string]bool
	facts map[*FuncFacts]bool
}

// valueObjectCandidates finds data clumps: groups of primitive params that
// appear together in at least minClumpFuncs functions. Each clump proposes
// one immutable value object.
func valueObjectCandidates(facts []*FuncFacts) []Candidate {
	fps := indexFuncParams(facts)
	clumps := findClumps(fps)
	clumps = maximalClumps(clumps)
	return buildValueObjectCandidates(clumps)
}

// funcParams pairs a function with its primitive param key set.
type funcParams struct {
	fact *FuncFacts
	keys map[string]bool
}

// indexFuncParams builds the param key set for each function with enough
// primitive params.
func indexFuncParams(facts []*FuncFacts) []funcParams {
	var fps []funcParams
	for _, f := range facts {
		if len(f.Params) < minClumpSize {
			continue
		}
		keys := make(map[string]bool, len(f.Params))
		for _, p := range f.Params {
			keys[paramKey(p)] = true
		}
		if len(keys) >= minClumpSize {
			fps = append(fps, funcParams{fact: f, keys: keys})
		}
	}
	return fps
}

// findClumps groups pairwise param intersections by key set, keeping those
// in enough functions.
func findClumps(fps []funcParams) []*clump {
	byKey := make(map[string]*clump)
	for i := 0; i < len(fps); i++ {
		for j := i + 1; j < len(fps); j++ {
			recordIntersection(byKey, fps[i], fps[j])
		}
	}
	var clumps []*clump
	for _, c := range byKey {
		if len(c.facts) >= minClumpFuncs {
			clumps = append(clumps, c)
		}
	}
	return clumps
}

// recordIntersection adds the param intersection of two functions to the
// clump index.
func recordIntersection(byKey map[string]*clump, a, b funcParams) {
	inter := intersectKeys(a.keys, b.keys)
	if len(inter) < minClumpSize {
		return
	}
	key := sortedKeys(inter)
	c, ok := byKey[key]
	if !ok {
		c = &clump{keys: inter, facts: make(map[*FuncFacts]bool)}
		byKey[key] = c
	}
	c.facts[a.fact] = true
	c.facts[b.fact] = true
}

// genericParamNames are placeholder names that carry no domain meaning.
// A data clump made entirely of these is not a domain concept — it's just
// convention (x, y for coordinates in a math helper; a, b for comparables).
var genericParamNames = map[string]bool{
	// Single letters.
	"a": true, "b": true, "c": true, "d": true, "e": true,
	"f": true, "g": true, "h": true, "i": true, "j": true,
	"k": true, "l": true, "m": true, "n": true, "o": true,
	"p": true, "q": true, "r": true, "s": true, "t": true,
	"u": true, "v": true, "w": true, "x": true, "y": true,
	"z": true,
	// Common placeholders.
	"foo": true, "bar": true, "baz": true, "qux": true,
	"tmp": true, "temp": true, "val": true, "var": true,
	"arg": true, "args": true, "param": true, "params": true,
	"obj": true, "item": true, "elem": true, "el": true,
	// Numbered placeholders.
	"p1": true, "p2": true, "p3": true,
	"a1": true, "a2": true, "a3": true,
	"v1": true, "v2": true,
	"x1": true, "x2": true, "y1": true, "y2": true,
}

// isGenericParamName reports whether a param name is a meaningless
// placeholder rather than a domain concept.
func isGenericParamName(name string) bool {
	return genericParamNames[strings.ToLower(name)]
}

// clumpKeyName extracts the param name from a "name:type" clump key.
func clumpKeyName(key string) string {
	if i := strings.Index(key, ":"); i >= 0 {
		return key[:i]
	}
	return key
}

// clumpHasMeaningfulName reports whether at least one param in the clump
// has a non-generic name. A clump of pure placeholders (x, y) is not a
// domain concept and should not propose a value object.
func clumpHasMeaningfulName(c *clump) bool {
	for k := range c.keys {
		if !isGenericParamName(clumpKeyName(k)) {
			return true
		}
	}
	return false
}

// buildValueObjectCandidates renders clumps as candidates, sorted.
func buildValueObjectCandidates(clumps []*clump) []Candidate {
	out := make([]Candidate, 0, len(clumps))
	for _, c := range clumps {
		if !clumpHasMeaningfulName(c) {
			continue
		}
		out = append(out, buildValueObjectCandidate(c))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Observation < out[j].Observation
	})
	return out
}

// intersectKeys returns the keys present in both sets.
func intersectKeys(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

// sortedKeys renders a key set as a sorted, comma-joined string.
func sortedKeys(keys map[string]bool) string {
	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	return strings.Join(list, ",")
}

// maximalClumps drops any clump that is a strict subset of another.
func maximalClumps(clumps []*clump) []*clump {
	var out []*clump
	for i, c := range clumps {
		maximal := true
		for j, other := range clumps {
			if i == j {
				continue
			}
			if isSubset(c.keys, other.keys) {
				maximal = false
				break
			}
		}
		if maximal {
			out = append(out, c)
		}
	}
	return out
}

// isSubset reports whether a is a strict subset of b.
func isSubset(a, b map[string]bool) bool {
	if len(a) >= len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// buildValueObjectCandidate renders one data clump as a candidate.
func buildValueObjectCandidate(c *clump) Candidate {
	// Collect and sort the param keys for stable output.
	keys := make([]string, 0, len(c.keys))
	for k := range c.keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the struct fields: "Amount int" from "amount:int".
	fields := make([]string, 0, len(keys))
	for _, k := range keys {
		parts := strings.SplitN(k, ":", 2)
		fields = append(fields, fmt.Sprintf("%s %s", capitalize(parts[0]), parts[1]))
	}

	// Collect sites, sorted by path and line.
	sites := make([]Site, 0, len(c.facts))
	for f := range c.facts {
		sites = append(sites, Site{
			Path:    f.Path,
			Line:    f.Line,
			EndLine: f.EndLine,
			ID:      f.ID,
			Name:    f.Name,
		})
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Path != sites[j].Path {
			return sites[i].Path < sites[j].Path
		}
		return sites[i].Line < sites[j].Line
	})

	group := strings.Join(keys, ", ")
	structDef := fmt.Sprintf("struct { %s }", strings.Join(fields, "; "))
	// Build FixSpec params: the param group as "name:type" pairs.
	params := map[string]string{
		"group": strings.Join(keys, ","),
	}
	return Candidate{
		Kind:       ValueObject,
		ScoreMilli: valueObjectScoreMilli,
		Breakdown: Breakdown{
			Support:       len(sites),
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("%d functions share the parameter group (%s)", len(sites), group),
		Inference:        "these primitives travel together — they describe a single domain concept",
		PossibleRefactor: fmt.Sprintf("extract an immutable value object: type <Name> %s", structDef),
		Sites:            sites,
		FixSpec: &FixSpec{
			Kind:    ValueObject,
			File:    sites[0].Path,
			Line:    sites[0].Line,
			EndLine: sites[0].EndLine,
			Params:  params,
		},
	}
}

// capitalize uppercases the first letter for exported Go field names.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
