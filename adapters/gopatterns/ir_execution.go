package gopatterns

import (
	"github.com/shanejonas/cyclo/domain/pdg"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/cfg"
	"strconv"
)

// A sparse must-precede basis: consecutive operation anchors in a reachable
// block, plus the nearest dominating operation for each block's first anchor.
// Successor links are analysis inputs; they are never exported as dependencies.
func (b *irBuilder) execution(body *ast.BlockStmt) {
	b.executionMetadata()
	flow := cfg.New(body, b.mayReturn)
	dom := newExecutionDominators(flow)
	anchors := make([][]pdg.Ref, len(flow.Blocks))
	controls := b.executionControls(body)
	for _, index := range dom.order {
		anchors[index] = b.executionAnchors(flow.Blocks[index], controls)
	}
	for _, index := range dom.order {
		previous := dom.previousAnchor(index, anchors)
		for _, node := range anchors[index] {
			b.executionEdge(previous, node)
			previous = node
		}
	}
}

func (b *irBuilder) mayReturn(call *ast.CallExpr) bool {
	builtin, ok := b.callee(call.Fun).(*types.Builtin)
	return !ok || builtin.Name() != "panic"
}

func (b *irBuilder) executionAnchors(block *cfg.Block, controls map[ast.Node]pdg.Ref) []pdg.Ref {
	var anchors []pdg.Ref
	if node := b.executionLoopAnchor(block); node != 0 {
		anchors = append(anchors, node)
	}
	for _, source := range block.Nodes {
		if statement, ok := source.(*ast.ExprStmt); ok {
			source = statement.X
		}
		node := b.executionAnchor(source, controls)
		if node == 0 {
			continue
		}
		anchors = append(anchors, node)
	}
	return anchors
}

func (d *executionDominators) previousAnchor(index int, anchors [][]pdg.Ref) pdg.Ref {
	for index != 0 {
		index = d.idom[index]
		nodes := anchors[index]
		if len(nodes) > 0 {
			return nodes[len(nodes)-1]
		}
	}
	return 0
}

func (b *irBuilder) executionEdge(from, to pdg.Ref) {
	if from == 0 || from == to {
		return
	}
	b.graph.Edges = append(b.graph.Edges, pdg.Edge{
		ID: b.pool.Text("e" + strconv.Itoa(len(b.graph.Edges)+1)), Source: from, Target: to, Kind: pdg.Execution,
		Subkind: b.pool.Text("must-precede"), Policy: b.executionPolicy,
		Evidence:   b.executionEvidence,
		Provenance: b.executionOrigin,
	})
}

func (b *irBuilder) executionLoopAnchor(block *cfg.Block) pdg.Ref {
	if block.Kind != cfg.KindRangeLoop && block.Kind != cfg.KindForLoop {
		return 0
	}
	return b.nodes[block.Stmt]
}

func (b *irBuilder) executionAnchor(source ast.Node, controls map[ast.Node]pdg.Ref) pdg.Ref {
	if node := b.nodes[source]; node != 0 {
		return node
	}
	return controls[source]
}

func (b *irBuilder) executionControls(body *ast.BlockStmt) map[ast.Node]pdg.Ref {
	controls := map[ast.Node]pdg.Ref{}
	ast.Inspect(body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		condition := executionCondition(node)
		if condition != nil {
			controls[condition] = b.nodes[node]
		}
		return true
	})
	return controls
}

func executionCondition(node ast.Node) ast.Node {
	switch node := node.(type) {
	case *ast.IfStmt:
		return node.Cond
	case *ast.ForStmt:
		return node.Cond
	case *ast.SwitchStmt:
		return node.Tag
	default:
		return nil
	}
}

func (b *irBuilder) executionMetadata() {
	policy := b.pool.Policy("cyclo.go-must-precede", "1")
	description := "Reachable statement/condition anchors; same-block order or nearest dominating operation. Intrastatement, panic/recover and concurrent ordering are not complete."
	b.executionPolicy = policy
	b.executionEvidence = b.pool.Evidence(pdg.Approximated, description, policy)
	b.executionOrigin = b.pool.Origin(pdg.Analysis, description, policy)
}
