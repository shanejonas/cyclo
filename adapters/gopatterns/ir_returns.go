package gopatterns

import (
	"go/ast"
	"strconv"

	"github.com/shanejonas/cyclo/domain/pdg"
)

// returnValues links explicit returns to their function's result slots.
// A return operation can produce several values, including one tuple-valued call.
// Nested closure returns belong to another function and are excluded.
func (b *irBuilder) returnValues(body *ast.BlockStmt) {
	if len(b.outputs) == 0 {
		return
	}
	policy := b.pool.Policy("cyclo.go-return-values", "1")
	description := "Syntactic transfer from an explicit return to its formal output slot; reaching definitions, deferred result writes and exceptional exits are not modeled."
	evidence := b.pool.Evidence(pdg.Approximated, description, policy)
	origin := b.pool.Origin(pdg.Analysis, description, policy)
	ast.Inspect(body, func(n ast.Node) bool {
		if _, closure := n.(*ast.FuncLit); closure {
			return false
		}
		if ret, ok := n.(*ast.ReturnStmt); ok {
			b.returnSlots(ret, policy, evidence, origin)
		}
		return true
	})
}

func (b *irBuilder) returnSlots(ret *ast.ReturnStmt, policy, evidence, origin pdg.Ref) {
	node := b.nodes[ret]
	if len(ret.Results) == 0 {
		b.namedReturnReads(node)
	}
	for position, output := range b.outputs {
		b.graph.Edges = append(b.graph.Edges, pdg.Edge{
			ID:     b.pool.Text("e" + strconv.Itoa(len(b.graph.Edges)+1)),
			Source: node, Target: output, Kind: pdg.Data, Subkind: b.pool.Text("return-value"),
			Position: pdg.Position(position + 1), Policy: policy, Evidence: evidence, Provenance: origin,
		})
	}
}

// Bare returns read the named result bindings at this return site. These are
// access facts, not claims about which definitions reach those bindings.
func (b *irBuilder) namedReturnReads(node pdg.Ref) {
	attrs := &b.attributes[b.graph.Nodes[node-1].Attributes-1]
	for position, param := range b.graph.Function.Outputs {
		if param.Symbol == 0 {
			continue
		}
		attrs.Reads = append(attrs.Reads, pdg.Access{Symbol: param.Symbol, Position: pdg.Position(position + 1)})
	}
}
