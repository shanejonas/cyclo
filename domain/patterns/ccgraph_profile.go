package patterns

import "fmt"

// CloneProfile names a complete routing policy, not a claim of paper fidelity.
type CloneProfile string

const (
	CycloPairProfile  CloneProfile = "cyclo.ccgraph-pairs/1"
	CompatBaseProfile CloneProfile = "compat.ccgraph-base/draft"
)

// ClonePairEvidence is emitted once per unordered pair, in sorted ID order.
// WLKnown distinguishes an uncomputed score from a computed zero.
type ClonePairEvidence struct {
	Profile                     CloneProfile
	Left, Right                 string
	Numerical, Name             float64
	Admitted, WLKnown, Accepted bool
	WLMilli                     uint32
}

type clonePairInputs struct {
	ids     []string
	names   []string
	filters ccPairFilters
	wls     []*Wl
}

// EvaluateClonePairs explicitly selects pair routing without AST, LSH or grouping.
// It streams evidence rather than retaining a quadratic list. Inputs must have
// known nine-feature facts and names. It does not change the report default.
func EvaluateClonePairs(profile CloneProfile, graphs map[string]*MiningGraph, names map[string]string, emit func(ClonePairEvidence) error) error {
	if profile != CycloPairProfile {
		return fmt.Errorf("clone profile %q is unavailable: compat policies and fixtures remain unresolved", profile)
	}
	if emit == nil {
		return fmt.Errorf("clone pair evidence sink is required")
	}
	inputs, err := prepareClonePairs(graphs, names)
	if err != nil {
		return err
	}
	return inputs.evaluate(profile, emit)
}

func prepareClonePairs(graphs map[string]*MiningGraph, names map[string]string) (*clonePairInputs, error) {
	ids := ccSortableIDs(graphs)
	inputs := &clonePairInputs{ids: ids, names: make([]string, len(ids)), wls: make([]*Wl, len(ids))}
	inputs.filters.vecs = make([][]float64, len(ids))
	for i, id := range ids {
		if err := validateClonePairInput(id, graphs[id], names); err != nil {
			return nil, err
		}
		inputs.names[i] = shortName(names[id])
		inputs.filters.vecs[i] = characteristicVector(graphs[id])
		inputs.wls[i] = NewWlLight(graphs[id], inputs.filters.vecs[i])
	}
	inputs.filters.norms = vectorNorms(inputs.filters.vecs)
	return inputs, nil
}

func validateClonePairInput(id string, graph *MiningGraph, names map[string]string) error {
	if graph.Paper == nil || !graph.Paper.ReferenceKnown {
		return fmt.Errorf("function %q requires known nine-feature facts", id)
	}
	if _, ok := names[id]; !ok {
		return fmt.Errorf("function %q requires a known name", id)
	}
	return nil
}

func (in *clonePairInputs) evaluate(profile CloneProfile, emit func(ClonePairEvidence) error) error {
	for i := range in.ids {
		for j := i + 1; j < len(in.ids); j++ {
			if err := emit(in.evidence(profile, i, j)); err != nil {
				return fmt.Errorf("clone pair evidence: %w", err)
			}
		}
	}
	return nil
}

func (in *clonePairInputs) evidence(profile CloneProfile, i, j int) ClonePairEvidence {
	e := ClonePairEvidence{Profile: profile, Left: in.ids[i], Right: in.ids[j], Numerical: in.filters.cosine(i, j)}
	e.Name = jaroWinkler(in.names[i], in.names[j])
	e.Admitted = e.Numerical >= charVecThreshold || e.Name >= ccStage2NameThreshold
	if !e.Admitted {
		return e
	}
	e.WLMilli = SimilarityMilli(in.wls[i], in.wls[j])
	e.WLKnown = true
	e.Accepted = e.WLMilli >= ccMatchThreshold
	return e
}
