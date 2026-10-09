package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// gostaticanalysis extractors: nilerr, forcetypeassert, typednil.

var errorType = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

// findNilErrHits finds `if err != nil { return nil }` and
// `if err == nil { return err }` patterns.
func findNilErrHits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.NilErrHit {
	var out []patterns.NilErrHit
	if fn.Body == nil {
		return out
	}
	errPos := errorResultPosition(fn, info)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if hit := checkNilErrIf(ifStmt, fset, info, errPos); hit != nil {
			out = append(out, *hit)
		}
		return true
	})
	return out
}

// errorResultPosition returns the index of the error-typed result in the
// function's signature, or -1 if there is none. For `return nil, err` in a
// `(T, error)` function, position 1 is the error — the nil at position 0
// is the value, not a swallowed error.
func errorResultPosition(fn *ast.FuncDecl, info *types.Info) int {
	if fn.Type == nil || fn.Type.Results == nil {
		return -1
	}
	if pos := errorPosFromTypes(fn, info); pos >= 0 {
		return pos
	}
	return errorPosFromAST(fn)
}

// errorPosFromTypes resolves the error position via types.Info.
func errorPosFromTypes(fn *ast.FuncDecl, info *types.Info) int {
	if info == nil {
		return -1
	}
	sig, ok := info.TypeOf(fn.Name).(*types.Signature)
	if !ok {
		return -1
	}
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		if isNilErrErrorType(results.At(i).Type()) {
			return i
		}
	}
	return -1
}

// errorPosFromAST falls back to inspecting AST result types for `error`.
func errorPosFromAST(fn *ast.FuncDecl) int {
	pos := 0
	for _, field := range fn.Type.Results.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if isErrorASTType(field.Type) {
			return pos
		}
		pos += count
	}
	return -1
}

// isNilErrErrorType reports whether t is the error interface type.
func isNilErrErrorType(t types.Type) bool {
	if t == nil {
		return false
	}
	iface, ok := t.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	// error is the interface with exactly the Error() string method.
	return iface.NumMethods() == 1 && iface.Method(0).Name() == "Error"
}

// isErrorASTType reports whether the AST type expression is `error`.
func isErrorASTType(e ast.Expr) bool {
	ident, ok := e.(*ast.Ident)
	return ok && ident.Name == "error"
}

// nilErrCheck holds the parsed `if err != nil` / `if err == nil` condition.
type nilErrCheck struct {
	errVal   ast.Expr
	isNotNil bool
}

// checkNilErrIf checks one if statement for the nilerr pattern.
func checkNilErrIf(ifStmt *ast.IfStmt, fset *token.FileSet, info *types.Info, errPos int) *patterns.NilErrHit {
	check := parseNilErrCond(ifStmt.Cond, info)
	if check == nil {
		return nil
	}
	branch := nilErrBranch(ifStmt, check.isNotNil)
	if branch == nil {
		return nil
	}
	ret := findReturnInBranch(branch)
	if ret == nil {
		return nil
	}
	ctx := nilRetCtx{ifStmt: ifStmt, fset: fset, ret: ret, info: info, errPos: errPos}
	if check.isNotNil {
		return checkNotNilReturn(ctx, branch, check)
	}
	return checkNilReturn(ctx, check)
}

// nilRetCtx bundles the shared context for the nil-return checkers.
type nilRetCtx struct {
	ifStmt *ast.IfStmt
	fset   *token.FileSet
	ret    *ast.ReturnStmt
	info   *types.Info
	errPos int
}

// checkNotNilReturn checks `if err != nil { return nil }`.
func checkNotNilReturn(ctx nilRetCtx, branch ast.Stmt, check *nilErrCheck) *patterns.NilErrHit {
	if returnsNilError(ctx.ret, ctx.errPos) && !usesErrorValue(branch, check.errVal, ctx.info) {
		return &patterns.NilErrHit{
			Line:     ctx.fset.Position(ctx.ifStmt.Pos()).Line,
			CondText: condText(ctx.fset, ctx.ifStmt.Cond),
			Kind:     "returns nil when err != nil",
		}
	}
	return nil
}

// checkNilReturn checks `if err == nil { return err }`.
func checkNilReturn(ctx nilRetCtx, check *nilErrCheck) *patterns.NilErrHit {
	if returnsErrValueAt(ctx.ret, check.errVal, ctx.errPos, ctx.info) {
		return &patterns.NilErrHit{
			Line:     ctx.fset.Position(ctx.ifStmt.Pos()).Line,
			CondText: condText(ctx.fset, ctx.ifStmt.Cond),
			Kind:     "returns err when err == nil",
		}
	}
	return nil
}

// parseNilErrCond parses `err != nil` or `err == nil`, returning the error
// value and whether it's the != form.
func parseNilErrCond(cond ast.Expr, info *types.Info) *nilErrCheck {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	if bin.Op == token.NEQ {
		if errVal := nilErrOperand(bin, info); errVal != nil {
			return &nilErrCheck{errVal: errVal, isNotNil: true}
		}
	} else if bin.Op == token.EQL {
		if errVal := nilErrOperand(bin, info); errVal != nil {
			return &nilErrCheck{errVal: errVal, isNotNil: false}
		}
	}
	return nil
}

// nilErrOperand returns the error-valued side of `err != nil` / `err == nil`.
func nilErrOperand(bin *ast.BinaryExpr, info *types.Info) ast.Expr {
	if isNilIdent(bin.Y) && isErrorExpr(info, bin.X) {
		return bin.X
	}
	if isNilIdent(bin.X) && isErrorExpr(info, bin.Y) {
		return bin.Y
	}
	return nil
}

// nilErrBranch returns the branch to check: Body for !=, Else for ==.
func nilErrBranch(ifStmt *ast.IfStmt, isNotNil bool) ast.Stmt {
	if isNotNil {
		return ifStmt.Body
	}
	return ifStmt.Else
}

// isNil reports whether e is the nil identifier.
func isNilIdent(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

// isErrorType reports whether e has an error type.
func isErrorExpr(info *types.Info, e ast.Expr) bool {
	if info == nil {
		return false
	}
	t := info.TypeOf(e)
	if t == nil {
		return false
	}
	return types.Implements(t, errorType)
}

// findReturnInBranch finds the first return statement in a branch.
func findReturnInBranch(branch ast.Stmt) *ast.ReturnStmt {
	var ret *ast.ReturnStmt
	ast.Inspect(branch, func(n ast.Node) bool {
		if r, ok := n.(*ast.ReturnStmt); ok {
			ret = r
			return false
		}
		return true
	})
	return ret
}

// returnsNilError reports whether ret returns nil at the error result
// position. errPos is the index of the error-typed result (-1 if none).
// For `return nil, err` in a `(T, error)` function, the nil is at position 0
// (the value), not the error — so this returns false.
func returnsNilError(ret *ast.ReturnStmt, errPos int) bool {
	if errPos < 0 || errPos >= len(ret.Results) {
		return false
	}
	return isNilIdent(ret.Results[errPos])
}

// returnsErrValueAt reports whether ret returns the errVal at the error
// result position.
func returnsErrValueAt(ret *ast.ReturnStmt, errVal ast.Expr, errPos int, info *types.Info) bool {
	if errPos < 0 || errPos >= len(ret.Results) {
		return false
	}
	return exprName(ret.Results[errPos]) == exprName(errVal)
}

// usesErrorValue reports whether the error value is used in the branch
// (e.g. log(err)). If used, it's not a nilerr bug.
func usesErrorValue(branch ast.Stmt, errVal ast.Expr, info *types.Info) bool {
	errName := exprName(errVal)
	used := false
	ast.Inspect(branch, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			for _, arg := range call.Args {
				if exprName(arg) == errName {
					used = true
					return false
				}
			}
		}
		return true
	})
	return used
}

// exprName returns the identifier name for a simple expression.
func exprName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// condText returns the source text of a condition.
func condText(fset *token.FileSet, cond ast.Expr) string {
	var sb strings.Builder
	// Simple printer: just use the position info.
	// For the spike, we'll use a basic format.
	sb.WriteString("(condition)")
	return sb.String()
}

// findForceTypeAssertHits finds unchecked `x.(T)` type assertions.
func findForceTypeAssertHits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.ForceTypeAssertHit {
	var out []patterns.ForceTypeAssertHit
	if fn.Body == nil {
		return out
	}
	commaOk := collectCommaOk(fn.Body)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if hit := checkTypeAssert(n, fset, info, commaOk); hit != nil {
			out = append(out, *hit)
		}
		return true
	})
	return out
}

// collectCommaOk finds type assertions in comma-ok form: `v, ok := x.(T)`.
func collectCommaOk(body ast.Node) map[token.Pos]bool {
	commaOk := map[token.Pos]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		collectAssignCommaOk(n, commaOk)
		collectValueSpecCommaOk(n, commaOk)
		return true
	})
	return commaOk
}

// collectAssignCommaOk marks assertions in `v, ok := x.(T)`.
func collectAssignCommaOk(n ast.Node, commaOk map[token.Pos]bool) {
	assign, ok := n.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 2 {
		return
	}
	for _, rhs := range assign.Rhs {
		markAssertExprs(rhs, commaOk)
	}
}

// collectValueSpecCommaOk marks assertions in `var v, ok = x.(T)`.
func collectValueSpecCommaOk(n ast.Node, commaOk map[token.Pos]bool) {
	vs, ok := n.(*ast.ValueSpec)
	if !ok || len(vs.Names) != 2 {
		return
	}
	for _, v := range vs.Values {
		markAssertExprs(v, commaOk)
	}
}

// markAssertExprs marks all type assertions under e.
func markAssertExprs(e ast.Expr, commaOk map[token.Pos]bool) {
	ast.Inspect(e, func(m ast.Node) bool {
		if tae, ok := m.(*ast.TypeAssertExpr); ok {
			commaOk[tae.Pos()] = true
		}
		return true
	})
}

// checkTypeAssert checks one node for an unchecked assertion.
func checkTypeAssert(n ast.Node, fset *token.FileSet, info *types.Info, commaOk map[token.Pos]bool) *patterns.ForceTypeAssertHit {
	tae, ok := n.(*ast.TypeAssertExpr)
	if !ok || commaOk[tae.Pos()] || tae.Type == nil {
		return nil
	}
	if isEmptyInterface(info, tae.Type) {
		return nil
	}
	return &patterns.ForceTypeAssertHit{
		Line:     fset.Position(tae.Pos()).Line,
		ExprText: "(type assertion)",
	}
}

// isEmptyInterface reports whether e is `any` or empty interface.
func isEmptyInterface(info *types.Info, e ast.Expr) bool {
	if info == nil {
		return false
	}
	t := info.TypeOf(e)
	if t == nil {
		return false
	}
	iface, ok := t.Underlying().(*types.Interface)
	return ok && iface.Empty()
}

// findTypedNilHits finds typed-nil vs untyped-nil comparisons.
// Tracks when a typed nil-able (pointer, etc.) is assigned to an interface
// and later compared to nil — the comparison is always false.
func findTypedNilHits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.TypedNilHit {
	var out []patterns.TypedNilHit
	if fn.Body == nil || info == nil {
		return out
	}
	typedVars := collectTypedVars(fn.Body, info)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if hit := checkTypedNilCompare(n, fset, typedVars); hit != nil {
			out = append(out, *hit)
		}
		return true
	})
	return out
}

// collectTypedVars finds variables assigned a typed nil-able value.
func collectTypedVars(body ast.Node, info *types.Info) map[string]bool {
	typedVars := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		collectValueSpecVars(n, info, typedVars)
		collectAssignVars(n, info, typedVars)
		return true
	})
	return typedVars
}

// collectValueSpecVars tracks `var x any = p` where p is nil-able.
func collectValueSpecVars(n ast.Node, info *types.Info, typedVars map[string]bool) {
	vs, ok := n.(*ast.ValueSpec)
	if !ok {
		return
	}
	for i, name := range vs.Names {
		if i < len(vs.Values) && isTypedNilable(info, vs.Values[i]) {
			typedVars[name.Name] = true
		}
	}
}

// collectAssignVars tracks `x := p` or `x = p` where p is nil-able.
func collectAssignVars(n ast.Node, info *types.Info, typedVars map[string]bool) {
	assign, ok := n.(*ast.AssignStmt)
	if !ok {
		return
	}
	for i, lhs := range assign.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && i < len(assign.Rhs) {
			if isTypedNilable(info, assign.Rhs[i]) {
				typedVars[id.Name] = true
			}
		}
	}
}

// checkTypedNilCompare checks one node for a typed-nil comparison.
func checkTypedNilCompare(n ast.Node, fset *token.FileSet, typedVars map[string]bool) *patterns.TypedNilHit {
	bin, ok := n.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
		return nil
	}
	if name := typedNilVar(bin.X, bin.Y, typedVars); name != "" {
		return &patterns.TypedNilHit{
			Line:     fset.Position(bin.Pos()).Line,
			ExprText: name + " may hold typed nil",
		}
	}
	if name := typedNilVar(bin.Y, bin.X, typedVars); name != "" {
		return &patterns.TypedNilHit{
			Line:     fset.Position(bin.Pos()).Line,
			ExprText: name + " may hold typed nil",
		}
	}
	return nil
}

// typedNilVar returns the var name if v is a tracked var and other is nil.
func typedNilVar(v, other ast.Expr, typedVars map[string]bool) string {
	id, ok := v.(*ast.Ident)
	if ok && typedVars[id.Name] && isNilIdent(other) {
		return id.Name
	}
	return ""
}

// isTypedNilable reports whether e has a type that can hold a typed nil
// (pointer, interface, slice, map, chan, func).
func isTypedNilable(info *types.Info, e ast.Expr) bool {
	t := info.TypeOf(e)
	if t == nil {
		return false
	}
	switch t.(type) {
	case *types.Pointer, *types.Interface, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return true
	}
	return false
}

// findSuppressedKinds extracts suppressed pattern kinds from the function's
// doc comment. Supports both the standard lint-ignore comment form and the
// legacy allowlist form.
func findSuppressedKinds(fn *ast.FuncDecl) []string {
	if fn.Doc == nil {
		return nil
	}
	var kinds []string
	for _, c := range fn.Doc.List {
		kinds = append(kinds, parseSuppressComment(c.Text)...)
	}
	return kinds
}

// parseSuppressComment extracts kind IDs from one comment line.
func parseSuppressComment(text string) []string {
	if strings.HasPrefix(text, "//lint:ignore") {
		return parseLintIgnoreKinds(text)
	}
	if strings.HasPrefix(text, "// cyclo-allow(") {
		return parseCycloAllowKinds(text)
	}
	return nil
}

// parseLintIgnoreKinds extracts kinds from `//lint:ignore k1, k2 reason`.
func parseLintIgnoreKinds(text string) []string {
	rest := strings.TrimSpace(strings.TrimPrefix(text, "//lint:ignore"))
	var kinds []string
	for _, p := range strings.Fields(rest) {
		clean := strings.TrimSuffix(p, ",")
		if !isPatternKind(clean) {
			break // Reason starts.
		}
		kinds = append(kinds, clean)
	}
	return kinds
}

// parseCycloAllowKinds extracts kinds from the legacy allowlist comment form.
func parseCycloAllowKinds(text string) []string {
	inner := strings.TrimPrefix(text, "// cyclo-allow(")
	idx := strings.Index(inner, "):")
	if idx < 0 {
		return nil
	}
	var kinds []string
	for _, r := range strings.Split(inner[:idx], ",") {
		r = strings.TrimSpace(r)
		if isPatternKind(r) {
			kinds = append(kinds, r)
		}
	}
	return kinds
}

// isPatternKind reports whether s is a known pattern kind ID.
func isPatternKind(s string) bool {
	switch s {
	case "nilerr", "forcetypeassert", "typednil":
		return true
	}
	return false
}
