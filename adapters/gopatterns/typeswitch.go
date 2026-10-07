package gopatterns

import (
	"bytes"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

// TypeSwitchHit is one type switch whose arms all call the same method on
// the case-bound value. It is the fixable subset of type switches: when
// every arm does `v.Method(...)` with the same method name, the switch can
// be replaced by a direct interface method call.
//
// Example:
//
//	switch v := x.(type) {
//	case Dog:
//		v.Speak()
//	case Cat:
//		v.Speak()
//	}
//
// becomes:
//
//	if v, ok := x.(interface{ Speak() }); ok {
//		v.Speak()
//	}
//
// The comma-ok form preserves the no-match behavior of a switch without a
// default clause. The interface signature is taken from the method
// declarations found in the same file; if they are missing or differ, the
// hit is skipped (the fixer re-validates).
type TypeSwitchHit struct {
	// Line is the line of the switch statement.
	Line int
	// Bound is the variable bound by `switch v := x.(type)`.
	Bound string
	// Expr is the source text of the switched expression (x).
	Expr string
	// Method is the method name called in every arm.
	Method string
	// Types are the case type names (Dog, Cat, ...).
	Types []string
	// Args is the source text of the call arguments, identical in every arm.
	Args string
}

// findTypeSwitches scans fn for type switches in the fixable subset:
// `switch v := x.(type)` with 2+ value cases, no default statements, each
// arm a single `v.Method(...)` call with the same method name and args.
func findTypeSwitches(fn *ast.FuncDecl, fset *token.FileSet) []TypeSwitchHit {
	var hits []TypeSwitchHit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sw, ok := n.(*ast.TypeSwitchStmt)
		if !ok {
			return true
		}
		if hit, ok := asFixableTypeSwitch(sw, fset); ok {
			hits = append(hits, hit)
		}
		return true
	})
	return hits
}

// asFixableTypeSwitch reports whether sw is a type switch in the fixable
// subset and returns the hit.
func asFixableTypeSwitch(sw *ast.TypeSwitchStmt, fset *token.FileSet) (TypeSwitchHit, bool) {
	bound, expr, ok := typeSwitchBinding(sw)
	if !ok {
		return TypeSwitchHit{}, false
	}
	arms, ok := typeSwitchArms(sw, bound)
	if !ok {
		return TypeSwitchHit{}, false
	}
	types := make([]string, len(arms))
	for i, a := range arms {
		types[i] = a.typ
	}
	return TypeSwitchHit{
		Line:   fset.PositionFor(sw.Pos(), false).Line,
		Bound:  bound,
		Expr:   expr,
		Method: arms[0].method,
		Types:  types,
		Args:   arms[0].args,
	}, true
}

// typeSwitchBinding extracts the bound variable and switched expression
// from `switch v := x.(type)`. In the AST this is a TypeSwitchStmt whose
// Assign is `v := x.(type)` with a TypeAssertExpr whose Type is nil.
func typeSwitchBinding(sw *ast.TypeSwitchStmt) (bound, expr string, ok bool) {
	assign, boundID, ok := bindingAssign(sw)
	if !ok {
		return "", "", false
	}
	assert, ok := bindingAssert(assign)
	if !ok {
		return "", "", false
	}
	var buf bytes.Buffer
	fset := token.NewFileSet()
	if err := printer.Fprint(&buf, fset, assert.X); err != nil {
		return "", "", false
	}
	return boundID.Name, buf.String(), true
}

// bindingAssign returns the `v := ...` assignment and bound identifier.
func bindingAssign(sw *ast.TypeSwitchStmt) (*ast.AssignStmt, *ast.Ident, bool) {
	assign, ok := sw.Assign.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return nil, nil, false
	}
	boundID, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	return assign, boundID, true
}

// bindingAssert returns the type assertion `x.(type)` from the assignment.
func bindingAssert(assign *ast.AssignStmt) (*ast.TypeAssertExpr, bool) {
	assert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
	if !ok || assert.Type != nil {
		return nil, false
	}
	return assert, true
}

// typeArm is one case arm: its type name, the method it calls, and the
// argument source text.
type typeArm struct {
	typ    string
	method string
	args   string
}

// typeSwitchArms extracts the arms of a type switch: 2+ value cases, no
// default with statements, each arm a single `bound.Method(...)` call with
// the same method name and identical args.
func typeSwitchArms(sw *ast.TypeSwitchStmt, bound string) ([]typeArm, bool) {
	var arms []typeArm
	for _, stmt := range sw.Body.List {
		arm, ok := caseArmOf(stmt, bound)
		if !ok {
			return nil, false
		}
		if arm != nil {
			arms = append(arms, *arm)
		}
	}
	if len(arms) < 2 {
		return nil, false
	}
	if !sameCall(arms) {
		return nil, false
	}
	return arms, true
}

// caseArmOf extracts a typeArm from one switch body statement. Returns
// (nil, true) for an empty default clause, (nil, false) when the statement
// is not a fixable arm.
func caseArmOf(stmt ast.Stmt, bound string) (*typeArm, bool) {
	cc, ok := stmt.(*ast.CaseClause)
	if !ok {
		return nil, false
	}
	if cc.List == nil {
		return defaultArm(cc)
	}
	return valueArm(cc, bound)
}

// defaultArm handles the default clause: only allowed when empty.
func defaultArm(cc *ast.CaseClause) (*typeArm, bool) {
	if len(cc.Body) > 0 {
		return nil, false
	}
	return nil, true
}

// valueArm extracts a typeArm from a value case clause.
func valueArm(cc *ast.CaseClause, bound string) (*typeArm, bool) {
	if len(cc.List) != 1 {
		return nil, false
	}
	typ, ok := typeNameOf(cc.List[0])
	if !ok {
		return nil, false
	}
	method, args, ok := singleBoundCall(cc.Body, bound)
	if !ok {
		return nil, false
	}
	return &typeArm{typ: typ, method: method, args: args}, true
}

// sameCall reports whether all arms call the same method with identical args.
func sameCall(arms []typeArm) bool {
	for _, a := range arms[1:] {
		if a.method != arms[0].method || a.args != arms[0].args {
			return false
		}
	}
	return true
}

// typeNameOf returns the type name of a case clause expression.
// Handles `Dog` and `*Dog` (pointer cases).
func typeNameOf(e ast.Expr) (string, bool) {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name, true
	case *ast.StarExpr:
		return typeNameOf(t.X)
	default:
		return "", false
	}
}

// singleBoundCall reports whether body is exactly one statement calling
// `bound.Method(...)`, returning the method name and the argument source
// text ("" for no args).
func singleBoundCall(body []ast.Stmt, bound string) (method, args string, ok bool) {
	call, ok := singleCallExpr(body)
	if !ok {
		return "", "", false
	}
	sel, _, ok := boundSelector(call, bound)
	if !ok {
		return "", "", false
	}
	return sel.Sel.Name, argsText(call), true
}

// singleCallExpr returns the call expression when body is exactly one
// expression statement containing a call.
func singleCallExpr(body []ast.Stmt) (*ast.CallExpr, bool) {
	if len(body) != 1 {
		return nil, false
	}
	exprStmt, ok := body[0].(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	return call, ok
}

// boundSelector returns the selector and receiver when the call is
// `bound.Method(...)`.
func boundSelector(call *ast.CallExpr, bound string) (*ast.SelectorExpr, *ast.Ident, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != bound {
		return nil, nil, false
	}
	return sel, recv, true
}

// argsText renders the call arguments as source text.
func argsText(call *ast.CallExpr) string {
	var buf bytes.Buffer
	fset := token.NewFileSet()
	for i, arg := range call.Args {
		if i > 0 {
			buf.WriteString(", ")
		}
		if err := printer.Fprint(&buf, fset, arg); err != nil {
			return ""
		}
	}
	return buf.String()
}

// methodSig finds the method declaration `func (r Type) Method(` in the
// same file and returns its signature text `Method(params) results`.
// Used by the fixer to build the interface literal.
func methodSig(f *ast.File, fset *token.FileSet, typ, method string) (string, bool) {
	for _, decl := range f.Decls {
		if sig, ok := methodSigOf(decl, fset, typ, method); ok {
			return sig, true
		}
	}
	return "", false
}

// methodSigOf returns the signature when decl is the wanted method.
func methodSigOf(decl ast.Decl, fset *token.FileSet, typ, method string) (string, bool) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Name.Name != method || fn.Recv == nil {
		return "", false
	}
	if !recvMatches(fn.Recv, typ) {
		return "", false
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, fn.Type); err != nil {
		return "", false
	}
	sig := strings.TrimPrefix(buf.String(), "func") // "func(params) results"
	return method + sig, true
}

// recvMatches reports whether the receiver list declares a receiver of
// the named type (value or pointer).
func recvMatches(recv *ast.FieldList, typ string) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	name, ok := typeNameOf(recv.List[0].Type)
	return ok && name == typ
}

// findTypeSwitchAtLine locates the type switch statement at the given line.
func findTypeSwitchAtLine(fset *token.FileSet, f *ast.File, line int) *ast.TypeSwitchStmt {
	var target *ast.TypeSwitchStmt
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.TypeSwitchStmt)
		if !ok {
			return true
		}
		if fset.Position(sw.Pos()).Line == line {
			target = sw
			return false
		}
		return true
	})
	return target
}
