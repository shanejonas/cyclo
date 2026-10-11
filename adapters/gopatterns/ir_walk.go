package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/domain/pdg"
)

// enrich adds source facts to the same graph, including writes hidden by
// abstraction lowering. It does not build a second graph or infer execution.
func (b *irBuilder) enrich(body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			b.stack = b.stack[:len(b.stack)-1]
			return true
		}
		if _, closure := n.(*ast.FuncLit); closure {
			return false
		}
		b.operation(n)
		b.stack = append(b.stack, n)
		return true
	})
}

func (b *irBuilder) operation(n ast.Node) {
	if b.typeSyntax(n) {
		return
	}
	switch n := n.(type) {
	case *ast.AssignStmt:
		b.assignment(n)
	case *ast.ValueSpec:
		b.declaration(n)
	case *ast.Ident:
		b.read(n)
	case *ast.CallExpr:
		b.call(n)
	default:
		b.statementOperation(n)
	}
}

func (b *irBuilder) statementOperation(n ast.Node) {
	switch n := n.(type) {
	case *ast.ReturnStmt:
		b.addNode(n, pdg.Return, pdg.Attributes{})
	case *ast.IncDecStmt:
		b.increment(n)
	case *ast.RangeStmt:
		b.rangeOperation(n)
	case *ast.SendStmt:
		b.addNode(n, pdg.Other, pdg.Attributes{Description: b.pool.Text("Channel send; concurrency dependencies are unknown.")})
	default:
		b.controlOperation(n)
	}
}

func (b *irBuilder) controlOperation(n ast.Node) {
	switch n := n.(type) {
	case *ast.IfStmt:
		b.addNode(n, pdg.Control, pdg.Attributes{Construct: b.pool.Text("if"), Outcomes: []pdg.Text{b.pool.Text("true"), b.pool.Text("false")}})
	case *ast.ForStmt:
		b.addNode(n, pdg.Control, pdg.Attributes{Construct: b.pool.Text("for")})
	case *ast.SwitchStmt:
		b.addNode(n, pdg.Control, pdg.Attributes{Construct: b.pool.Text("switch")})
	case *ast.TypeSwitchStmt:
		b.addNode(n, pdg.Control, pdg.Attributes{Construct: b.pool.Text("type-switch")})
	default:
		b.otherStatement(n)
	}
}

func (b *irBuilder) otherStatement(n ast.Node) {
	switch n := n.(type) {
	case *ast.SelectStmt:
		b.addNode(n, pdg.Control, pdg.Attributes{Construct: b.pool.Text("select")})
	case *ast.GoStmt:
		b.addNode(n, pdg.Other, pdg.Attributes{Description: b.pool.Text("Goroutine spawn; scheduling dependencies are unknown.")})
	case *ast.DeferStmt:
		b.addNode(n, pdg.Other, pdg.Attributes{Description: b.pool.Text("Deferred call; exit-time dependencies are unknown.")})
	case *ast.BranchStmt:
		b.addNode(n, pdg.Other, pdg.Attributes{Description: b.pool.Text("Branch transfer; execution dependencies are unknown.")})
	default:
		b.expressionOperation(n)
	}
}

func (b *irBuilder) expressionOperation(n ast.Node) {
	switch n := n.(type) {
	case *ast.BasicLit:
		b.addNode(n, pdg.Computation, pdg.Attributes{Literal: b.pool.Known(n.Value), Operator: b.pool.Known("literal")})
	case *ast.BinaryExpr:
		b.addNode(n, pdg.Computation, pdg.Attributes{Operator: b.pool.Known(n.Op.String())})
	case *ast.UnaryExpr:
		b.addNode(n, pdg.Computation, pdg.Attributes{Operator: b.pool.Known(n.Op.String())})
	case *ast.StarExpr:
		b.memoryRead(n, "dereference")
	default:
		b.accessOperation(n)
	}
}

func (b *irBuilder) accessOperation(n ast.Node) {
	switch n := n.(type) {
	case *ast.SelectorExpr:
		b.selector(n)
	case *ast.IndexExpr:
		b.memoryRead(n, "index")
	case *ast.SliceExpr:
		b.addNode(n, pdg.Computation, pdg.Attributes{Operator: b.pool.Known("slice")})
	case *ast.CompositeLit:
		b.addNode(n, pdg.Computation, pdg.Attributes{Operator: b.pool.Known("composite")})
	case *ast.TypeAssertExpr:
		b.addNode(n, pdg.Computation, pdg.Attributes{Operator: b.pool.Known("type-assert")})
	}
}

func (b *irBuilder) assignment(n *ast.AssignStmt) {
	attrs := pdg.Attributes{WriteForm: b.pool.Text(n.Tok.String())}
	category := pdg.Assignment
	if n.Tok == token.DEFINE {
		category = pdg.Declaration
	}
	id := b.addNode(n, category, attrs)
	for _, lhs := range n.Lhs {
		b.write(id, lhs)
	}
}

func (b *irBuilder) declaration(n *ast.ValueSpec) {
	id := b.addNode(n, pdg.Declaration, pdg.Attributes{})
	for _, name := range n.Names {
		b.write(id, name)
	}
}

func (b *irBuilder) increment(n *ast.IncDecStmt) {
	id := b.addNode(n, pdg.Assignment, pdg.Attributes{WriteForm: b.pool.Text(n.Tok.String())})
	b.write(id, n.X)
}

func (b *irBuilder) rangeOperation(n *ast.RangeStmt) {
	id := b.addNode(n, pdg.Control, pdg.Attributes{Construct: b.pool.Text("range")})
	if n.Key != nil {
		b.write(id, n.Key)
	}
	if n.Value != nil {
		b.write(id, n.Value)
	}
}

func (b *irBuilder) write(node pdg.Ref, expr ast.Expr) {
	access := pdg.Access{UnresolvedReason: b.pool.Text("Memory target and aliases are unresolved.")}
	if id, ok := expr.(*ast.Ident); ok {
		if id.Name == "_" {
			b.writeNames[id] = true
			return
		}
		access = b.variableWrite(id)
		b.definition(node, access.Symbol)
	}
	attrs := &b.attributes[b.graph.Nodes[node-1].Attributes-1]
	attrs.Writes = append(attrs.Writes, access)
	if access.Symbol != 0 {
		attrs.Symbols = append(attrs.Symbols, access.Symbol)
	}
}

func (b *irBuilder) variableWrite(id *ast.Ident) pdg.Access {
	b.writeNames[id] = true
	symbol := b.symbol(id, pdg.LocalRole)
	if symbol == 0 {
		return pdg.Access{UnresolvedReason: b.pool.Text("Write binding is unresolved.")}
	}
	return pdg.Access{Symbol: symbol}
}

func (b *irBuilder) read(id *ast.Ident) {
	if b.writeNames[id] || b.selectorName(id) {
		return
	}
	obj, ok := b.info.Uses[id].(*types.Var)
	if !ok {
		return
	}
	role := pdg.LocalRole
	if obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
		role = pdg.GlobalRole
	}
	symbol := b.symbol(id, role)
	node := b.readOwner(id)
	attrs := &b.attributes[b.graph.Nodes[node-1].Attributes-1]
	attrs.Reads = append(attrs.Reads, pdg.Access{Symbol: symbol})
}

func (b *irBuilder) selectorName(id *ast.Ident) bool {
	if len(b.stack) == 0 {
		return false
	}
	selector, ok := b.stack[len(b.stack)-1].(*ast.SelectorExpr)
	return ok && selector.Sel == id
}

func (b *irBuilder) memoryRead(n ast.Node, operator string) {
	b.addNode(n, pdg.Read, pdg.Attributes{
		Operator: b.pool.Known(operator),
		Reads:    []pdg.Access{{UnresolvedReason: b.pool.Text("Memory target and aliases are unresolved.")}},
	})
}

func (b *irBuilder) selector(n *ast.SelectorExpr) {
	if selection := b.info.Selections[n]; selection != nil && selection.Kind() == types.FieldVal {
		b.memoryRead(n, "field")
	}
}

func (b *irBuilder) call(n *ast.CallExpr) {
	if tv, ok := b.info.Types[n.Fun]; ok && tv.IsType() {
		b.addNode(n, pdg.Computation, pdg.Attributes{Operator: b.pool.Known("conversion")})
		return
	}
	b.addNode(n, pdg.Call, b.callAttributes(n.Fun))
}

func (b *irBuilder) callAttributes(expr ast.Expr) pdg.Attributes {
	unknown := b.pool.Missing(pdg.Unknown, "Call target is unresolved.")
	switch obj := b.callee(expr).(type) {
	case *types.Func:
		if b.dynamicMethod(expr) {
			return pdg.Attributes{CallForm: pdg.IndirectCall, Callee: unknown}
		}
		return pdg.Attributes{CallForm: pdg.DirectCall, Callee: b.pool.Known(patterns.FuncID(obj))}
	case *types.Var:
		return pdg.Attributes{CallForm: pdg.IndirectCall, Callee: unknown}
	case *types.Builtin:
		return pdg.Attributes{CallForm: pdg.DirectCall, Callee: b.pool.Known(obj.Name())}
	default:
		return pdg.Attributes{CallForm: pdg.UnknownCall, Callee: unknown}
	}
}

func (b *irBuilder) callee(expr ast.Expr) types.Object {
	switch expr := unparen(expr).(type) {
	case *ast.Ident:
		return b.info.ObjectOf(expr)
	case *ast.SelectorExpr:
		return b.info.ObjectOf(expr.Sel)
	default:
		return nil
	}
}

func (b *irBuilder) typeSyntax(n ast.Node) bool {
	expr, ok := n.(ast.Expr)
	return ok && b.info.Types[expr].IsType()
}

// Scalar reads are facts on their source operation, not separate topology.
func (b *irBuilder) readOwner(id *ast.Ident) pdg.Ref {
	for i := len(b.stack) - 1; i >= 0; i-- {
		if ref := b.nodes[b.stack[i]]; ref != 0 {
			return ref
		}
	}
	return b.addNode(id, pdg.Read, pdg.Attributes{})
}
