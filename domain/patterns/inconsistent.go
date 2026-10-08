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
func FindInconsistentEdits(groups [][]Subgraph, facts []*FuncFacts) []InconsistentEdit {
	bugsByFunc := buildBugsByFunc(facts)
	factByID := buildFactByID(facts)
	var out []InconsistentEdit
	for _, group := range groups {
		out = append(out, checkGroup(group, bugsByFunc, factByID)...)
	}
	return out
}

func buildBugsByFunc(facts []*FuncFacts) map[string]map[CandidateKind]bool {
	out := map[string]map[CandidateKind]bool{}
	for _, f := range facts {
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
		out[f.ID] = bugs
	}
	return out
}

func buildFactByID(facts []*FuncFacts) map[string]*FuncFacts {
	out := map[string]*FuncFacts{}
	for _, f := range facts {
		out[f.ID] = f
	}
	return out
}

func checkGroup(
	group []Subgraph,
	bugsByFunc map[string]map[CandidateKind]bool,
	factByID map[string]*FuncFacts,
) []InconsistentEdit {
	var out []InconsistentEdit
	for _, sg := range group {
		bugs := bugsByFunc[sg.FuncID]
		if len(bugs) == 0 {
			continue
		}
		siblings := siblingIDs(group, sg.FuncID)
		if len(siblings) == 0 {
			continue
		}
		out = append(out, checkMember(sg, bugs, siblings, bugsByFunc, factByID)...)
	}
	return out
}

func siblingIDs(group []Subgraph, exclude string) []string {
	var out []string
	for _, other := range group {
		if other.FuncID != exclude {
			out = append(out, other.FuncID)
		}
	}
	return out
}

func checkMember(
	sg Subgraph,
	bugs map[CandidateKind]bool,
	siblings []string,
	bugsByFunc map[string]map[CandidateKind]bool,
	factByID map[string]*FuncFacts,
) []InconsistentEdit {
	var out []InconsistentEdit
	for bugKind := range bugs {
		if siblingHasBug(siblings, bugKind, bugsByFunc) {
			continue
		}
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
	return out
}

func siblingHasBug(siblings []string, kind CandidateKind, bugsByFunc map[string]map[CandidateKind]bool) bool {
	for _, sibID := range siblings {
		if bugsByFunc[sibID][kind] {
			return true
		}
	}
	return false
}

// InconsistentCloneCandidates converts to pattern candidates.
func InconsistentCloneCandidates(edits []InconsistentEdit, facts []*FuncFacts) []Candidate {
	factByID := buildFactByID(facts)
	var out []Candidate
	for _, e := range edits {
		f := factByID[e.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:             InconsistentClone,
			ScoreMilli:       850,
			Observation:      fmt.Sprintf("has %s bug but %d clones do not", e.BugKind, len(e.CloneIDs)),
			Inference:        "the copies were not consistently modified; the edit was missed here",
			PossibleRefactor: "apply the same fix as the sibling clones, or extract a shared helper",
			Sites: []Site{
				{Path: f.Path, Line: e.Line, Name: f.Name},
			},
		})
	}
	return out
}
