package patterns

import (
	"fmt"
	"sort"
	"strings"
)

// Primitive-obsession proposals: DDD-inspired detection of domain concepts
// hiding behind raw string/int params. When the same concept (e.g. "email")
// appears as a raw string param across functions, the type system can't help;
// a named type (type Email string) documents the domain and prevents mixups.
//
// Unlike value objects (which need co-occurring param groups), primitive
// obsession is a single-param signal: one concept, one type, 2+ functions.
// Fixed score, below value objects (more heuristic).

// primitiveObsessionScoreMilli is the fixed score for primitive-obsession
// proposals. They are design suggestions with heuristic concept matching,
// so they rank below the more structural findings.
const primitiveObsessionScoreMilli = 350

// minPrimitiveFuncs is the minimum functions sharing a concept.
const minPrimitiveFuncs = 2

// primitiveConceptKeywords are domain-concept words that, when found as a
// whole word in a string/int param name, suggest the param is a domain
// concept hiding behind a primitive. Matching is on camelCase/snake_case
// word boundaries: "userId" matches "id", but "valid" does not.
var primitiveConceptKeywords = []string{
	"email",
	"phone",
	"url",
	"uri",
	"uuid",
	"guid",
	"id",
	"name",
	"address",
	"street",
	"city",
	"state",
	"zip",
	"postal",
	"country",
	"currency",
	"price",
	"amount",
	"quantity",
	"ssn",
	"passport",
	"license",
	"username",
	"password",
	"token",
	"apikey",
	"secret",
	"creditcard",
	"iban",
	"isbn",
	"sku",
	"serial",
}

// splitWords splits a param name into lowercase words on camelCase and
// snake_case/kebab boundaries: "userEmail" -> ["user", "email"].
func splitWords(name string) []string {
	var words []string
	var cur strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r == '_' || r == '-' {
			words = appendWord(words, &cur)
			continue
		}
		if isCamelBoundary(runes, i) {
			words = appendWord(words, &cur)
		}
		cur.WriteRune(toLower(r))
	}
	return appendWord(words, &cur)
}

// appendWord appends the builder's content if non-empty.
func appendWord(words []string, cur *strings.Builder) []string {
	if cur.Len() > 0 {
		words = append(words, cur.String())
		cur.Reset()
	}
	return words
}

// isCamelBoundary reports a lowercase/digit -> uppercase transition.
func isCamelBoundary(runes []rune, i int) bool {
	if i == 0 {
		return false
	}
	return isUpper(runes[i]) && (isLower(runes[i-1]) || isDigit(runes[i-1]))
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }
func toLower(r rune) rune {
	if isUpper(r) {
		return r + ('a' - 'A')
	}
	return r
}

// primitiveConcept extracts the domain concept from a param name, or "" if
// the name doesn't suggest one. Matching is whole-word: "userEmail" ->
// "email", "userId" -> "id", but "valid" -> "" (no "id" word).
func primitiveConcept(name string) string {
	words := splitWords(name)
	for _, w := range words {
		for _, kw := range primitiveConceptKeywords {
			if w == kw {
				return kw
			}
		}
	}
	return ""
}

// primitiveOccurrence is one function using a concept as a raw primitive.
type primitiveOccurrence struct {
	fact    *FuncFacts
	param   string
	concept string
	typ     string
}

// primitiveObsessionCandidates finds domain concepts used as raw string/int
// params in at least minPrimitiveFuncs functions. Each concept proposes one
// named type.
func primitiveObsessionCandidates(facts []*FuncFacts) []Candidate {
	byConcept := groupOccurrences(facts)
	return buildCandidates(byConcept)
}

// groupOccurrences groups concept occurrences by (concept, type).
func groupOccurrences(facts []*FuncFacts) map[string][]primitiveOccurrence {
	byConcept := make(map[string][]primitiveOccurrence)
	for _, f := range facts {
		if f == nil {
			continue
		}
		for key, occ := range factOccurrences(f) {
			byConcept[key] = append(byConcept[key], occ)
		}
	}
	return byConcept
}

// factOccurrences returns one occurrence per (concept, type) in a function.
func factOccurrences(f *FuncFacts) map[string]primitiveOccurrence {
	out := make(map[string]primitiveOccurrence)
	for _, p := range f.Params {
		if occ, ok := paramOccurrence(f, p); ok {
			key := occ.concept + ":" + occ.typ
			if _, seen := out[key]; !seen {
				out[key] = occ
			}
		}
	}
	return out
}

// paramOccurrence builds the occurrence for a param, or false if the param
// isn't a primitive with a domain concept.
func paramOccurrence(f *FuncFacts, p ParamInfo) (primitiveOccurrence, bool) {
	var zero primitiveOccurrence
	if p.Type != "string" && p.Type != "int" {
		return zero, false
	}
	concept := primitiveConcept(p.Name)
	if concept == "" {
		return zero, false
	}
	return primitiveOccurrence{fact: f, param: p.Name, concept: concept, typ: p.Type}, true
}

// buildCandidates renders candidates for concepts in enough functions.
func buildCandidates(byConcept map[string][]primitiveOccurrence) []Candidate {
	var out []Candidate
	for key, occs := range byConcept {
		if len(occs) < minPrimitiveFuncs {
			continue
		}
		out = append(out, buildPrimitiveObsessionCandidate(key, occs))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Observation < out[j].Observation
	})
	return out
}

// primitiveInitialisms maps concepts to their Go initialism form.
var primitiveInitialisms = map[string]string{
	"id": "ID", "url": "URL", "uri": "URI", "uuid": "UUID", "guid": "GUID",
	"api": "API", "ssn": "SSN", "isbn": "ISBN", "iban": "IBAN", "sku": "SKU",
}

// primitiveTypeName renders the Go type name for a concept, honoring
// common initialisms: "id" -> "ID", "url" -> "URL".
func primitiveTypeName(concept string) string {
	if init, ok := primitiveInitialisms[concept]; ok {
		return init
	}
	return capitalize(concept)
}
func buildPrimitiveObsessionCandidate(key string, occs []primitiveOccurrence) Candidate {
	parts := strings.SplitN(key, ":", 2)
	concept, typ := parts[0], parts[1]
	typeName := primitiveTypeName(concept)

	// Collect sites, sorted by path and line. Dedupe functions (a function
	// appears once per concept by construction).
	sites := make([]Site, 0, len(occs))
	for _, o := range occs {
		f := o.fact
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

	// Show the param names that triggered, e.g. "'email', 'userEmail'".
	names := make([]string, 0, len(occs))
	seen := make(map[string]bool)
	for _, o := range occs {
		if !seen[o.param] {
			seen[o.param] = true
			names = append(names, "'"+o.param+"'")
		}
	}
	sort.Strings(names)

	return Candidate{
		Kind:       PrimitiveObsession,
		ScoreMilli: primitiveObsessionScoreMilli,
		Breakdown: Breakdown{
			Support:       len(sites),
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("%d functions take %s as raw %s (%s)", len(sites), concept, typ, strings.Join(names, ", ")),
		Inference:        "domain concept without a type — primitive obsession",
		PossibleRefactor: fmt.Sprintf("introduce type %s %s", typeName, typ),
		Sites:            sites,
	}
}
