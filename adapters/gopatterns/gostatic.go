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
// the equivalent `if err == nil { ... } else { return nil }` pattern.
func findNilErrHits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.NilErrHit {
	var out []patterns.NilErrHit
	if fn.Body == nil {
		return out
	}
	errPositions := errorResultPositions(fn, info)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if hit := checkNilErrIf(ifStmt, fset, info, errPositions); hit != nil {
			out = append(out, *hit)
		}
		return true
	})
	return out
}

// errorResultPositions returns every error result, including named results.
func errorResultPositions(fn *ast.FuncDecl, info *types.Info) []int {
	if fn.Type == nil || fn.Type.Results == nil {
		return nil
	}
	if info != nil {
		if object, ok := info.Defs[fn.Name].(*types.Func); ok {
			return signatureErrorPositions(object.Type().(*types.Signature))
		}
	}
	return astErrorPositions(fn.Type.Results)
}

func signatureErrorPositions(sig *types.Signature) []int {
	var positions []int
	for i := 0; i < sig.Results().Len(); i++ {
		if isNilErrErrorType(sig.Results().At(i).Type()) {
			positions = append(positions, i)
		}
	}
	return positions
}

func astErrorPositions(results *ast.FieldList) []int {
	var positions []int
	pos := 0
	for _, field := range results.List {
		count := max(len(field.Names), 1)
		if isErrorASTType(field.Type) {
			for i := 0; i < count; i++ {
				positions = append(positions, pos+i)
			}
		}
		pos += count
	}
	return positions
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
	return types.Identical(iface, errorType)
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
func checkNilErrIf(ifStmt *ast.IfStmt, fset *token.FileSet, info *types.Info, errPositions []int) *patterns.NilErrHit {
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
	if !returnsNilErrors(ret, errPositions) || usesErrorValue(branch, check.errVal, info) {
		return nil
	}
	return &patterns.NilErrHit{
		Line:     fset.Position(ifStmt.Pos()).Line,
		CondText: condText(fset, ifStmt.Cond),
		Kind:     "returns nil when err != nil",
	}
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
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		if r, ok := n.(*ast.ReturnStmt); ok {
			ret = r
			return false
		}
		return true
	})
	return ret
}

// returnsNilErrors requires nil in every error result. A second error result
// may propagate the checked error even when an earlier error result is nil.
func returnsNilErrors(ret *ast.ReturnStmt, positions []int) bool {
	if len(positions) == 0 {
		return false
	}
	for _, pos := range positions {
		if pos >= len(ret.Results) || !isNilIdent(ret.Results[pos]) {
			return false
		}
	}
	return true
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

// findTypedNilHits finds interface nil checks after a concrete nil-able
// value with nil evidence is boxed. Unknown constructor results are omitted.
func findTypedNilHits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.TypedNilHit {
	var out []patterns.TypedNilHit
	if fn.Body == nil || info == nil {
		return out
	}
	typedVars := collectTypedVars(fn.Body, info)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if hit := checkTypedNilCompare(n, fset, info, typedVars); hit != nil {
			out = append(out, *hit)
		}
		return true
	})
	return out
}

type typedNilEvidence struct {
	boxed   map[types.Object]token.Pos
	nilVars map[types.Object]bool
}

// collectTypedVars tracks interface objects, so shadowed names stay separate.
func collectTypedVars(body ast.Node, info *types.Info) map[types.Object]token.Pos {
	evidence := &typedNilEvidence{
		boxed:   map[types.Object]token.Pos{},
		nilVars: stableNilVars(body, info),
	}
	ast.Inspect(body, func(n ast.Node) bool {
		collectValueSpecVars(n, info, evidence)
		collectAssignVars(n, info, evidence)
		return true
	})
	for object, count := range interfaceWrites(body, info) {
		if count > 1 {
			delete(evidence.boxed, object)
		}
	}
	return evidence.boxed
}

func collectValueSpecVars(n ast.Node, info *types.Info, evidence *typedNilEvidence) {
	vs, ok := n.(*ast.ValueSpec)
	if !ok {
		return
	}
	for i, name := range vs.Names {
		if i < len(vs.Values) {
			trackTypedNilAssignment(name, vs.Values[i], info, evidence)
		}
	}
}

func collectAssignVars(n ast.Node, info *types.Info, evidence *typedNilEvidence) {
	assign, ok := n.(*ast.AssignStmt)
	if !ok {
		return
	}
	for i, lhs := range assign.Lhs {
		if i < len(assign.Rhs) {
			trackTypedNilAssignment(lhs, assign.Rhs[i], info, evidence)
		}
	}
}

func trackTypedNilAssignment(lhs, rhs ast.Expr, info *types.Info, evidence *typedNilEvidence) {
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return
	}
	object := info.ObjectOf(id)
	if object == nil {
		return
	}
	if _, ok := object.Type().Underlying().(*types.Interface); !ok {
		return
	}
	if isTypedNilable(info, rhs) && hasNilEvidence(rhs, info, evidence.nilVars) {
		evidence.boxed[object] = rhs.Pos()
	}
}

func checkTypedNilCompare(n ast.Node, fset *token.FileSet, info *types.Info, typedVars map[types.Object]token.Pos) *patterns.TypedNilHit {
	bin, ok := n.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
		return nil
	}
	name := typedNilVar(bin.X, bin.Y, info, typedVars)
	if name == "" {
		name = typedNilVar(bin.Y, bin.X, info, typedVars)
	}
	if name == "" {
		return nil
	}
	return &patterns.TypedNilHit{
		Line:     fset.Position(bin.Pos()).Line,
		ExprText: name + " may hold typed nil",
	}
}

func typedNilVar(v, other ast.Expr, info *types.Info, typedVars map[types.Object]token.Pos) string {
	id, ok := v.(*ast.Ident)
	if ok && typedVars[info.ObjectOf(id)] > 0 && typedVars[info.ObjectOf(id)] < id.Pos() && isNilIdent(other) {
		return id.Name
	}
	return ""
}

// isTypedNilable requires a concrete nil-able source. An interface-returning
// call alone is not evidence of a boxed typed nil. Named types use Underlying.
func isTypedNilable(info *types.Info, e ast.Expr) bool {
	t := info.TypeOf(e)
	if t == nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return true
	case *types.Interface:
		return isTypedNilConversion(info, e)
	}
	return false
}

func isTypedNilConversion(info *types.Info, e ast.Expr) bool {
	if paren, ok := e.(*ast.ParenExpr); ok {
		return isTypedNilable(info, paren.X)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	if !info.Types[call.Fun].IsType() {
		return false
	}
	return isTypedNilable(info, call.Args[0])
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
