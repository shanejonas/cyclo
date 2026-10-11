package patterns

import "github.com/shanejonas/cyclo/domain/pdg"

// PaperCharacteristics is computed once from canonical source facts. The order
// is declarations, assignments, controls, calls, other nodes, control edges,
// data edges, execution edges, reference bindings. Counts is immutable.
type PaperCharacteristics struct {
	Counts         [9]float64
	ReferenceKnown bool
}

func nativeCharacteristics(g *pdg.Graph) *PaperCharacteristics {
	out := &PaperCharacteristics{ReferenceKnown: g.Function.ReferenceCount.Status == pdg.Known}
	for _, node := range g.Nodes {
		if !paperFeatureNode(node.Category) {
			continue
		}
		out.Counts[paperCategoryIndex(node.Category)]++
	}
	for _, edge := range g.Edges {
		out.Counts[paperEdgeIndex(edge.Kind)]++
	}
	if out.ReferenceKnown {
		out.Counts[8] = float64(g.Function.ReferenceCount.Value)
	}
	return out
}

func paperCategoryIndex(category pdg.Category) int {
	switch category {
	case pdg.Declaration, pdg.FormalInput:
		return 0
	case pdg.Assignment:
		return 1
	case pdg.Control:
		return 2
	case pdg.Call:
		return 3
	default:
		return 4
	}
}

func paperEdgeIndex(kind pdg.EdgeKind) int {
	switch kind {
	case pdg.ControlEdge:
		return 5
	case pdg.Execution:
		return 7
	default:
		return 6
	}
}

func paperFeatureNode(category pdg.Category) bool {
	switch category {
	case pdg.FormalOutput, pdg.Entry, pdg.Exit:
		return false
	default:
		return true
	}
}
