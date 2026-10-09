package gopatterns

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"unicode"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// Fix is a single applied fix, implemented by GuardFix and ParamFix.
type Fix interface {
	// FixLine is the primary source line of the fix.
	FixLine() int
	// FixKind is the candidate kind fixed.
	FixKind() string
}

// ParamFix describes one applied parameterize rewrite: two parallel
// functions whose common body was extracted into a helper.
type ParamFix struct {
	// Lines are the original function definition lines.
	Lines []int
	// Kind is always "parameterize".
	Kind string
	// Extracted is the name of the new helper function.
	Extracted string
}

// FixLine implements Fix.
func (p ParamFix) FixLine() int { return p.Lines[0] }

// FixKind implements Fix.
func (p ParamFix) FixKind() string { return p.Kind }

// maxParamHoles caps the holes per pair; more than this is not a clean
// extraction.
const maxParamHoles = 3

// FixParameterize finds pairs of structurally parallel functions in f and
// extracts their common body into a helper. Holes (differing subexpressions)
// become parameters of the helper; both originals become thin wrappers.
//
// If candidates is non-empty, only the named pairs are considered;
// otherwise all pairs in the file are auto-detected via AST comparison.
// It returns the rewritten source (gofmt-clean) and the fixes applied.
// The transform is behavior-preserving when the safety checks pass;
// otherwise the pair is skipped.
func FixParameterize(fset *token.FileSet, f *ast.File, src []byte, candidates []patterns.Candidate) ([]byte, []ParamFix, error) {
	funcs := freeFuncs(f)
	if len(funcs) < 2 {
		return src, nil, nil
	}
	allow := buildAllowlist(candidates)
	edits, fixes := collectEdits(fset, funcs, allow, src)
	if len(edits) == 0 {
		return src, nil, nil
	}
	return formatEdits(src, edits, fixes)
}

// collectEdits finds all parameterize pairs and builds their edits.
func collectEdits(fset *token.FileSet, funcs []*ast.FuncDecl, allow map[[2]string]bool, src []byte) ([]textEdit, []ParamFix) {
	var edits []textEdit
	var fixes []ParamFix
	used := map[string]bool{}
	tc := &tryCtx{fset: fset, allow: allow, used: used, src: src}
	for i := 0; i < len(funcs); i++ {
		for j := i + 1; j < len(funcs); j++ {
			if edit, fix, ok := tc.tryPair(funcs[i], funcs[j]); ok {
				edits = append(edits, edit)
				fixes = append(fixes, fix)
			}
		}
	}
	return edits, fixes
}

// tryCtx carries the context for trying function pairs.
type tryCtx struct {
	fset  *token.FileSet
	allow map[[2]string]bool
	used  map[string]bool
	src   []byte
}

// tryPair attempts one pair, marking used on success.
func (tc *tryCtx) tryPair(a, b *ast.FuncDecl) (textEdit, ParamFix, bool) {
	if tc.used[a.Name.Name] || tc.used[b.Name.Name] {
		return textEdit{}, ParamFix{}, false
	}
	if !pairAllowed(a, b, tc.allow) {
		return textEdit{}, ParamFix{}, false
	}
	edit, fix, ok := paramEdit(tc.fset, a, b, tc.src)
	if !ok {
		return textEdit{}, ParamFix{}, false
	}
	tc.used[a.Name.Name] = true
	tc.used[b.Name.Name] = true
	return edit, fix, true
}

// pairAllowed reports whether the pair is in the allowlist.
func pairAllowed(a, b *ast.FuncDecl, allow map[[2]string]bool) bool {
	return allow == nil || allow[[2]string{a.Name.Name, b.Name.Name}]
}

// formatEdits applies edits and gofmts the result.
func formatEdits(src []byte, edits []textEdit, fixes []ParamFix) ([]byte, []ParamFix, error) {
	out := applyEdits(src, edits)
	formatted, err := format.Source(out)
	if err != nil {
		return nil, nil, fmt.Errorf("gofmt after parameterize fix: %w", err)
	}
	return formatted, fixes, nil
}

// buildAllowlist builds the pair allowlist from miner candidates.
// It returns nil when candidates is empty (allow all pairs).
func buildAllowlist(candidates []patterns.Candidate) map[[2]string]bool {
	if len(candidates) == 0 {
		return nil
	}
	allow := map[[2]string]bool{}
	for _, c := range candidates {
		if c.Kind != patterns.Parameterize {
			continue
		}
		for i := 0; i < len(c.Sites); i++ {
			for j := i + 1; j < len(c.Sites); j++ {
				a, b := c.Sites[i].Name, c.Sites[j].Name
				allow[[2]string{a, b}] = true
				allow[[2]string{b, a}] = true
			}
		}
	}
	return allow
}

// freeFuncs returns the free (non-method) function declarations in f.
func freeFuncs(f *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil {
			continue
		}
		out = append(out, fn)
	}
	return out
}

// hole is one differing subexpression between two parallel functions.
type hole struct {
	// aExpr, bExpr are the differing expressions.
	aExpr, bExpr ast.Expr
}

// safeToExtract reports whether the pair passes the safety preconditions:
// matching signatures, no closures, no named results.
func safeToExtract(fset *token.FileSet, a, b *ast.FuncDecl) bool {
	if !sameSignature(fset, a, b) {
		return false
	}
	if hasFuncLit(a.Body) || hasFuncLit(b.Body) {
		return false
	}
	return !hasNamedResults(a) && !hasNamedResults(b)
}

// allIdentHoles reports whether every hole is a simple identifier.
func allIdentHoles(holes []hole) bool {
	for _, h := range holes {
		if _, ok := h.aExpr.(*ast.Ident); !ok {
			return false
		}
		if _, ok := h.bExpr.(*ast.Ident); !ok {
			return false
		}
	}
	return true
}

// paramEdit computes the text edit that extracts the common body of a and b.
// It returns false when the extraction cannot be proven safe.
func paramEdit(fset *token.FileSet, a, b *ast.FuncDecl, src []byte) (textEdit, ParamFix, bool) {
	if !safeToExtract(fset, a, b) {
		return textEdit{}, ParamFix{}, false
	}
	r, holes, ok := diffPair(a, b)
	if !ok {
		return textEdit{}, ParamFix{}, false
	}
	if !adjacent(fset, a, b, src) {
		return textEdit{}, ParamFix{}, false
	}
	ctx := &paramCtx{fset: fset, a: a, b: b, src: src, r: r, holes: holes}
	return buildEdit(ctx)
}

// diffPair builds renames and diffs the bodies, validating holes.
// Uses anti-unification (Bulychev & Minea 2008) for the principled
// most-specific template instead of heuristic diffing.
func diffPair(a, b *ast.FuncDecl) (*renames, []hole, bool) {
	r, ok := buildRenames(a, b)
	if !ok {
		return nil, nil, false
	}
	holes, ok := antiUnifyHoles(a.Body, b.Body, r)
	if !ok || len(holes) == 0 || len(holes) > maxParamHoles {
		return nil, nil, false
	}
	if !allIdentHoles(holes) {
		return nil, nil, false
	}
	return r, holes, true
}

// buildEdit constructs the final text edit from validated holes.
func buildEdit(ctx *paramCtx) (textEdit, ParamFix, bool) {
	ctx.extractedName = helperName(ctx.a.Name.Name, ctx.b.Name.Name)
	replacement, ok := buildReplacement(ctx)
	if !ok {
		return textEdit{}, ParamFix{}, false
	}
	aStart := ctx.fset.PositionFor(ctx.a.Pos(), false).Offset
	bEnd := ctx.fset.PositionFor(ctx.b.End(), false).Offset
	lineA := ctx.fset.PositionFor(ctx.a.Pos(), false).Line
	lineB := ctx.fset.PositionFor(ctx.b.Pos(), false).Line
	return textEdit{start: aStart, end: bEnd, replacement: replacement},
		ParamFix{Lines: []int{lineA, lineB}, Kind: "parameterize", Extracted: ctx.extractedName},
		true
}

// adjacent reports whether b immediately follows a (only whitespace/comments
// between).
func adjacent(fset *token.FileSet, a, b *ast.FuncDecl, src []byte) bool {
	aEnd := fset.PositionFor(a.End(), false).Offset
	bStart := fset.PositionFor(b.Pos(), false).Offset
	between := bytes.TrimSpace(src[aEnd:bStart])
	for len(between) > 0 {
		if bytes.HasPrefix(between, []byte("//")) {
			idx := bytes.IndexByte(between, '\n')
			if idx < 0 {
				return true
			}
			between = bytes.TrimSpace(between[idx+1:])
		} else if bytes.HasPrefix(between, []byte("/*")) {
			idx := bytes.Index(between, []byte("*/"))
			if idx < 0 {
				return false
			}
			between = bytes.TrimSpace(between[idx+2:])
		} else {
			return false
		}
	}
	return true
}

// sameSignature reports whether a and b have identical parameter and result
// types (names may differ).
func sameSignature(fset *token.FileSet, a, b *ast.FuncDecl) bool {
	return sameFieldList(fset, a.Type.Params, b.Type.Params) &&
		sameFieldList(fset, a.Type.Results, b.Type.Results)
}

// sameFieldList compares two field lists by type string.
func sameFieldList(fset *token.FileSet, a, b *ast.FieldList) bool {
	as := fieldList(a)
	bs := fieldList(b)
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if typeString(fset, as[i].Type) != typeString(fset, bs[i].Type) {
			return false
		}
	}
	return true
}

// fieldList returns the fields or nil.
func fieldList(fl *ast.FieldList) []*ast.Field {
	if fl == nil {
		return nil
	}
	return fl.List
}

// typeString formats a type expression, or "" on error.
func typeString(fset *token.FileSet, t ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, t); err != nil {
		return ""
	}
	return buf.String()
}

// hasFuncLit reports whether n contains a function literal.
func hasFuncLit(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		if _, ok := x.(*ast.FuncLit); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

// hasNamedResults reports whether fn has named result parameters.
func hasNamedResults(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, f := range fn.Type.Results.List {
		if len(f.Names) > 0 {
			return true
		}
	}
	return false
}

// paramCtx carries the shared context for parameterizing a function pair.
type paramCtx struct {
	fset          *token.FileSet
	a, b          *ast.FuncDecl
	src           []byte
	r             *renames
	holes         []hole
	params        []holeParam
	extractedName string
}

// renames maps identifier names in a to the corresponding names in b,
// for params and locally-declared variables.
type renames struct {
	aToB    map[string]string
	localsA map[string]bool
	localsB map[string]bool
}

// buildRenames builds the rename mapping from a's locals to b's locals.
// It returns false if the declaration structures differ.
func buildRenames(a, b *ast.FuncDecl) (*renames, bool) {
	r := &renames{
		aToB:    map[string]string{},
		localsA: map[string]bool{},
		localsB: map[string]bool{},
	}
	if !mapParams(r, a, b) {
		return nil, false
	}
	if !mapLocals(r, a, b) {
		return nil, false
	}
	return r, true
}

// mapParams records the param name mapping.
func mapParams(r *renames, a, b *ast.FuncDecl) bool {
	pa := paramNames(a)
	pb := paramNames(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		r.aToB[pa[i]] = pb[i]
		r.localsA[pa[i]] = true
		r.localsB[pb[i]] = true
	}
	return true
}

// mapLocals records the local variable name mapping.
func mapLocals(r *renames, a, b *ast.FuncDecl) bool {
	la := declaredNames(a.Body)
	lb := declaredNames(b.Body)
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if existing, ok := r.aToB[la[i]]; ok && existing != lb[i] {
			return false
		}
		r.aToB[la[i]] = lb[i]
		r.localsA[la[i]] = true
		r.localsB[lb[i]] = true
	}
	return true
}

// paramNames returns the parameter names in order.
func paramNames(fn *ast.FuncDecl) []string {
	var out []string
	if fn.Type.Params == nil {
		return out
	}
	for _, f := range fn.Type.Params.List {
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

// declaredNames returns locally-declared variable names in order of
// appearance (from :=, var, range).
func declaredNames(body *ast.BlockStmt) []string {
	c := &nameCollector{seen: map[string]bool{}}
	ast.Inspect(body, c.visit)
	return c.out
}

// nameCollector accumulates declared names during an AST walk.
type nameCollector struct {
	out  []string
	seen map[string]bool
}

// visit records a name if unseen.
func (c *nameCollector) add(name string) {
	if !c.seen[name] {
		c.seen[name] = true
		c.out = append(c.out, name)
	}
}

// visit is the ast.Inspect callback.
func (c *nameCollector) visit(n ast.Node) bool {
	switch x := n.(type) {
	case *ast.AssignStmt:
		c.visitAssign(x)
	case *ast.DeclStmt:
		c.visitDecl(x)
	case *ast.RangeStmt:
		c.visitRange(x)
	}
	return true
}

// visitAssign records := targets.
func (c *nameCollector) visitAssign(x *ast.AssignStmt) {
	if x.Tok != token.DEFINE {
		return
	}
	for _, lhs := range x.Lhs {
		if id, ok := lhs.(*ast.Ident); ok {
			c.add(id.Name)
		}
	}
}

// visitDecl records var names.
func (c *nameCollector) visitDecl(x *ast.DeclStmt) {
	gd, ok := x.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, n := range vs.Names {
			c.add(n.Name)
		}
	}
}

// visitRange records range key/value names.
func (c *nameCollector) visitRange(x *ast.RangeStmt) {
	for _, e := range []ast.Expr{x.Key, x.Value} {
		if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
			c.add(id.Name)
		}
	}
}

// diffBlock compares two block statements, returning holes on structural match.
func diffBlock(a, b *ast.BlockStmt, r *renames) ([]hole, bool) {
	if len(a.List) != len(b.List) {
		return nil, false
	}
	var holes []hole
	for i := range a.List {
		h, ok := diffStmt(a.List[i], b.List[i], r)
		if !ok {
			return nil, false
		}
		holes = append(holes, h...)
	}
	return holes, true
}

// diffStmt compares two statements, dispatching by type.
func diffStmt(a, b ast.Stmt, r *renames) ([]hole, bool) {
	switch at := a.(type) {
	case *ast.ExprStmt:
		return diffExprStmt(at, b, r)
	case *ast.AssignStmt:
		return diffAssignStmt(at, b, r)
	case *ast.IfStmt:
		return diffIfStmt(at, b, r)
	case *ast.ForStmt:
		return diffForStmt(at, b, r)
	case *ast.ReturnStmt:
		return diffReturnStmt(at, b, r)
	}
	return diffStmtExtra(a, b, r)
}

// diffStmtExtra handles the less common statement types.
func diffStmtExtra(a, b ast.Stmt, r *renames) ([]hole, bool) {
	switch at := a.(type) {
	case *ast.RangeStmt:
		return diffRangeStmt(at, b, r)
	case *ast.IncDecStmt:
		return diffIncDecStmt(at, b, r)
	case *ast.BranchStmt:
		return diffBranchStmt(at, b, r)
	case *ast.DeclStmt:
		return diffDeclStmt(at, b, r)
	}
	return nil, false
}

// diffExprStmt compares expression statements.
func diffExprStmt(at *ast.ExprStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	return diffExpr(at.X, bt.X, r)
}

// diffAssignStmt compares assignments.
func diffAssignStmt(at *ast.AssignStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.AssignStmt)
	if !ok || !assignHeadersMatch(at, bt) {
		return nil, false
	}
	holes, ok := diffExprLists(at.Lhs, bt.Lhs, r)
	if !ok {
		return nil, false
	}
	h, ok := diffExprLists(at.Rhs, bt.Rhs, r)
	if !ok {
		return nil, false
	}
	return append(holes, h...), true
}

// assignHeadersMatch reports whether two assignments have the same shape.
func assignHeadersMatch(at, bt *ast.AssignStmt) bool {
	return at.Tok == bt.Tok &&
		len(at.Lhs) == len(bt.Lhs) &&
		len(at.Rhs) == len(bt.Rhs)
}

// diffExprLists compares two expression lists.
func diffExprLists(a, b []ast.Expr, r *renames) ([]hole, bool) {
	var holes []hole
	for i := range a {
		h, ok := diffExpr(a[i], b[i], r)
		if !ok {
			return nil, false
		}
		holes = append(holes, h...)
	}
	return holes, true
}

// diffIfStmt compares if statements.
func diffIfStmt(at *ast.IfStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.IfStmt)
	if !ok {
		return nil, false
	}
	return diffIf(at, bt, r)
}

// diffForStmt compares for loops.
func diffForStmt(at *ast.ForStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.ForStmt)
	if !ok || !forPartsMatch(at, bt) {
		return nil, false
	}
	holes, ok := diffForParts(at, bt, r)
	if !ok {
		return nil, false
	}
	h, ok := diffBlock(at.Body, bt.Body, r)
	if !ok {
		return nil, false
	}
	return append(holes, h...), true
}

// forPartsMatch reports whether init/cond/post are all present or all absent.
func forPartsMatch(at, bt *ast.ForStmt) bool {
	return (at.Init == nil) == (bt.Init == nil) &&
		(at.Cond == nil) == (bt.Cond == nil) &&
		(at.Post == nil) == (bt.Post == nil)
}

// diffForParts compares the init/cond/post of two for loops.
func diffForParts(at, bt *ast.ForStmt, r *renames) ([]hole, bool) {
	var holes []hole
	if h, ok := diffOptStmt(at.Init, bt.Init, r); !ok {
		return nil, false
	} else {
		holes = append(holes, h...)
	}
	if h, ok := diffOptExpr(at.Cond, bt.Cond, r); !ok {
		return nil, false
	} else {
		holes = append(holes, h...)
	}
	if h, ok := diffOptStmt(at.Post, bt.Post, r); !ok {
		return nil, false
	} else {
		holes = append(holes, h...)
	}
	return holes, true
}

// diffRangeStmt compares range loops.
func diffRangeStmt(at *ast.RangeStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.RangeStmt)
	if !ok || at.Tok != bt.Tok {
		return nil, false
	}
	holes, ok := diffRangeExprs(at, bt, r)
	if !ok {
		return nil, false
	}
	h, ok := diffBlock(at.Body, bt.Body, r)
	if !ok {
		return nil, false
	}
	return append(holes, h...), true
}

// diffRangeExprs compares the key/value/x of two range statements.
func diffRangeExprs(at, bt *ast.RangeStmt, r *renames) ([]hole, bool) {
	var holes []hole
	for _, pair := range [][2]ast.Expr{{at.Key, bt.Key}, {at.Value, bt.Value}, {at.X, bt.X}} {
		h, ok := diffOptExpr(pair[0], pair[1], r)
		if !ok {
			return nil, false
		}
		holes = append(holes, h...)
	}
	return holes, true
}

// diffOptExpr compares two optional expressions (both nil or both non-nil).
func diffOptExpr(a, b ast.Expr, r *renames) ([]hole, bool) {
	if (a == nil) != (b == nil) {
		return nil, false
	}
	if a == nil {
		return nil, true
	}
	return diffExpr(a, b, r)
}

// diffOptStmt compares two optional statements (both nil or both non-nil).
func diffOptStmt(a, b ast.Stmt, r *renames) ([]hole, bool) {
	if (a == nil) != (b == nil) {
		return nil, false
	}
	if a == nil {
		return nil, true
	}
	return diffStmt(a, b, r)
}

// diffReturnStmt compares return statements.
func diffReturnStmt(at *ast.ReturnStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.ReturnStmt)
	if !ok || len(at.Results) != len(bt.Results) {
		return nil, false
	}
	var holes []hole
	for i := range at.Results {
		h, ok := diffExpr(at.Results[i], bt.Results[i], r)
		if !ok {
			return nil, false
		}
		holes = append(holes, h...)
	}
	return holes, true
}

// diffIncDecStmt compares ++/-- statements.
func diffIncDecStmt(at *ast.IncDecStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.IncDecStmt)
	if !ok || at.Tok != bt.Tok {
		return nil, false
	}
	return diffExpr(at.X, bt.X, r)
}

// diffBranchStmt compares break/continue.
func diffBranchStmt(at *ast.BranchStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.BranchStmt)
	if !ok || at.Tok != bt.Tok {
		return nil, false
	}
	return nil, true
}

// diffDeclStmt compares declarations by formatted text (v1: require identical).
func diffDeclStmt(at *ast.DeclStmt, b ast.Stmt, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.DeclStmt)
	if !ok {
		return nil, false
	}
	var abuf, bbuf bytes.Buffer
	fset := token.NewFileSet()
	if err := format.Node(&abuf, fset, at); err != nil {
		return nil, false
	}
	if err := format.Node(&bbuf, fset, bt); err != nil {
		return nil, false
	}
	if abuf.String() != bbuf.String() {
		return nil, false
	}
	return nil, true
}

// diffIf compares two if statements.
func diffIf(a, b *ast.IfStmt, r *renames) ([]hole, bool) {
	if (a.Init == nil) != (b.Init == nil) {
		return nil, false
	}
	holes, ok := diffIfHead(a, b, r)
	if !ok {
		return nil, false
	}
	h, ok := diffIfElse(a, b, r)
	if !ok {
		return nil, false
	}
	return append(holes, h...), true
}

// diffIfHead compares the init, cond, and body of two ifs.
func diffIfHead(a, b *ast.IfStmt, r *renames) ([]hole, bool) {
	var holes []hole
	if a.Init != nil {
		h, ok := diffStmt(a.Init, b.Init, r)
		if !ok {
			return nil, false
		}
		holes = append(holes, h...)
	}
	h, ok := diffExpr(a.Cond, b.Cond, r)
	if !ok {
		return nil, false
	}
	holes = append(holes, h...)
	h, ok = diffBlock(a.Body, b.Body, r)
	if !ok {
		return nil, false
	}
	return append(holes, h...), true
}

// diffIfElse compares the else branches (must be plain blocks).
func diffIfElse(a, b *ast.IfStmt, r *renames) ([]hole, bool) {
	if (a.Else == nil) != (b.Else == nil) {
		return nil, false
	}
	if a.Else == nil {
		return nil, true
	}
	ab, aok := a.Else.(*ast.BlockStmt)
	bb, bok := b.Else.(*ast.BlockStmt)
	if !aok || !bok {
		return nil, false
	}
	return diffBlock(ab, bb, r)
}

// diffExpr compares two expressions, dispatching by type.
func diffExpr(a, b ast.Expr, r *renames) ([]hole, bool) {
	if a == nil || b == nil {
		return nil, a == b
	}
	if h, ok, done := diffLeaf(a, b, r); done {
		return h, ok
	}
	return diffExprSwitch(a, b, r)
}

// diffExprSwitch dispatches composite expressions by type.
func diffExprSwitch(a, b ast.Expr, r *renames) ([]hole, bool) {
	switch at := a.(type) {
	case *ast.CallExpr:
		return diffCallExpr(at, b, r)
	case *ast.SelectorExpr:
		return diffSelectorExpr(at, b, r)
	case *ast.BinaryExpr:
		return diffBinaryExpr(at, b, r)
	}
	return diffExprExtra(a, b, r)
}

// diffExprExtra handles the remaining expression types.
func diffExprExtra(a, b ast.Expr, r *renames) ([]hole, bool) {
	switch at := a.(type) {
	case *ast.UnaryExpr:
		return diffUnaryExpr(at, b, r)
	case *ast.ParenExpr:
		return diffParenExpr(at, b, r)
	}
	return nil, false
}

// diffLeaf handles identifiers and basic literals. It returns done=true when
// the nodes were leaves (handled), with the holes and match result.
func diffLeaf(a, b ast.Expr, r *renames) ([]hole, bool, bool) {
	if _, ok := a.(*ast.Ident); ok {
		return diffIdentLeaf(a, b, r)
	}
	if _, ok := a.(*ast.BasicLit); ok {
		return diffLitLeaf(a, b, r)
	}
	return nil, false, false
}

// diffIdentLeaf handles identifier leaves.
func diffIdentLeaf(a, b ast.Expr, r *renames) ([]hole, bool, bool) {
	ai := a.(*ast.Ident)
	bi, ok := b.(*ast.Ident)
	if !ok {
		return nil, false, true
	}
	if ai.Name == bi.Name {
		return nil, true, true
	}
	return diffRenamedIdent(ai, bi, r)
}

// diffRenamedIdent handles identifiers that differ by name.
func diffRenamedIdent(ai, bi *ast.Ident, r *renames) ([]hole, bool, bool) {
	if r.localsA[ai.Name] && r.localsB[bi.Name] {
		return nil, r.aToB[ai.Name] == bi.Name, true
	}
	if !r.localsA[ai.Name] && !r.localsB[bi.Name] {
		return []hole{{aExpr: ai, bExpr: bi}}, true, true
	}
	return nil, false, true
}

// diffLitLeaf handles basic literal leaves.
func diffLitLeaf(a, b ast.Expr, r *renames) ([]hole, bool, bool) {
	al := a.(*ast.BasicLit)
	bl, ok := b.(*ast.BasicLit)
	if !ok || al.Kind != bl.Kind {
		return nil, false, true
	}
	if al.Value == bl.Value {
		return nil, true, true
	}
	return []hole{{aExpr: a, bExpr: b}}, true, true
}

// diffCallExpr compares call expressions.
func diffCallExpr(at *ast.CallExpr, b ast.Expr, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.CallExpr)
	if !ok || len(at.Args) != len(bt.Args) {
		return nil, false
	}
	var holes []hole
	h, ok := diffExpr(at.Fun, bt.Fun, r)
	if !ok {
		return nil, false
	}
	holes = append(holes, h...)
	for i := range at.Args {
		h, ok := diffExpr(at.Args[i], bt.Args[i], r)
		if !ok {
			return nil, false
		}
		holes = append(holes, h...)
	}
	return holes, true
}

// diffSelectorExpr compares selector expressions.
func diffSelectorExpr(at *ast.SelectorExpr, b ast.Expr, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.SelectorExpr)
	if !ok || at.Sel.Name != bt.Sel.Name {
		return nil, false
	}
	return diffExpr(at.X, bt.X, r)
}

// diffBinaryExpr compares binary expressions.
func diffBinaryExpr(at *ast.BinaryExpr, b ast.Expr, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.BinaryExpr)
	if !ok || at.Op != bt.Op {
		return nil, false
	}
	var holes []hole
	h, ok := diffExpr(at.X, bt.X, r)
	if !ok {
		return nil, false
	}
	holes = append(holes, h...)
	h, ok = diffExpr(at.Y, bt.Y, r)
	if !ok {
		return nil, false
	}
	return append(holes, h...), true
}

// diffUnaryExpr compares unary expressions.
func diffUnaryExpr(at *ast.UnaryExpr, b ast.Expr, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.UnaryExpr)
	if !ok || at.Op != bt.Op {
		return nil, false
	}
	return diffExpr(at.X, bt.X, r)
}

// diffParenExpr compares parenthesized expressions.
func diffParenExpr(at *ast.ParenExpr, b ast.Expr, r *renames) ([]hole, bool) {
	bt, ok := b.(*ast.ParenExpr)
	if !ok {
		return nil, false
	}
	return diffExpr(at.X, bt.X, r)
}

// helperName derives the extracted function name from the two originals.
// It uses the longest common affix (prefix or suffix), unexported.
func helperName(a, b string) string {
	prefix := commonPrefix(a, b)
	suffix := commonSuffix(a, b)
	name := prefix
	if len(suffix) > len(prefix) {
		name = suffix
	}
	if name == "" {
		return "extracted"
	}
	// Unexport: lowercase first letter.
	runes := []rune(name)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// commonPrefix returns the longest common prefix.
func commonPrefix(a, b string) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return a[:i]
}

// commonSuffix returns the longest common suffix.
func commonSuffix(a, b string) string {
	ia, ib := len(a)-1, len(b)-1
	for ia >= 0 && ib >= 0 && a[ia] == b[ib] {
		ia--
		ib--
	}
	return a[ia+1:]
}

// holeParam is a helper parameter derived from a hole.
type holeParam struct {
	name  string // param name in helper
	typ   string // param type string
	aExpr string // original expr text in a
	bExpr string // original expr text in b
}

// holeParams builds the helper parameters from holes.
func holeParams(ctx *paramCtx) ([]holeParam, bool) {
	var params []holeParam
	for i, h := range ctx.holes {
		ai := h.aExpr.(*ast.Ident)
		bi := h.bExpr.(*ast.Ident)
		typ, ok := inferHoleFuncType(ctx, ai, bi)
		if !ok {
			return nil, false
		}
		params = append(params, holeParam{
			name:  fmt.Sprintf("fn%d", i+1),
			typ:   typ,
			aExpr: ai.Name,
			bExpr: bi.Name,
		})
	}
	if len(params) == 1 {
		params[0].name = "fn"
	}
	return params, true
}

// writeHelperSig writes the helper function signature.
func writeHelperSig(out *bytes.Buffer, ctx *paramCtx) {
	out.WriteString("func " + ctx.extractedName + "(")
	writeHoleParams(out, ctx.params)
	writeOrigParams(out, ctx.fset, ctx.a, len(ctx.params) > 0)
	out.WriteString(")")
	if res := resultString(ctx.fset, ctx.a); res != "" {
		out.WriteString(" " + res)
	}
	out.WriteString(" {\n")
}

// writeHoleParams writes the hole-derived parameters.
func writeHoleParams(out *bytes.Buffer, params []holeParam) {
	for i, p := range params {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.name + " " + p.typ)
	}
}

// writeOrigParams writes the original function parameters.
func writeOrigParams(out *bytes.Buffer, fset *token.FileSet, a *ast.FuncDecl, hasHoleParams bool) {
	origParams := paramNames(a)
	origTypes := paramTypes(fset, a)
	for i, n := range origParams {
		if hasHoleParams || i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(n + " " + origTypes[i])
	}
}

// buildReplacement generates the replacement source: the extracted helper
// followed by the two thin wrappers.
func buildReplacement(ctx *paramCtx) ([]byte, bool) {
	params, ok := holeParams(ctx)
	if !ok {
		return nil, false
	}
	ctx.params = params

	var out bytes.Buffer
	writeHelperSig(&out, ctx)

	bodyText, ok := buildHelperBody(ctx)
	if !ok {
		return nil, false
	}
	out.Write(bodyText)
	out.WriteString("}\n\n")

	// --- Wrapper for a ---
	writeWrapper(&out, ctx, ctx.a, true)
	out.WriteString("\n")
	// --- Wrapper for b ---
	writeWrapper(&out, ctx, ctx.b, false)

	return out.Bytes(), true
}

// paramTypes returns the type strings for a function's params.
func paramTypes(fset *token.FileSet, fn *ast.FuncDecl) []string {
	var out []string
	if fn.Type.Params == nil {
		return out
	}
	for _, f := range fn.Type.Params.List {
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f.Type); err != nil {
			return nil
		}
		for range f.Names {
			out = append(out, buf.String())
		}
	}
	return out
}

// resultString returns the result type string, e.g. "(string, error)".
func resultString(fset *token.FileSet, fn *ast.FuncDecl) string {
	if fn.Type.Results == nil {
		return ""
	}
	parts := resultParts(fset, fn)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// resultParts returns the individual result type strings.
func resultParts(fset *token.FileSet, fn *ast.FuncDecl) []string {
	var parts []string
	for _, f := range fn.Type.Results.List {
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f.Type); err != nil {
			return nil
		}
		for _, n := range f.Names {
			parts = append(parts, n.Name+" "+buf.String())
		}
		if len(f.Names) == 0 {
			parts = append(parts, buf.String())
		}
	}
	return parts
}

// inferHoleFuncType infers the function type for a hole identifier by
// finding its call site in both functions.
func inferHoleFuncType(ctx *paramCtx, ai, bi *ast.Ident) (string, bool) {
	callA := findCall(ctx.a.Body, ai.Name)
	callB := findCall(ctx.b.Body, bi.Name)
	if callA == nil || callB == nil {
		return "", false
	}
	argTypes, ok := callArgTypes(ctx.fset, ctx.a, callA)
	if !ok {
		return "", false
	}
	resTypes := resultTypes(ctx.fset, ctx.a)
	if len(resTypes) == 0 {
		return "", false
	}
	_ = callB
	return fmt.Sprintf("func(%s) (%s)", strings.Join(argTypes, ", "), strings.Join(resTypes, ", ")), true
}

// callArgTypes infers the argument types of a call (must be param idents).
func callArgTypes(fset *token.FileSet, fn *ast.FuncDecl, call *ast.CallExpr) ([]string, bool) {
	var argTypes []string
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok {
			return nil, false
		}
		typ, ok := paramTypeOf(fset, fn, id.Name)
		if !ok {
			return nil, false
		}
		argTypes = append(argTypes, typ)
	}
	return argTypes, true
}

// findCall finds the first call expression calling the named function.
func findCall(n ast.Node, name string) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(n, func(x ast.Node) bool {
		if call, ok := x.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
				found = call
				return false
			}
		}
		return true
	})
	return found
}

// paramTypeOf returns the type string for a named param.
func paramTypeOf(fset *token.FileSet, fn *ast.FuncDecl, name string) (string, bool) {
	if fn.Type.Params == nil {
		return "", false
	}
	for _, f := range fn.Type.Params.List {
		for _, n := range f.Names {
			if n.Name == name {
				var buf bytes.Buffer
				if err := format.Node(&buf, fset, f.Type); err != nil {
					return "", false
				}
				return buf.String(), true
			}
		}
	}
	return "", false
}

// resultTypes returns the result type strings.
func resultTypes(fset *token.FileSet, fn *ast.FuncDecl) []string {
	var out []string
	if fn.Type.Results == nil {
		return out
	}
	for _, f := range fn.Type.Results.List {
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f.Type); err != nil {
			return nil
		}
		out = append(out, buf.String())
	}
	return out
}

// buildHelperBody builds the helper body text from a's body, replacing
// holes with param names.
func buildHelperBody(ctx *paramCtx) ([]byte, bool) {
	body := cloneBlock(ctx.a.Body)
	if body == nil {
		return nil, false
	}
	rewriteHoles(body, ctx.holes, ctx.params)
	return formatStmts(ctx.fset, body.List)
}

// rewriteHoles renames hole identifiers to param names in the cloned body.
func rewriteHoles(body *ast.BlockStmt, holes []hole, params []holeParam) {
	holeMap := map[string]string{}
	for i, h := range holes {
		ai := h.aExpr.(*ast.Ident)
		holeMap[ai.Name] = params[i].name
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if p, ok := holeMap[id.Name]; ok {
				id.Name = p
			}
		}
		return true
	})
}

// formatStmts formats a statement list, one per line.
func formatStmts(fset *token.FileSet, stmts []ast.Stmt) ([]byte, bool) {
	var buf bytes.Buffer
	for _, stmt := range stmts {
		if err := format.Node(&buf, fset, stmt); err != nil {
			return nil, false
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes(), true
}

// cloneBlock deep-copies a block statement (via format+parse).
func cloneBlock(b *ast.BlockStmt) *ast.BlockStmt {
	fset := token.NewFileSet()
	var buf bytes.Buffer
	buf.WriteString("package p\nfunc f() ")
	if err := format.Node(&buf, fset, b); err != nil {
		return nil
	}
	f, err := parser.ParseFile(fset, "clone.go", buf.Bytes(), 0)
	if err != nil {
		return nil
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn.Body
		}
	}
	return nil
}

// writeWrapper writes a thin wrapper function that delegates to the helper.
func writeWrapper(out *bytes.Buffer, ctx *paramCtx, fn *ast.FuncDecl, isA bool) {
	out.WriteString("func " + fn.Name.Name + "(")
	// Original params.
	out.WriteString(paramString(ctx.fset, fn))
	out.WriteString(")")
	if res := resultString(ctx.fset, fn); res != "" {
		out.WriteString(" " + res)
	}
	out.WriteString(" {\n")
	// Body: return extracted(holeArgs..., origArgs...)
	out.WriteString("\treturn " + ctx.extractedName + "(")
	var args []string
	for _, h := range ctx.holes {
		if isA {
			args = append(args, h.aExpr.(*ast.Ident).Name)
		} else {
			args = append(args, h.bExpr.(*ast.Ident).Name)
		}
	}
	for _, n := range paramNames(fn) {
		args = append(args, n)
	}
	out.WriteString(strings.Join(args, ", "))
	out.WriteString(")\n}\n")
}

// paramString returns the parameter list as "name type, name type".
func paramString(fset *token.FileSet, fn *ast.FuncDecl) string {
	if fn.Type.Params == nil {
		return ""
	}
	var parts []string
	for _, f := range fn.Type.Params.List {
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, f.Type); err != nil {
			return ""
		}
		for _, n := range f.Names {
			parts = append(parts, n.Name+" "+buf.String())
		}
	}
	return strings.Join(parts, ", ")
}

// antiUnifyHoles uses anti-unification to find the differing parts between
// two function bodies. Returns the holes (differing expressions) for
// parameter extraction. This is the principled replacement for the
// heuristic diffBlock: anti-unification gives the most specific template.
func antiUnifyHoles(a, b *ast.BlockStmt, r *renames) ([]hole, bool) {
	// Build the rename map for anti-unification.
	renameMap := map[string]string{}
	for aName, bName := range r.aToB {
		renameMap[aName] = bName
	}
	_, subs := patterns.AntiUnifyWithRenames(a, b, renameMap)
	if len(subs) == 0 {
		return nil, false // Identical bodies; nothing to parameterize.
	}
	var holes []hole
	for _, sub := range subs {
		// Holes must be expressions to become parameters.
		// If anti-unification produced a non-expression hole (e.g., a
		// statement), the bodies differ structurally and can't be
		// parameterized.
		aExpr, okA := sub.A.(ast.Expr)
		bExpr, okB := sub.B.(ast.Expr)
		if !okA || !okB {
			return nil, false
		}
		holes = append(holes, hole{aExpr: aExpr, bExpr: bExpr})
	}
	return holes, true
}
