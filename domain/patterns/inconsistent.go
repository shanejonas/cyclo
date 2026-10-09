package patterns

// Inconsistent clone detection (CP-Miner style) rebuilt on CCGraph groups.
//
// CP-Miner (Li et al.) finds "copy-paste bugs": cloned code where one instance
// was changed but the others weren't, suggesting a bug fix applied in one
// place but missed in the clones, or accidental divergence.
//
// The old implementation depended on the semantic_clone BFS enumeration.
// This rebuild works on CCGraph clone groups instead: within a group, pairs
// whose WL kernel similarity is at or above the clone threshold but below
// the divergence ceiling are suspicious — close enough to be clones, different
// enough that one of them may have diverged.
//
// Detection-only: deciding which version is correct requires human judgment.

// Thresholds in thousandths, matching SimilarityMilli's 0-1000 scale.
const (
	// inconsistentMinSim is the minimum WL similarity for a pair to count as
	// clones. Matches CCGraph's ccMatchThreshold.
	inconsistentMinSim uint32 = 900
	// inconsistentDivergent is the similarity below which a clone pair is
	// flagged as suspiciously divergent.
	inconsistentDivergent uint32 = 950
)

// FindInconsistentClones returns the similarity groups that contain at least
// one suspiciously divergent pair: members similar enough to be clones
// (>= inconsistentMinSim) but different enough to suggest inconsistent
// changes (< inconsistentDivergent).
func FindInconsistentClones(groups []SimilarityGroup, pdgs map[string]*Pdg) []SimilarityGroup {
	var out []SimilarityGroup
	for _, group := range groups {
		if hasDivergentPair(group.MemberIDs, pdgs) {
			out = append(out, group)
		}
	}
	return out
}

// hasDivergentPair reports whether any pair in the group falls in the
// suspicious similarity band.
func hasDivergentPair(group []string, pdgs map[string]*Pdg) bool {
	wls := buildGroupWls(group, pdgs)
	for i := 0; i < len(group); i++ {
		for j := i + 1; j < len(group); j++ {
			if isDivergentPair(group[i], group[j], wls) {
				return true
			}
		}
	}
	return false
}

// isDivergentPair reports whether two group members are clones that have
// suspiciously diverged.
func isDivergentPair(a, b string, wls map[string]*Wl) bool {
	wa, wb := wls[a], wls[b]
	if wa == nil || wb == nil {
		return false
	}
	sim := SimilarityMilli(wa, wb)
	return sim >= inconsistentMinSim && sim < inconsistentDivergent
}

// buildGroupWls builds WL vectors for group members that have PDGs.
func buildGroupWls(group []string, pdgs map[string]*Pdg) map[string]*Wl {
	out := make(map[string]*Wl, len(group))
	for _, id := range group {
		if pdg := pdgs[id]; pdg != nil {
			out[id] = NewWl(pdg)
		}
	}
	return out
}

// InconsistentCloneCandidates converts divergent similarity groups into
// pattern candidates for the report pipeline.
func InconsistentCloneCandidates(groups []SimilarityGroup, facts []*FuncFacts) []Candidate {
	factByID := buildFactMap(facts)
	var out []Candidate
	for _, group := range groups {
		if c, ok := makeInconsistentCandidate(group.MemberIDs, factByID); ok {
			out = append(out, c)
		}
	}
	return out
}

// makeInconsistentCandidate builds one candidate from a divergent group.
// Groups with fewer than two resolvable sites are dropped.
func makeInconsistentCandidate(group []string, factByID map[string]*FuncFacts) (Candidate, bool) {
	var sites []Site
	for _, id := range group {
		f := factByID[id]
		if f == nil {
			continue
		}
		sites = append(sites, Site{
			Path: f.Path,
			Line: f.Line,
			Name: f.Name,
		})
	}
	if len(sites) < 2 {
		return Candidate{}, false
	}
	return Candidate{
		Kind:             InconsistentClone,
		ScoreMilli:       600,
		Observation:      inconsistentObservation(sites),
		Inference:        "these clones have diverged: one instance may have a bug fix or change the others missed",
		PossibleRefactor: "compare the group members; apply the missing change or extract a shared helper",
		Sites:            sites,
	}, true
}

// inconsistentObservation names the group size for the report.
func inconsistentObservation(sites []Site) string {
	names := make([]string, 0, len(sites))
	for _, s := range sites {
		names = append(names, s.Name)
	}
	return "clone group of " + itoa(len(sites)) + " with divergent members: " + joinNames(names)
}

// joinNames joins function names with ", ".
func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// itoa converts a small int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
