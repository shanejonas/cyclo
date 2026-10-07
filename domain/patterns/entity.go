package patterns

import "fmt"

// Entity identity patterns: DDD Entities are defined by identity, not
// attributes (Evans). These find code that violates that principle.

// entityIdentityScoreMilli is the fixed score for attribute-based equality.
// It's a real, actionable simplification with a mechanical fix.
const entityIdentityScoreMilli = 450

// missingIdentityScoreMilli is the fixed score for structs used as entities
// without an ID field. The fix (adding an ID field) is mechanical.
const missingIdentityScoreMilli = 400

// mutableIdentityScoreMilli is the fixed score for ID assignments outside
// constructors. Detection-only: there's no safe mechanical fix.
const mutableIdentityScoreMilli = 350

// entityIdentityCandidates builds candidates from attribute-based equality hits.
// Two modes:
// - ID exists: rewrite the comparison to use identity (standard fix).
// - No ID (paired with missing_identity): add the ID field first. After
//   re-mining, the standard rewrite fires. The phased fixer handles this.
func entityIdentityCandidates(facts []*FuncFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		for _, hit := range f.EntityIdentities {
			hit := hit
			f := f
			if hit.IDField == "" {
				out = append(out, noIDCandidate(f, hit))
				continue
			}
			out = append(out, rewriteCandidate(f, hit))
		}
	}
	return out
}

// noIDCandidate builds the candidate for attribute equality without an ID.
// The fixer adds the ID field; the comparison rewrite happens next phase.
func noIDCandidate(f *FuncFacts, hit EntityIdentityHit) Candidate {
	return Candidate{
		Kind:       EntityIdentity,
		ScoreMilli: entityIdentityScoreMilli,
		Breakdown: Breakdown{
			Support:       1,
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("line %d compares %s by %d fields (%s) but %s has no identity field", hit.Line, hit.TypeName, len(hit.Fields), joinFields(hit.Fields), hit.TypeName),
		Inference:        "entities need identity before they can be compared by it",
		PossibleRefactor: fmt.Sprintf("add ID field to %s, then compare by identity", hit.TypeName),
		Sites: []Site{{
			Path:    f.Path,
			Line:    f.Line,
			EndLine: f.EndLine,
			ID:      f.ID,
			Name:    f.Name,
		}},
		FixSpec: &FixSpec{
			Kind:    EntityIdentity,
			File:    hit.StructPath,
			Line:    hit.StructLine,
			EndLine: hit.StructLine,
			Params: map[string]string{
				"type":   hit.TypeName,
				"add_id": "true",
			},
		},
	}
}

// rewriteCandidate builds the candidate for rewriting to identity comparison.
func rewriteCandidate(f *FuncFacts, hit EntityIdentityHit) Candidate {
	return Candidate{
		Kind:       EntityIdentity,
		ScoreMilli: entityIdentityScoreMilli,
		Breakdown: Breakdown{
			Support:       1,
			CoverageMilli: 1000,
		},
		Observation:      fmt.Sprintf("line %d compares %s by %d fields (%s) instead of by identity", hit.Line, hit.TypeName, len(hit.Fields), joinFields(hit.Fields)),
		Inference:        "entities should be compared by identity, not attributes",
		PossibleRefactor: fmt.Sprintf("replace with %s.%s == %s.%s", hit.Left, hit.IDField, hit.Right, hit.IDField),
		Sites: []Site{{
			Path:    f.Path,
			Line:    f.Line,
			EndLine: f.EndLine,
			ID:      f.ID,
			Name:    f.Name,
		}},
		FixSpec: &FixSpec{
			Kind:    EntityIdentity,
			File:    f.Path,
			Line:    hit.Line,
			EndLine: hit.Line,
			Params: map[string]string{
				"line":      fmt.Sprintf("%d", hit.Line),
				"type":      hit.TypeName,
				"id_field":  hit.IDField,
				"left":      hit.Left,
				"right":     hit.Right,
			},
		},
	}
}

// missingIdentityCandidates builds detection-only candidates for structs
// compared by attributes without an ID field. This is the FIRST GATE: it
// flags the problem. The entity_identity fixer does the work (adds the ID
// and rewrites the comparison). No FixSpec: adding an ID speculatively,
// without a paired attribute comparison, is noise.
func missingIdentityCandidates(hits []MissingIdentityHit) []Candidate {
	var out []Candidate
	for _, hit := range hits {
		hit := hit
		out = append(out, Candidate{
			Kind:       MissingIdentity,
			ScoreMilli: missingIdentityScoreMilli,
			Breakdown: Breakdown{
				Support:       hit.UseCount,
				CoverageMilli: 1000,
			},
			Observation:      fmt.Sprintf("type %s is compared by %d attributes but has no identity field", hit.TypeName, hit.UseCount),
			Inference:        "entities need a stable identity independent of their attributes",
			PossibleRefactor: fmt.Sprintf("add an ID field to %s and compare by identity", hit.TypeName),
			Sites: []Site{{
				Path:    hit.Path,
				Line:    hit.Line,
				EndLine: hit.EndLine,
				Name:    hit.TypeName,
			}},
			// No FixSpec: detection-only first gate. The entity_identity
			// fixer handles the compound fix (add ID + rewrite comparison).
			FixSpec: nil,
		})
	}
	return out
}

// mutableIdentityCandidates builds detection-only candidates for ID mutation.
// These have no FixSpec: there's no safe mechanical fix for moving an
// assignment into a constructor.
func mutableIdentityCandidates(facts []*FuncFacts) []Candidate {
	var out []Candidate
	for _, f := range facts {
		for _, hit := range f.MutableIdentities {
			hit := hit
			f := f
			out = append(out, Candidate{
				Kind:       MutableIdentity,
				ScoreMilli: mutableIdentityScoreMilli,
				Breakdown: Breakdown{
					Support:       1,
					CoverageMilli: 1000,
				},
				Observation:      fmt.Sprintf("line %d assigns .%s outside a constructor in %s", hit.Line, hit.Field, hit.FuncName),
				Inference:        "entity identity should be immutable after creation",
				PossibleRefactor: "move the ID assignment into the constructor or factory function",
				Sites: []Site{{
					Path:    f.Path,
					Line:    f.Line,
					EndLine: f.EndLine,
					ID:      f.ID,
					Name:    f.Name,
				}},
				// No FixSpec: detection-only. There is no safe mechanical
				// transform for relocating an assignment into a constructor.
				FixSpec: nil,
			})
		}
	}
	return out
}

// joinFields formats field names for display.
func joinFields(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	out := fields[0]
	for _, f := range fields[1:] {
		out += ", " + f
	}
	return out
}
