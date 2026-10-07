package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func (b *builder) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.IfStmt:
		b.ifStmt(s)
	case *ast.ForStmt:
		b.forStmt(s)
	case *ast.RangeStmt:
		b.rangeStmt(s)
	case *ast.SwitchStmt:
		b.switchStmt(s)
	case *ast.TypeSwitchStmt:
		b.typeSwitch(s)
	default:
		b.stmtLeaf(s)
	}
}

// stmtLeaf lowers non-control-flow statements.
func (b *builder) stmtLeaf(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.BlockStmt:
		for _, inner := range s.List {
			b.stmt(inner)
		}
	case *ast.ExprStmt:
		b.expr(s.X)
	case *ast.AssignStmt:
		b.assign(s)
	case *ast.ReturnStmt:
		b.returnStmt(s)
	default:
		b.stmtEffect(s)
	}
}

// stmtEffect lowers statements with launch/send effects and skips
// declarations, empty statements, and branches (break/continue/goto), which
// carry no data value.
func (b *builder) stmtEffect(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.IncDecStmt:
		b.incDec(s)
	case *ast.LabeledStmt:
		b.stmt(s.Stmt)
	case *ast.DeferStmt:
		b.wrappedCall(s.Call, patterns.Defer, s.Pos())
	case *ast.GoStmt:
		b.wrappedCall(s.Call, patterns.Go, s.Pos())
	case *ast.SendStmt:
		n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: "send:<-", Line: b.line(s.Pos())})
		b.data(b.expr(s.Chan), n, 0)
		b.data(b.expr(s.Value), n, 1)
	default:
		// *ast.DeclStmt, *ast.EmptyStmt, *ast.BranchStmt: no data value;
		// control structure is already captured by Ctrl edges.
	}
}

func (b *builder) incDec(s *ast.IncDecStmt) {
	class := "inc"
	if s.Tok == token.DEC {
		class = "dec"
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: class + ":" + s.Tok.String(), Line: b.line(s.Pos())})
	b.data(b.expr(s.X), n, 0)
}

// assign lowers assignments. `x := e` is transparent (x IS e's node);
// destructuring (`a, b := f()`) gets a Let node; `=` becomes an Op node.
// Bindings are flow-insensitive: reassignments never move a binding.
func (b *builder) assign(s *ast.AssignStmt) {
	if s.Tok == token.DEFINE {
		b.define(s)
		return
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Op, Detail: assignClass(s.Tok) + ":" + s.Tok.String(), Line: b.line(s.Pos())})
	pos := 0
	for i, lhs := range s.Lhs {
		pos = b.assignOperand(n, lhs, assignRhs(s, i, len(s.Lhs)), pos)
	}
}

// assignOperand wires one LHS/RHS pair into the assignment Op node.
func (b *builder) assignOperand(n int, lhs, rhs ast.Expr, pos int) int {
	if id, ok := unparen(lhs).(*ast.Ident); ok && id.Name != "_" {
		if old, ok := b.lookup(b.info.ObjectOf(id)); ok {
			b.data(old, n, pos)
			pos++
		}
	}
	if rhs != nil {
		b.data(b.expr(rhs), n, pos)
		pos++
	}
	return pos
}

func assignClass(tok token.Token) string {
	if tok == token.ASSIGN {
		return "assign"
	}
	return "assignop"
}

func assignRhs(s *ast.AssignStmt, index, lhsCount int) ast.Expr {
	if len(s.Rhs) == 0 {
		return nil
	}
	if len(s.Rhs) == lhsCount {
		return s.Rhs[index]
	}
	return s.Rhs[0]
}

// define handles `:=`. A single name bound to a single-valued expression is
// transparent (x IS e's node); anything else (including `_, err := f()` where
// f is multi-valued) is a destructuring Let.
func (b *builder) define(s *ast.AssignStmt) {
	idents := definedIdents(s.Lhs)
	if len(idents) == 1 && len(s.Rhs) == 1 && b.singleValued(s.Rhs[0]) {
		// Simple `x := e`: transparent.
		if obj := b.info.Defs[idents[0]]; obj != nil {
			b.bind(obj, b.expr(s.Rhs[0]))
		}
		return
	}
	b.destructure(s, idents)
}

// destructure lowers `a, b := ...` to one Let node binding all names.
func (b *builder) destructure(s *ast.AssignStmt, idents []*ast.Ident) {
	n := b.node(patterns.PdgNode{Kind: patterns.Let, Line: b.line(s.Pos())})
	for i, rhs := range s.Rhs {
		b.data(b.expr(rhs), n, i)
	}
	for _, id := range idents {
		b.bind(b.info.Defs[id], n)
	}
}

// definedIdents returns the non-blank identifiers defined by an LHS list.
func definedIdents(lhs []ast.Expr) []*ast.Ident {
	idents := make([]*ast.Ident, 0, len(lhs))
	for _, e := range lhs {
		if id, ok := unparen(e).(*ast.Ident); ok && id.Name != "_" {
			idents = append(idents, id)
		}
	}
	return idents
}

// singleValued reports whether an expression produces exactly one value.
// Multi-valued calls (tuples) are destructurings, not transparent bindings.
func (b *builder) singleValued(e ast.Expr) bool {
	tv, ok := b.info.Types[unparen(e)]
	if !ok {
		return true
	}
	if tup, ok := tv.Type.(*types.Tuple); ok {
		return tup.Len() <= 1
	}
	return true
}

// ifStmt lowers if statements, recognizing the Try idiom:
// `if err != nil { return err }` becomes a Try node (Go's `?`).
func (b *builder) ifStmt(s *ast.IfStmt) {
	if s.Init != nil {
		b.stmt(s.Init)
	}
	if obj, ok := b.tryIdiom(s); ok {
		b.tryNode(s, obj)
		return
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Branch, Line: b.line(s.Pos())})
	b.data(b.expr(s.Cond), n, 0)
	b.under(n, 0, func() { b.stmt(s.Body) })
	if s.Else != nil {
		b.under(n, 1, func() { b.stmt(s.Else) })
	}
}

// tryIdiom matches `if <id> != nil { return <id> }` with no else.
func (b *builder) tryIdiom(s *ast.IfStmt) (types.Object, bool) {
	if s.Else != nil || len(s.Body.List) != 1 {
		return nil, false
	}
	obj, ok := b.tryCond(s.Cond)
	if !ok {
		return nil, false
	}
	if !b.tryBody(s.Body.List[0], obj) {
		return nil, false
	}
	return obj, true
}

// tryCond matches `<id> != nil` and returns the identified object.
func (b *builder) tryCond(cond ast.Expr) (types.Object, bool) {
	bin, ok := neqNil(cond)
	if !ok {
		return nil, false
	}
	x, ok := unparen(bin.X).(*ast.Ident)
	if !ok || x.Name == "_" {
		return nil, false
	}
	obj := b.info.ObjectOf(x)
	if obj == nil {
		return nil, false
	}
	return obj, true
}

// neqNil matches `<x> != nil` and returns the comparison.
func neqNil(cond ast.Expr) (*ast.BinaryExpr, bool) {
	bin, ok := unparen(cond).(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ || !isNil(bin.Y) {
		return nil, false
	}
	return bin, true
}

// tryBody matches `return <id>` for the same object.
func (b *builder) tryBody(s ast.Stmt, obj types.Object) bool {
	ret, ok := s.(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	r, ok := unparen(ret.Results[0]).(*ast.Ident)
	return ok && b.info.ObjectOf(r) == obj
}

func isNil(e ast.Expr) bool {
	id, ok := unparen(e).(*ast.Ident)
	return ok && id.Name == "nil"
}

// tryNode emits the Try node plus its controlled `return err`, matching the
// validated spike encoding: Try takes the error value at 0; the Return takes
// it at 0 and is Ctrl-dependent on the Try (arm 0).
func (b *builder) tryNode(s *ast.IfStmt, obj types.Object) {
	n := b.node(patterns.PdgNode{Kind: patterns.Try, Line: b.line(s.Pos())})
	if v, ok := b.lookup(obj); ok {
		b.data(v, n, 0)
	}
	b.under(n, 0, func() {
		r := b.node(patterns.PdgNode{Kind: patterns.Return, Line: b.line(s.Body.Pos())})
		if v, ok := b.lookup(obj); ok {
			b.data(v, r, 0)
		}
	})
}

// forStmt lowers for loops. Canonical counter loops
// (`for i := 0; i < len(coll); i++`) become Iterate over coll with the
// loop-element canonicalization; everything else becomes Loop.
func (b *builder) forStmt(s *ast.ForStmt) {
	if s.Init != nil {
		b.stmt(s.Init)
	}
	if counter, coll, ok := b.counterLoop(s); ok {
		collNode := b.expr(coll)
		n := b.node(patterns.PdgNode{Kind: patterns.Iterate, Line: b.line(s.Pos())})
		b.data(collNode, n, 0)
		b.bind(counter, n)
		b.loops = append(b.loops, loopCtx{counter: counter, coll: collNode, iterate: n})
		b.under(n, 0, func() { b.stmt(s.Body) })
		b.loops = b.loops[:len(b.loops)-1]
		return
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Loop, Line: b.line(s.Pos())})
	if s.Cond != nil {
		b.data(b.expr(s.Cond), n, 0)
	}
	b.under(n, 0, func() { b.stmt(s.Body) })
	if s.Post != nil {
		b.stmt(s.Post)
	}
}

// counterLoop recognizes `for i := <int>; i < len(coll); i++`
// (also <=, >, >= with the operands flipped). coll must be a plain
// identifier for the element canonicalization to apply.
func (b *builder) counterLoop(s *ast.ForStmt) (types.Object, ast.Expr, bool) {
	counter, ok := b.counterInit(s.Init)
	if !ok {
		return nil, nil, false
	}
	if !b.counterPost(s.Post, counter) {
		return nil, nil, false
	}
	coll, ok := lenBound(s.Cond, counter, b.info)
	if !ok {
		return nil, nil, false
	}
	return counter, coll, true
}

// counterInit matches `i := <expr>` and returns the counter object.
func (b *builder) counterInit(init ast.Stmt) (types.Object, bool) {
	assign, ok := init.(*ast.AssignStmt)
	if !ok || !isSingleDefine(assign) {
		return nil, false
	}
	id, ok := unparen(assign.Lhs[0]).(*ast.Ident)
	if !ok {
		return nil, false
	}
	obj := b.info.ObjectOf(id)
	if obj == nil {
		return nil, false
	}
	return obj, true
}

func isSingleDefine(assign *ast.AssignStmt) bool {
	return assign.Tok == token.DEFINE && len(assign.Lhs) == 1 && len(assign.Rhs) == 1
}

// counterPost matches `i++` / `i--` on the counter.
func (b *builder) counterPost(post ast.Stmt, counter types.Object) bool {
	inc, ok := post.(*ast.IncDecStmt)
	if !ok {
		return false
	}
	id, ok := unparen(inc.X).(*ast.Ident)
	return ok && b.info.ObjectOf(id) == counter
}

// lenBound finds `len(coll)` in a `counter < len(coll)`-shaped comparison and
// returns coll when it is a plain identifier.
func lenBound(cond ast.Expr, counter types.Object, info *types.Info) (ast.Expr, bool) {
	bin, ok := unparen(cond).(*ast.BinaryExpr)
	if !ok {
		return nil, false
	}
	switch bin.Op {
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return nil, false
	}
	if coll, ok := lenSide(bin.X, bin.Y, counter, info); ok {
		return coll, true
	}
	return lenSide(bin.Y, bin.X, counter, info)
}

// lenSide checks whether side is `len(coll)` and other is the counter.
func lenSide(side, other ast.Expr, counter types.Object, info *types.Info) (ast.Expr, bool) {
	coll, ok := lenCallArg(side, info)
	if !ok {
		return nil, false
	}
	id, ok := unparen(other).(*ast.Ident)
	if !ok || info.ObjectOf(id) != counter {
		return nil, false
	}
	return coll, true
}

// lenCallArg extracts coll from `len(coll)` when coll is a plain identifier.
func lenCallArg(side ast.Expr, info *types.Info) (ast.Expr, bool) {
	call, ok := unparen(side).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	if !isLenCall(call, info) {
		return nil, false
	}
	coll, ok := unparen(call.Args[0]).(*ast.Ident)
	if !ok {
		return nil, false
	}
	return coll, true
}

func isLenCall(call *ast.CallExpr, info *types.Info) bool {
	fun, ok := unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := info.ObjectOf(fun).(*types.Builtin)
	return ok && builtin.Name() == "len"
}

// rangeStmt lowers `for k, v := range coll`: Iterate over coll, with the
// value (and key) binding to the Iterate node itself, like rstyle's iterate.
func (b *builder) rangeStmt(s *ast.RangeStmt) {
	collNode := b.expr(s.X)
	n := b.node(patterns.PdgNode{Kind: patterns.Iterate, Line: b.line(s.Pos())})
	b.data(collNode, n, 0)
	for _, v := range []ast.Expr{s.Key, s.Value} {
		if id, ok := unparen(v).(*ast.Ident); ok && id.Name != "_" {
			b.bind(b.info.ObjectOf(id), n)
		}
	}
	// Range key as counter: coll[key] inside resolves to the element.
	var counter types.Object
	if id, ok := unparen(s.Key).(*ast.Ident); ok && id.Name != "_" {
		counter = b.info.ObjectOf(id)
	}
	b.loops = append(b.loops, loopCtx{counter: counter, coll: collNode, iterate: n})
	b.under(n, 0, func() { b.stmt(s.Body) })
	b.loops = b.loops[:len(b.loops)-1]
}

// switchStmt lowers value switches to Match: scrutinee at 0, each case body
// under its arm index.
func (b *builder) switchStmt(s *ast.SwitchStmt) {
	if s.Init != nil {
		b.stmt(s.Init)
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Match, Line: b.line(s.Pos())})
	if s.Tag != nil {
		b.data(b.expr(s.Tag), n, 0)
	}
	for i, stmt := range s.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		arm := i
		b.under(n, arm, func() {
			for _, inner := range clause.Body {
				b.stmt(inner)
			}
		})
	}
}

// typeSwitch lowers `switch v := x.(type)`: Match on x, v binds to the Match
// node (like rstyle's matcher binding arm patterns to the node).
func (b *builder) typeSwitch(s *ast.TypeSwitchStmt) {
	if s.Init != nil {
		b.stmt(s.Init)
	}
	n := b.node(patterns.PdgNode{Kind: patterns.Match, Line: b.line(s.Pos())})
	if bound := b.typeSwitchTarget(s, n); bound != nil {
		b.bind(b.info.ObjectOf(bound), n)
	}
	for i, stmt := range s.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		arm := i
		b.under(n, arm, func() {
			for _, inner := range clause.Body {
				b.stmt(inner)
			}
		})
	}
}

// typeSwitchTarget links the switched value at 0 and returns the bound
// identifier, if the switch binds one.
func (b *builder) typeSwitchTarget(s *ast.TypeSwitchStmt, n int) *ast.Ident {
	switch assign := s.Assign.(type) {
	case *ast.AssignStmt:
		return b.typeSwitchAssign(assign, n)
	case *ast.ExprStmt:
		if ta, ok := unparen(assign.X).(*ast.TypeAssertExpr); ok {
			b.data(b.expr(ta.X), n, 0)
		}
	}
	return nil
}

func (b *builder) typeSwitchAssign(assign *ast.AssignStmt, n int) *ast.Ident {
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return nil
	}
	ta, ok := unparen(assign.Rhs[0]).(*ast.TypeAssertExpr)
	if !ok {
		return nil
	}
	b.data(b.expr(ta.X), n, 0)
	if id, ok := unparen(assign.Lhs[0]).(*ast.Ident); ok && id.Name != "_" {
		return id
	}
	return nil
}

func (b *builder) returnStmt(s *ast.ReturnStmt) {
	n := b.node(patterns.PdgNode{Kind: patterns.Return, Line: b.line(s.Pos())})
	for i, res := range s.Results {
		b.data(b.expr(res), n, i)
	}
}

// wrappedCall lowers `defer f()` / `go f()`: the call lowers normally, then a
// Defer/Go node wraps it with a data edge, so the launch semantics survive.
func (b *builder) wrappedCall(call *ast.CallExpr, kind patterns.NodeKind, pos token.Pos) {
	c := b.expr(call)
	n := b.node(patterns.PdgNode{Kind: kind, Line: b.line(pos)})
	b.data(c, n, 0)
}
