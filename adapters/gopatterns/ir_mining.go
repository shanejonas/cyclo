package gopatterns

import (
	"go/ast"
	"strconv"

	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/domain/pdg"
)

func (b *builder) matchingNode(n patterns.PdgNode) int {
	var ref pdg.Ref
	if n.Kind == patterns.Param {
		b.parameter++
		ref = pdg.Ref(b.parameter)
	} else {
		category := miningCategory(n.Kind)
		attrs := pdg.Attributes{}
		if category == pdg.Other {
			attrs.Description = b.ir.pool.Text("Abstraction operation: " + string(n.Kind))
		}
		ref = b.ir.appendNode(b.origin, category, attrs)
	}
	node := &b.ir.graph.Nodes[ref-1]
	node.MatchLabel = b.ir.pool.MatchLabel(b.matchLabel(n))
	node.Line = uint32(n.Line)
	b.markSynthetic(node, n.Kind)
	return int(ref) - 1
}

func (b *builder) matchLabel(n patterns.PdgNode) pdg.MatchLabel {
	p := b.ir.pool
	return pdg.MatchLabel{Kind: p.Text(string(n.Kind)), TypeClass: p.Text(n.TyClass), SignatureClass: p.Text(n.SigClass), Callee: p.Text(n.CalleeID), LiteralKind: p.Text(n.LitKind), Detail: p.Text(n.Detail), Effects: n.Effects}
}

var miningCategories = map[patterns.NodeKind]pdg.Category{
	patterns.Param: pdg.FormalInput, patterns.Let: pdg.Declaration,
	patterns.Call: pdg.Call, patterns.Field: pdg.Read,
	patterns.Branch: pdg.Control, patterns.Match: pdg.Control,
	patterns.Iterate: pdg.Control, patterns.Loop: pdg.Control,
	patterns.Return: pdg.Return, patterns.Case: pdg.Control,
	patterns.Closure: pdg.Other, patterns.Defer: pdg.Other, patterns.Go: pdg.Other,
}

func miningCategory(kind patterns.NodeKind) pdg.Category {
	if category, ok := miningCategories[kind]; ok {
		return category
	}
	return pdg.Computation
}

func (b *irBuilder) miningEdge(from, to, position int, kind pdg.EdgeKind) {
	description := "Flow-insensitive first-binding or syntactic value dependence; not a reaching-definition assertion."
	subkind := "mining-value"
	if kind == pdg.ControlEdge {
		description = "Lexical control nesting; not post-dominator control dependence."
		subkind = "lexical-control"
	}
	b.graph.Edges = append(b.graph.Edges, pdg.Edge{
		ID:     b.pool.Text("e" + strconv.Itoa(len(b.graph.Edges)+1)),
		Source: pdg.Ref(from + 1), Target: pdg.Ref(to + 1), Kind: kind,
		Subkind: b.pool.Text(subkind), Position: pdg.Position(position + 1), Policy: b.policy,
		Evidence:   b.pool.Evidence(pdg.Approximated, description, b.policy),
		Provenance: b.pool.Origin(pdg.Analysis, description, b.policy),
	})
}

func (b *irBuilder) pack() error {
	if err := pdg.PackAttributes(&b.graph, b.attributes); err != nil {
		return err
	}
	pdg.Compact(&b.graph)
	b.attributes = nil
	return nil
}

func (b *builder) markSynthetic(node *pdg.Node, kind patterns.NodeKind) {
	if !syntheticMiningNode(kind, b.origin) {
		return
	}
	node.Synthetic = true
	node.Provenance = b.ir.pool.Origin(pdg.Desugaring, "Cyclo abstraction lowering of source operations.", b.ir.policy)
}

func syntheticMiningNode(kind patterns.NodeKind, origin ast.Node) bool {
	switch kind {
	case patterns.Try, patterns.Iterate:
		return true
	case patterns.Return:
		_, explicit := origin.(*ast.ReturnStmt)
		return !explicit
	default:
		return false
	}
}
