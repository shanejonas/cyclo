package patterns

import "fmt"

// CP-Miner (Li, Lu, Myagmar, Zhou, OSDI 2004):
// Find copy-pasted code, then check whether the copies were CONSISTENTLY
// modified. Inconsistent edits to clones are bugs (e.g., forgot the nil
// check in the pasted copy).
//
// Modern clone detectors find clones but don't do the "were the edits
// consistent?" check. For cyclo: an `inconsistent_clone` pattern that pairs
// semantic_clone groups with the bug detectors (nilerr, etc.). If one clone
// has a bug and its siblings don't, the edit was inconsistent.

// InconsistentEdit is a clone with a bug its siblings lack.
type InconsistentEdit struct {
	FuncID   string
	Line     int
	CloneIDs []string // Sibling clones without the bug.
	BugKind  CandidateKind
}

// FindInconsistentEdits checks semantic clone groups for asymmetric bugs.
// For each group, if some members have a bug (e.g., nilerr) and others
// don't, the buggy ones are inconsistent edits.
func FindInconsistentEdits(
	groups [][]Subgraph,
	facts []*FuncFacts,
) []InconsistentEdit {
	// Map func ID to its bug kinds.
	bugsByFunc := map[string]map[CandidateKind]bool{}
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
		bugs := map[CandidateKind]bool{}
		if len(f.NilErrHits) > 0 {
			bugs[NilErr] = true
		}
		if len(f.ForceTypeAssertHits) > 0 {
			bugs[ForceTypeAssert] = true
		}
		if len(f.TypedNilHits) > 0 {
			bugs[TypedNil] = true
		}
		bugsByFunc[f.ID] = bugs
	}
	var out []InconsistentEdit
	for _, group := range groups {
		// Collect bug kinds per member.
		for _, sg := range group {
			bugs := bugsByFunc[sg.FuncID]
			if len(bugs) == 0 {
				continue
			}
			// This member has bugs; check if siblings lack them.
			var siblings []string
			for _, other := range group {
				if other.FuncID == sg.FuncID {
					continue
				}
				siblings = append(siblings, other.FuncID)
			}
			if len(siblings) == 0 {
				continue
			}
			// For each bug kind, check if any sibling lacks it.
			for bugKind := range bugs {
				siblingHasBug := false
				for _, sibID := range siblings {
					if bugsByFunc[sibID][bugKind] {
						siblingHasBug = true
						break
					}
				}
				if !siblingHasBug {
					f := factByID[sg.FuncID]
					line := f.Line
					if len(sg.Lines) > 0 {
						line = sg.Lines[0]
					}
					out = append(out, InconsistentEdit{
						FuncID:   sg.FuncID,
						Line:     line,
						CloneIDs: siblings,
						BugKind:  bugKind,
					})
				}
			}
		}
	}
	return out
}

// InconsistentCloneCandidates converts to pattern candidates.
func InconsistentCloneCandidates(edits []InconsistentEdit, facts []*FuncFacts) []Candidate {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, e := range edits {
		f := factByID[e.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:        InconsistentClone,
			ScoreMilli:  850, // High: inconsistent edits are bugs.
			Observation: fmt.Sprintf("has %s bug but %d clones do not", e.BugKind, len(e.CloneIDs)),
			Inference:   "the copies were not consistently modified; the edit was missed here",
			PossibleRefactor: fmt.Sprintf(
				"apply the same fix as the sibling clones, or extract a shared helper",
			),
			Sites: []Site{
				{Path: f.Path, Line: e.Line, Name: f.Name},
			},
		})
	}
	return out
}
