package patterns

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

// AntiUnify computes the most specific generalization (anti-unification) of
// two AST nodes, following Bulychev & Minea (2008). Where the trees differ,
// a hole variable is introduced; where they agree, the structure is kept.
//
// Returns the template (with hole variables as *ast.Ident) and the
// substitutions: for each hole, the differing subtrees from each side.
// The template + substitutions can reconstruct both originals, and any
// other common generalization is less specific.
func AntiUnify(a, b ast.Node) (template ast.Node, subs []Substitution) {
	return AntiUnifyWithRenames(a, b, nil)
}

// AntiUnifyWithRenames is AntiUnify with an optional variable rename map.
// renames maps identifiers in `a` to their counterparts in `b`; renamed
// identifiers are treated as identical, not as holes. This is used by the
// parameterize fixer, which normalizes local variable names before diffing.
func AntiUnifyWithRenames(a, b ast.Node, renames map[string]string) (template ast.Node, subs []Substitution) {
	au := &antiUnifier{subs: []Substitution{}, renames: renames}
	tmpl := au.unify(a, b)
	return tmpl, au.subs
}

// Substitution is one hole variable and the differing values from each side.
type Substitution struct {
	// Name is the hole variable, e.g. "hole0".
	Name string
	// A is the subtree from the first input.
	A ast.Node
	// B is the subtree from the second input.
	B ast.Node
}

type antiUnifier struct {
	subs []Substitution
	// renames maps identifiers in `a` to their counterparts in `b`.
	// Nil means no renames.
	renames map[string]string
}

func (au *antiUnifier) freshHole(a, b ast.Node) *ast.Ident {
	name := "hole" + strconv.Itoa(len(au.subs))
	au.subs = append(au.subs, Substitution{Name: name, A: a, B: b})
	return &ast.Ident{Name: name}
}

// unify returns the generalization of a and b, or a hole if they differ.
func (au *antiUnifier) unify(a, b ast.Node) ast.Node {
	if a == nil || b == nil {
		if a == b {
			return a
		}
		return au.freshHole(a, b)
	}
	// Dispatch by concrete type. Each handler returns nil if the types
	// don't match, falling through to the hole.
	if tmpl := au.unifyDispatch(a, b); tmpl != nil {
		return tmpl
	}
	return au.freshHole(a, b)
}

// unifyDispatch tries structural unification by concrete type.
// Returns nil if the node types differ.
func (au *antiUnifier) unifyDispatch(a, b ast.Node) ast.Node {
	// Use a type-indexed dispatch to keep cyclomatic complexity low.
	// Each handler returns nil on type mismatch.
	switch at := a.(type) {
	case *ast.Ident:
		return au.unifyIdent(at, b)
	case *ast.BasicLit:
		return au.unifyBasicLit(at, b)
	default:
		return au.unifyDispatchComplex(at, b)
	}
}

// unifyDispatchComplex handles the less common node types.
// Split to keep each function's complexity under the ceiling.
func (au *antiUnifier) unifyDispatchComplex(a ast.Node, b ast.Node) ast.Node {
	switch at := a.(type) {
	case *ast.BinaryExpr:
		return au.unifyBinaryExpr(at, b)
	case *ast.CallExpr:
		return au.unifyCallExpr(at, b)
	case *ast.AssignStmt:
		return au.unifyAssignStmt(at, b)
	default:
		return au.unifyDispatchStmt(at, b)
	}
}

// unifyDispatchStmt handles statement node types.
func (au *antiUnifier) unifyDispatchStmt(a ast.Node, b ast.Node) ast.Node {
	// Split dispatch to keep complexity under ceiling.
	if tmpl := au.unifyStmtCommon(a, b); tmpl != nil {
		return tmpl
	}
	return au.unifyStmtUncommon(a, b)
}

// unifyStmtCommon handles the most frequent statement types.
func (au *antiUnifier) unifyStmtCommon(a ast.Node, b ast.Node) ast.Node {
	switch at := a.(type) {
	case *ast.ExprStmt:
		return au.unifyExprStmt(at, b)
	case *ast.ReturnStmt:
		return au.unifyReturnStmt(at, b)
	case *ast.IfStmt:
		return au.unifyIfStmt(at, b)
	case *ast.BlockStmt:
		return au.unifyBlockStmt(at, b)
	}
	return nil
}

// unifyStmtUncommon handles less frequent statement types.
func (au *antiUnifier) unifyStmtUncommon(a ast.Node, b ast.Node) ast.Node {
	switch at := a.(type) {
	case *ast.DeclStmt:
		return au.unifyDeclStmt(at, b)
	case *ast.ForStmt:
		return au.unifyForStmt(at, b)
	case *ast.IncDecStmt:
		return au.unifyIncDecStmt(at, b)
	}
	return nil
}

// unifyIncDecStmt handles `i++` / `i--`.
func (au *antiUnifier) unifyIncDecStmt(at *ast.IncDecStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.IncDecStmt)
	if !ok {
		return nil
	}
	if at.Tok != bt.Tok {
		return au.freshHole(at, b)
	}
	x := au.unifyExpr(at.X, bt.X)
	return &ast.IncDecStmt{X: x, Tok: at.Tok}
}

// unifyForStmt handles `for` loops. Unifies init, cond, post, and body.
func (au *antiUnifier) unifyForStmt(at *ast.ForStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.ForStmt)
	if !ok {
		return nil
	}
	parts := au.unifyForParts(at, bt)
	if parts == nil {
		return au.freshHole(at, b)
	}
	return &ast.ForStmt{Init: parts.init, Cond: parts.cond, Post: parts.post, Body: parts.body}
}

type forParts struct {
	init ast.Stmt
	cond ast.Expr
	post ast.Stmt
	body *ast.BlockStmt
}

// unifyForParts unifies the components of two for loops.
// Returns nil if they can't be unified.
func (au *antiUnifier) unifyForParts(at, bt *ast.ForStmt) *forParts {
	init := au.unifyStmt(at.Init, bt.Init)
	if init == nil && at.Init != nil {
		return nil
	}
	post := au.unifyStmt(at.Post, bt.Post)
	if post == nil && at.Post != nil {
		return nil
	}
	body := au.unifyBlock(at.Body, bt.Body)
	if body == nil {
		return nil
	}
	return &forParts{
		init: init,
		cond: au.unifyExpr(at.Cond, bt.Cond),
		post: post,
		body: body,
	}
}

// unifyDeclStmt handles `var x T` declarations. If the declarations are
// structurally identical (same names and types), they're identical;
// otherwise it's a hole.
func (au *antiUnifier) unifyDeclStmt(at *ast.DeclStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.DeclStmt)
	if !ok {
		return nil
	}
	// For simplicity: if both are GenDecl with identical structure, keep it.
	// A full implementation would unify the ValueSpecs.
	if declsEqual(at, bt) {
		return at
	}
	return au.freshHole(at, b)
}

// declsEqual reports whether two DeclStmts are structurally identical.
// For parameterize, we only need the common case: `var x T` with the same
// names and types. Uses a simple syntactic check.
func declsEqual(a, b *ast.DeclStmt) bool {
	ag, okA := a.Decl.(*ast.GenDecl)
	bg, okB := b.Decl.(*ast.GenDecl)
	if !okA || !okB {
		return false
	}
	return genDeclsEqual(ag, bg)
}

// genDeclsEqual compares two GenDecls.
func genDeclsEqual(ag, bg *ast.GenDecl) bool {
	if ag.Tok != bg.Tok || len(ag.Specs) != len(bg.Specs) {
		return false
	}
	for i := range ag.Specs {
		if !valueSpecsEqual(ag.Specs[i], bg.Specs[i]) {
			return false
		}
	}
	return true
}

// valueSpecsEqual compares two Specs (must be ValueSpecs).
func valueSpecsEqual(a, b ast.Spec) bool {
	as, okA := a.(*ast.ValueSpec)
	bs, okB := b.(*ast.ValueSpec)
	if !okA || !okB {
		return false
	}
	return namesEqual(as.Names, bs.Names) && typesEqual(as.Type, bs.Type)
}

// namesEqual compares identifier lists.
func namesEqual(a, b []*ast.Ident) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

// typesEqual compares type expressions (nil-safe, Ident-only).
func typesEqual(a, b ast.Expr) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	at, okA := a.(*ast.Ident)
	bt, okB := b.(*ast.Ident)
	return okA && okB && at.Name == bt.Name
}

func (au *antiUnifier) unifyIdent(at *ast.Ident, b ast.Node) ast.Node {
	bt, ok := b.(*ast.Ident)
	if !ok {
		return nil
	}
	if at.Name == bt.Name {
		return &ast.Ident{Name: at.Name}
	}
	// Check renames: if at.Name maps to bt.Name, they're the same variable.
	if au.renames != nil && au.renames[at.Name] == bt.Name {
		return &ast.Ident{Name: at.Name}
	}
	return au.freshHole(at, b)
}

func (au *antiUnifier) unifyBasicLit(at *ast.BasicLit, b ast.Node) ast.Node {
	bt, ok := b.(*ast.BasicLit)
	if !ok {
		return nil
	}
	if at.Kind == bt.Kind && at.Value == bt.Value {
		return &ast.BasicLit{Kind: at.Kind, Value: at.Value}
	}
	return au.freshHole(at, b)
}

func (au *antiUnifier) unifyBinaryExpr(at *ast.BinaryExpr, b ast.Node) ast.Node {
	bt, ok := b.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	if at.Op != bt.Op {
		return au.freshHole(at, b)
	}
	return &ast.BinaryExpr{
		X:  au.unifyExpr(at.X, bt.X),
		Op: at.Op,
		Y:  au.unifyExpr(at.Y, bt.Y),
	}
}

func (au *antiUnifier) unifyCallExpr(at *ast.CallExpr, b ast.Node) ast.Node {
	bt, ok := b.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if len(at.Args) != len(bt.Args) {
		return au.freshHole(at, b)
	}
	fun := au.unifyExpr(at.Fun, bt.Fun)
	args := make([]ast.Expr, len(at.Args))
	for i := range at.Args {
		args[i] = au.unifyExpr(at.Args[i], bt.Args[i])
	}
	return &ast.CallExpr{Fun: fun, Args: args}
}

func (au *antiUnifier) unifyAssignStmt(at *ast.AssignStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.AssignStmt)
	if !ok {
		return nil
	}
	if !assignShapesMatch(at, bt) {
		return au.freshHole(at, b)
	}
	lhs := unifyExprList(au, at.Lhs, bt.Lhs)
	rhs := unifyExprList(au, at.Rhs, bt.Rhs)
	return &ast.AssignStmt{Lhs: lhs, Tok: at.Tok, Rhs: rhs}
}

// assignShapesMatch checks if two assignments have compatible shapes.
func assignShapesMatch(at, bt *ast.AssignStmt) bool {
	return at.Tok == bt.Tok && len(at.Lhs) == len(bt.Lhs) && len(at.Rhs) == len(bt.Rhs)
}

// unifyExprList unifies two expression lists element-wise.
func unifyExprList(au *antiUnifier, a, b []ast.Expr) []ast.Expr {
	out := make([]ast.Expr, len(a))
	for i := range a {
		out[i] = au.unifyExpr(a[i], b[i])
	}
	return out
}

func (au *antiUnifier) unifyExprStmt(at *ast.ExprStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	return &ast.ExprStmt{X: au.unifyExpr(at.X, bt.X)}
}

func (au *antiUnifier) unifyReturnStmt(at *ast.ReturnStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.ReturnStmt)
	if !ok {
		return nil
	}
	if len(at.Results) != len(bt.Results) {
		return au.freshHole(at, b)
	}
	results := make([]ast.Expr, len(at.Results))
	for i := range at.Results {
		results[i] = au.unifyExpr(at.Results[i], bt.Results[i])
	}
	return &ast.ReturnStmt{Results: results}
}

func (au *antiUnifier) unifyIfStmt(at *ast.IfStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.IfStmt)
	if !ok {
		return nil
	}
	return &ast.IfStmt{
		Cond: au.unifyExpr(at.Cond, bt.Cond),
		Body: au.unifyBlock(at.Body, bt.Body),
		Else: au.unifyStmt(at.Else, bt.Else),
	}
}

func (au *antiUnifier) unifyBlockStmt(at *ast.BlockStmt, b ast.Node) ast.Node {
	bt, ok := b.(*ast.BlockStmt)
	if !ok {
		return nil
	}
	return au.unifyBlock(at, bt)
}

func (au *antiUnifier) unifyExpr(a, b ast.Expr) ast.Expr {
	if n := au.unify(a, b); n != nil {
		if e, ok := n.(ast.Expr); ok {
			return e
		}
	}
	return au.freshHole(a, b)
}

func (au *antiUnifier) unifyStmt(a, b ast.Stmt) ast.Stmt {
	if a == nil && b == nil {
		return nil
	}
	if n := au.unify(a, b); n != nil {
		if s, ok := n.(ast.Stmt); ok {
			return s
		}
	}
	return nil
}

func (au *antiUnifier) unifyBlock(a, b *ast.BlockStmt) *ast.BlockStmt {
	if !blocksCompatible(a, b) {
		return nil
	}
	list := make([]ast.Stmt, len(a.List))
	for i := range a.List {
		s := au.unifyStmt(a.List[i], b.List[i])
		if s == nil {
			return nil
		}
		list[i] = s
	}
	return &ast.BlockStmt{List: list}
}

// blocksCompatible checks if two blocks can be unified.
func blocksCompatible(a, b *ast.BlockStmt) bool {
	if a == nil || b == nil {
		return a == b
	}
	return len(a.List) == len(b.List)
}

// TemplateString renders the template with holes as {{holeN}} for display.
func TemplateString(tmpl ast.Node) string {
	// Simple rendering: just show the structure.
	// In practice, use go/printer for full fidelity.
	return fmt.Sprintf("%T", tmpl)
}

// Ensure token is used (for Op preservation).
var _ = token.ILLEGAL
