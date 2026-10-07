package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// expr lowers an expression to the node producing its value, or none when the
// expression is a bare variable use (resolved through binds), blank, or has
// no meaningful value node.
func (b *builder) expr(e ast.Expr) int {
	switch e := unparen(e).(type) {
	case *ast.Ident:
		return b.ident(e)
	case *ast.BasicLit:
		return b.litNode(e)
	case *ast.CallExpr:
		return b.callExpr(e)
	case *ast.SelectorExpr:
		return b.field(e)
	case *ast.FuncLit:
		// Opaque: the body is not descended into (like rstyle's Closure).
		return b.node(patterns.PdgNode{Kind: patterns.Closure, Line: b.line(e.Pos())})
	default:
		return b.exprOp(e)
	}
}

func (b *builder) litNode(e *ast.BasicLit) int {
	return b.node(patterns.PdgNode{Kind: patterns.Lit, LitKind: litKind(e.Kind), Detail: e.Value, Line: b.line(e.Pos())})
}

// exprOp lowers operator-like expressions: indexing, arithmetic, composites.
func (b *builder) exprOp(e ast.Expr) int {
	switch e := e.(type) {
	case *ast.IndexExpr:
		return b.indexExpr(e)
	case *ast.BinaryExpr:
		return b.binary(e)
	case *ast.UnaryExpr:
		return b.unary(e)
	case *ast.StarExpr:
		n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: "deref:*", Line: b.line(e.Pos())})
		b.data(b.expr(e.X), n, 0)
		return n
	case *ast.CompositeLit:
		return b.composite(e)
	default:
		return b.exprWrap(e)
	}
}

// exprWrap lowers transparent wrappers and casts.
func (b *builder) exprWrap(e ast.Expr) int {
	switch e := e.(type) {
	case *ast.IndexListExpr:
		return b.expr(e.X)
	case *ast.TypeAssertExpr:
		n := b.node(patterns.PdgNode{Kind: patterns.Cast, Line: b.line(e.Pos())})
		b.data(b.expr(e.X), n, 0)
		return n
	case *ast.SliceExpr:
		return b.sliceExpr(e)
	case *ast.KeyValueExpr:
		return b.expr(e.Value)
	case *ast.Ellipsis:
		return b.expr(e.Elt)
	default:
		return none
	}
}

func (b *builder) sliceExpr(e *ast.SliceExpr) int {
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: "slice:[:]", Line: b.line(e.Pos())})
	b.data(b.expr(e.X), n, 0)
	b.data(b.expr(e.Low), n, 1)
	b.data(b.expr(e.High), n, 2)
	b.data(b.expr(e.Max), n, 3)
	return n
}

func unparen(e ast.Expr) ast.Expr {
	if e == nil {
		return nil
	}
	if p, ok := e.(*ast.ParenExpr); ok {
		return unparen(p.X)
	}
	return e
}

func (b *builder) ident(e *ast.Ident) int {
	if e.Name == "_" {
		return none
	}
	if obj := b.info.Defs[e]; obj != nil {
		// Definition site; the binding statement handles it.
		return none
	}
	obj := b.info.Uses[e]
	if obj == nil {
		return none
	}
	if n, ok := b.lookup(obj); ok {
		return n
	}
	return none
}

func litKind(k token.Token) string {
	switch k {
	case token.INT:
		return "int"
	case token.FLOAT:
		return "float"
	case token.IMAG:
		return "imag"
	case token.CHAR:
		return "char"
	case token.STRING:
		return "string"
	default:
		return "other"
	}
}

// field lowers a selector expression that is not a call (calls handle their
// own receiver). Detail carries the field name; it is not part of the label.
func (b *builder) field(e *ast.SelectorExpr) int {
	n := b.node(patterns.PdgNode{Kind: patterns.Field, Detail: e.Sel.Name, Line: b.line(e.Pos())})
	b.data(b.expr(e.X), n, 0)
	return n
}

// indexExpr lowers indexing, with the loop-element canonicalization:
// coll[counter] inside the loop iterating coll resolves to the Iterate node,
// so C-style `for` and `range` over the same collection share a shape.
func (b *builder) indexExpr(e *ast.IndexExpr) int {
	if elem, ok := b.loopElement(e); ok {
		return elem
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: "index:[]", Line: b.line(e.Pos())})
	b.data(b.expr(e.X), n, 0)
	b.data(b.expr(e.Index), n, 1)
	return n
}

// loopElement resolves coll[counter] to the enclosing Iterate node when the
// index is that loop's counter and the base is its collection.
func (b *builder) loopElement(e *ast.IndexExpr) (int, bool) {
	id, ok := unparen(e.Index).(*ast.Ident)
	if !ok {
		return 0, false
	}
	obj := b.info.ObjectOf(id)
	if obj == nil {
		return 0, false
	}
	for i := len(b.loops) - 1; i >= 0; i-- {
		if elem, ok := b.loops[i].element(e.X, obj, b); ok {
			return elem, true
		}
	}
	return 0, false
}

// element checks one loop context for the canonicalization.
func (lc loopCtx) element(x ast.Expr, obj types.Object, b *builder) (int, bool) {
	if lc.counter == nil || obj != lc.counter {
		return 0, false
	}
	base, ok := unparen(x).(*ast.Ident)
	if !ok {
		return 0, false
	}
	if n, ok := b.lookup(b.info.ObjectOf(base)); ok && n == lc.coll {
		return lc.iterate, true
	}
	return 0, false
}

func (b *builder) binary(e *ast.BinaryExpr) int {
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: opClass(e.Op) + ":" + e.Op.String(), Line: b.line(e.Pos())})
	b.data(b.expr(e.X), n, 0)
	b.data(b.expr(e.Y), n, 1)
	return n
}

// opClass is the coarse operator class for labels: the operator after the
// colon is a hole, not part of the class (like rstyle's "cmp:<").
func opClass(op token.Token) string {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.GTR, token.LEQ, token.GEQ:
		return "cmp"
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM:
		return "arith"
	case token.AND, token.OR, token.XOR, token.SHL, token.SHR, token.AND_NOT:
		return "bit"
	case token.LAND, token.LOR:
		return "logic"
	default:
		return "op"
	}
}

func (b *builder) unary(e *ast.UnaryExpr) int {
	var class string
	switch e.Op {
	case token.AND:
		class = "addr"
	case token.ARROW:
		class = "recv"
	default:
		class = "unary"
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: class + ":" + e.Op.String(), Line: b.line(e.Pos())})
	b.data(b.expr(e.X), n, 0)
	return n
}

func (b *builder) composite(e *ast.CompositeLit) int {
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: "composite:{}", Line: b.line(e.Pos())})
	for i, elt := range e.Elts {
		b.data(b.expr(elt), n, i)
	}
	return n
}

// callExpr lowers a call. The callee resolves to a Call node carrying the
// stable callee id and normalized signature class (never the raw name).
// Receiver (method calls) links at position 0; free-function args start at 0.
func (b *builder) callExpr(e *ast.CallExpr) int {
	fun := unparen(e.Fun)
	// Type conversion: T(x).
	if b.info.Types[fun].IsType() {
		n := b.node(patterns.PdgNode{Kind: patterns.Cast, Line: b.line(e.Pos())})
		if len(e.Args) > 0 {
			b.data(b.expr(e.Args[0]), n, 0)
		}
		return n
	}
	calleeID, sigClass, receiver := b.resolveCallee(fun)
	n := b.node(patterns.PdgNode{Kind: patterns.Call, CalleeID: calleeID, SigClass: sigClass, Line: b.line(e.Pos())})
	argPos := 0
	switch {
	case receiver != nil:
		b.data(b.expr(receiver), n, 0)
		argPos = 1
	case calleeID == "":
		// Unresolved callee (func value, closure): link what it evaluates to.
		if c := b.expr(fun); c != none {
			b.data(c, n, 0)
			argPos = 1
		}
	}
	b.callArgs(e, n, argPos)
	return n
}

// resolveCallee identifies the call target: methods (x.M), package functions
// (pkg.F), free functions, builtins, or unresolved (func values).
func (b *builder) resolveCallee(fun ast.Expr) (string, string, ast.Expr) {
	if sel, ok := fun.(*ast.SelectorExpr); ok {
		return b.selectorCallee(sel)
	}
	id, ok := fun.(*ast.Ident)
	if !ok {
		return "", b.signatureClass(fun), nil
	}
	switch obj := b.info.ObjectOf(id).(type) {
	case *types.Builtin:
		// Builtins have no types.Func; the name is the class.
		return "builtin." + obj.Name(), "builtin." + obj.Name(), nil
	case *types.Func:
		return patterns.FuncID(obj), sigClassOf(obj), nil
	default:
		return "", b.signatureClass(fun), nil
	}
}

// selectorCallee resolves x.M(): the stable method id plus receiver, or
// pkg.F() with no receiver to link.
func (b *builder) selectorCallee(sel *ast.SelectorExpr) (string, string, ast.Expr) {
	obj, ok := b.info.ObjectOf(sel.Sel).(*types.Func)
	if !ok {
		return "", b.signatureClass(sel), nil
	}
	var receiver ast.Expr
	if b.info.Types[sel.X].IsValue() {
		receiver = sel.X
	}
	return patterns.FuncID(obj), sigClassOf(obj), receiver
}

func (b *builder) signatureClass(e ast.Expr) string {
	if sig, ok := b.info.TypeOf(e).(*types.Signature); ok {
		return patterns.SigClass(sig)
	}
	return "_"
}

func sigClassOf(fn *types.Func) string {
	if sig, ok := fn.Type().(*types.Signature); ok {
		return patterns.SigClass(sig)
	}
	return "_"
}

func (b *builder) callArgs(e *ast.CallExpr, n, argPos int) {
	for _, arg := range e.Args {
		b.data(b.expr(arg), n, argPos)
		argPos++
	}
}
