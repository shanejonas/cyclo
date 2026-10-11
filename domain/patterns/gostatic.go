package patterns

import (
	"fmt"
	"sort"
)

// gostaticanalysis ports: nilerr, forcetypeassert, typednil.
// All three are detection-only bug finders (no safe mechanical fix).

// NilErrHit is one `if err != nil { return nil }` (or inverse) site.
type NilErrHit struct {
	Line     int
	CondText string
	Kind     string // "returns nil when err != nil"
}

// ForceTypeAssertHit is one unchecked `x.(T)` assertion.
type ForceTypeAssertHit struct {
	Line     int
	ExprText string
}

// TypedNilHit is one typed-nil vs untyped-nil comparison.
type TypedNilHit struct {
	Line     int
	ExprText string
}

const (
	nilErrScoreMilli          = 800
	forceTypeAssertScoreMilli = 700
	typedNilScoreMilli        = 750
)

// nilErrCandidates builds candidates from NilErrHit findings.
func nilErrCandidates(facts []*MiningFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		if isSuppressed(f.SuppressedKinds, NilErr) {
			continue
		}
		for _, h := range f.NilErrHits {
			out = append(out, Candidate{
				Kind:       NilErr,
				ScoreMilli: nilErrScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation: fmt.Sprintf(
					"%s: `%s`",
					h.Kind, h.CondText,
				),
				Inference: "checked error is swallowed",
				Sites: []Site{{
					Path: f.Path, Line: h.Line, EndLine: h.Line,
					ID: f.ID, Name: f.Name,
				}},
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Sites[0].Line < out[j].Sites[0].Line
	})
	return out
}

// forceTypeAssertCandidates builds candidates from ForceTypeAssertHit findings.
func forceTypeAssertCandidates(facts []*MiningFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		if isSuppressed(f.SuppressedKinds, ForceTypeAssert) {
			continue
		}
		for _, h := range f.ForceTypeAssertHits {
			out = append(out, Candidate{
				Kind:       ForceTypeAssert,
				ScoreMilli: forceTypeAssertScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation: fmt.Sprintf(
					"unchecked type assertion: `%s`",
					h.ExprText,
				),
				Inference: "panics on type mismatch; use comma-ok",
				Sites: []Site{{
					Path: f.Path, Line: h.Line, EndLine: h.Line,
					ID: f.ID, Name: f.Name,
				}},
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Sites[0].Line < out[j].Sites[0].Line
	})
	return out
}

// typedNilCandidates builds candidates from TypedNilHit findings.
func typedNilCandidates(facts []*MiningFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		if isSuppressed(f.SuppressedKinds, TypedNil) {
			continue
		}
		for _, h := range f.TypedNilHits {
			out = append(out, Candidate{
				Kind:       TypedNil,
				ScoreMilli: typedNilScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation: fmt.Sprintf(
					"typed nil compared to nil: `%s`",
					h.ExprText,
				),
				Inference: "interface nil checks do not test whether the contained value is nil",
				Sites: []Site{{
					Path: f.Path, Line: h.Line, EndLine: h.Line,
					ID: f.ID, Name: f.Name,
				}},
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Sites[0].Line < out[j].Sites[0].Line
	})
	return out
}

// isSuppressed reports whether kind is in the suppressed list.
func isSuppressed(suppressed []string, kind CandidateKind) bool {
	for _, s := range suppressed {
		if s == string(kind) {
			return true
		}
	}
	return false
}
