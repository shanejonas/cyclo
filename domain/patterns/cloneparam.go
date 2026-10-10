package patterns

// cloneGroupFacts resolves members through the shared corpus index.
func cloneGroupFacts(factByID map[string]*FuncFacts, group []string) []*FuncFacts {
	var fns []*FuncFacts
	for _, id := range group {
		if f := factByID[id]; f != nil && f.Pdg != nil {
			fns = append(fns, f)
		}
	}
	return fns
}

// alignCloneGroup runs PDG alignment on the group members' PDGs, returning
// the cluster with hole columns. Nil when alignment fails to join members.
// buildCluster tries the top-3 templates by similarity and keeps the
// alignment with the most joined members.
func alignCloneGroup(fns []*FuncFacts, params Params) *Cluster {
	pdgs := make([]*Pdg, len(fns))
	for i, f := range fns {
		pdgs[i] = f.Pdg
	}
	canonical, wls := prepare(pdgs, params, nil)
	indices := make([]int, len(canonical))
	for i := range indices {
		indices[i] = i
	}
	cluster, ok := buildCluster(canonical, wls, indices, params)
	if !ok {
		return nil
	}
	return &cluster
}

// ParameterizeFromCloneGroup bridges CCGraph clone detection and the
// anti-unification abstraction proposer (Bulychev & Minea 2008).
//
// CCGraph finds *that* functions are similar (clone groups of function IDs).
// This derives *how* they differ — via PDG alignment yielding hole columns —
// and proposes the extracted helper with the differing parts as parameters.
//
// Takes:
//   - facts: all FuncFacts for ID lookup
//   - group: a CCGraph clone group (function IDs from CCGraphClones)
//   - params: clustering params for the alignment (thresholds, max holes)
//
// Returns a *Candidate with Kind=Parameterize, or nil when the group does
// not yield a parameterizable abstraction (too few members, no PDGs,
// alignment fails, or parameterize rejects it).
//
// Deterministic: group order is preserved, alignment is deterministic, and
// the candidate construction sorts sites.
func ParameterizeFromCloneGroup(facts []*FuncFacts, group []string, params Params) *Candidate {
	return parameterizeCloneGroup(newCandidateIndex(facts, nil, params), group, params)
}

func parameterizeCloneGroup(ix *candidateIndex, group []string, params Params) *Candidate {
	if len(group) < 2 {
		return nil
	}
	fns := cloneGroupFacts(ix.byID, group)
	if len(fns) < 2 {
		return nil
	}
	// Safety: don't propose extracting helpers from functions with
	// incompatible side-effect profiles. If one member does IO and another
	// is pure, a shared helper would be wrong.
	if !compatibleEffects(fns) {
		return nil
	}
	cluster := alignCloneGroup(fns, params)
	if cluster == nil {
		return nil
	}
	// parameterize expects sites aligned with cluster.Members.
	sites := sitesOf(fns, cluster)
	return parameterize(ix, sites, cluster)
}

// compatibleEffects reports whether all functions in the group have the same
// side-effect profile (union of Call node Effects). Functions with wildly
// different effects (e.g., one does IO, another is pure) should not share
// an extracted helper.
func compatibleEffects(fns []*FuncFacts) bool {
	var first uint16
	for i, f := range fns {
		var union uint16
		if f.Pdg != nil {
			for _, n := range f.Pdg.Nodes {
				union |= n.Effects
			}
		}
		if i == 0 {
			first = union
		} else if union != first {
			return false
		}
	}
	return true
}

// CloneGroupParameterizeCandidates runs ParameterizeFromCloneGroup on every
// CCGraph clone group, returning the parameterize candidates. Groups that
// do not yield an abstraction are skipped.
func CloneGroupParameterizeCandidates(facts []*FuncFacts, groups [][]string, params Params) []Candidate {
	ix := newCandidateIndex(facts, nil, params)
	var out []Candidate
	for _, group := range groups {
		if c := parameterizeCloneGroup(ix, group, params); c != nil {
			out = append(out, *c)
		}
	}
	return out
}
