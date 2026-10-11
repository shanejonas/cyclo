package patterns

import "github.com/shanejonas/cyclo/domain/pdg"

// Run derives a temporary matching view from the canonical IR.
func Run(facts []*FuncFacts, options Options) PatternsReport {
	views := make([]*MiningFacts, 0, len(facts))
	labels := sharedBaseLabels(facts)
	for _, f := range facts {
		if f == nil {
			continue
		}
		view := miningFacts(f)
		labels.attach(view.Pdg)
		views = append(views, view)
	}
	return RunMining(views, options)
}

func miningFacts(f *FuncFacts) *MiningFacts {
	return &MiningFacts{
		ID:                  f.ID,
		Name:                f.Name,
		Path:                f.Path,
		Line:                f.Line,
		EndLine:             f.EndLine,
		SuppressedKinds:     f.SuppressedKinds,
		AstTypes:            f.AstTypes,
		Mutates:             f.Mutates,
		EffectKinds:         f.EffectKinds,
		SigKey:              f.SigKey,
		SelfTy:              f.SelfTy,
		Implements:          f.Implements,
		GuardClauses:        f.GuardClauses,
		EnumDispatches:      f.EnumDispatches,
		TypeSwitches:        f.TypeSwitches,
		EntityIdentities:    f.EntityIdentities,
		MutableIdentities:   f.MutableIdentities,
		AggregateMods:       f.AggregateMods,
		DbCalls:             f.DbCalls,
		FactoryLits:         f.FactoryLits,
		SpecRules:           f.SpecRules,
		NilErrHits:          f.NilErrHits,
		ForceTypeAssertHits: f.ForceTypeAssertHits,
		TypedNilHits:        f.TypedNilHits,
		ErrorCheckSites:     f.ErrorCheckSites,
		Params:              f.Params,
		Pdg:                 MiningView(f.Pdg),
	}
}

// MiningView resolves interned labels. It owns no compiler objects and is not
// retained by the canonical graph. Characteristics use canonical source categories.
func MiningView(g *pdg.Graph) *MiningGraph {
	if g == nil {
		return nil
	}
	view := &MiningGraph{Paper: nativeCharacteristics(g), Nodes: make([]PdgNode, 0, matchingNodeCount(g)), Edges: make([]PdgEdge, 0, len(g.Edges))}
	indexes := make([]int, len(g.Nodes))
	for i, n := range g.Nodes {
		indexes[i] = -1
		if n.MatchLabel == 0 {
			continue
		}
		indexes[i] = len(view.Nodes)
		view.Nodes = append(view.Nodes, miningNode(g, n))
	}
	for _, e := range g.Edges {
		appendMiningEdge(view, indexes, e)
	}
	return view
}

func miningNode(g *pdg.Graph, n pdg.Node) PdgNode {
	return miningLabel(g.Tables, g.Tables.MatchLabels[n.MatchLabel-1], int(n.Line))
}

func appendMiningEdge(view *MiningGraph, indexes []int, e pdg.Edge) {
	from, to := indexes[e.Source-1], indexes[e.Target-1]
	if from < 0 || to < 0 {
		return
	}
	kind := Data
	if e.Kind == pdg.ControlEdge {
		kind = Ctrl
	}
	if e.Kind == pdg.Execution {
		kind = Exec
	}
	view.Edges = append(view.Edges, PdgEdge{From: from, To: to, Kind: kind, ArgPos: int(e.Position) - 1})
}

func matchingNodeCount(g *pdg.Graph) int {
	count := 0
	for _, node := range g.Nodes {
		if node.MatchLabel != 0 {
			count++
		}
	}
	return count
}

func miningLabel(t *pdg.Tables, l pdg.MatchLabel, line int) PdgNode {
	return PdgNode{Kind: NodeKind(t.Text(l.Kind)), TyClass: t.Text(l.TypeClass), SigClass: t.Text(l.SignatureClass), CalleeID: t.Text(l.Callee), LitKind: t.Text(l.LiteralKind), Detail: t.Text(l.Detail), Effects: l.Effects, Line: line}
}
