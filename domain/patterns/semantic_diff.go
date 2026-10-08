package patterns

// Semantic Diff (Horwitz, PLDI 1990):
// Compare two program versions by their DEPENDENCE GRAPHS, not their text.
// Partition components into semantically changed vs. unchanged, even when
// the text moved. A refactoring that moves code produces zero semantic
// diff; a one-line change that alters dependences is flagged as significant.
//
// For cyclo: `check --changed` classifies each hunk as semantic vs.
// cosmetic by comparing PDG structure before/after. Pure refactors get a
// free pass from the quality gate; behavior changes get scrutiny. This
// fixes the #1 complaint about diff-gated linters (flagging moved code).

// DiffResult classifies a change as semantic or cosmetic.
type DiffResult struct {
	// Semantic is true if the dependence structure changed.
	Semantic bool
	// Reason explains the classification.
	Reason string
}

// SemanticDiff compares two PDGs (old vs. new version of a function).
// Returns Semantic=false if the dependence structure is identical
// (cosmetic change: moved code, renamed variables, reformatting).
// Returns Semantic=true if dependences were added, removed, or rewired.
func SemanticDiff(oldPdg, newPdg *Pdg) DiffResult {
	if oldPdg == nil || newPdg == nil {
		return DiffResult{Semantic: true, Reason: "missing PDG"}
	}
	oldHash := pdgStructureHash(oldPdg)
	newHash := pdgStructureHash(newPdg)
	if oldHash == newHash {
		return DiffResult{
			Semantic: false,
			Reason:   "identical dependence structure (cosmetic: moved/renamed/reformatted)",
		}
	}
	return DiffResult{
		Semantic: true,
		Reason:   "dependence structure changed",
	}
}

// pdgStructureHash computes a canonical hash of the PDG structure,
// ignoring line numbers and variable names (like semantic_clone).
// Two PDGs with the same hash are semantically equivalent.
func pdgStructureHash(pdg *Pdg) string {
	// Reuse the subgraph hashing logic for the whole PDG.
	// Build a canonical representation: sorted node labels + edge descriptors.
	var nodeLabels []string
	for _, n := range pdg.Nodes {
		nodeLabels = append(nodeLabels, nodeLabel(n))
	}
	// Sort for canonical form.
	sortStrings(nodeLabels)
	var edgeDescs []string
	for _, e := range pdg.Edges {
		from := nodeLabel(pdg.Nodes[e.From])
		to := nodeLabel(pdg.Nodes[e.To])
		edgeDescs = append(edgeDescs, from+"-"+to+":"+string(e.Kind))
	}
	sortStrings(edgeDescs)
	return joinStrings(nodeLabels, ";") + "#" + joinStrings(edgeDescs, ";")
}

// nodeLabel returns the canonical label for a PDG node (ignoring names/lines).
func nodeLabel(n PdgNode) string {
	return string(n.Kind) + "|" + n.TyClass + "|" + n.SigClass
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func joinStrings(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, x := range s[1:] {
		out += sep + x
	}
	return out
}
