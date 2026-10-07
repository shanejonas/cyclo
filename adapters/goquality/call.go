package goquality

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"

	"github.com/shanejonas/cyclo/domain/quality"
)

func (x *extractor) call(call *ast.CallExpr) {
	fun := unwrappedCallee(call.Fun)
	object := x.calledObject(fun)
	if builtin, ok := object.(*types.Builtin); ok {
		x.builtin(call, builtin.Name())
		return
	}
	if x.pkg.TypesInfo.Types[call.Fun].IsType() {
		x.conversion(call)
		return
	}
	line := x.line(call.Pos())
	args := x.classifyArgs(call.Args)
	var fact quality.Call
	switch object := object.(type) {
	case *types.Func:
		fact = quality.Call{Line: line, Callee: resolvedName(object), Local: object.Pkg() == x.pkg.Types, Dynamic: interfaceCall(x.pkg.TypesInfo, fun), Args: args}
	case *types.Var:
		fact = quality.Call{Line: line, Callee: object.Name(), Dynamic: true, Args: args}
	default:
		fact = quality.Call{Line: line, Callee: "<dynamic>", Dynamic: true, Args: args}
	}
	x.fact.Calls = append(x.fact.Calls, fact)
}

// classifyArgs records each argument's provenance relative to the caller's
// parameters: a bare parameter (and its index), a constant, a field rooted
// at a local, or anything else.
func (x *extractor) classifyArgs(args []ast.Expr) []quality.ArgSource {
	if len(args) == 0 {
		return nil
	}
	out := make([]quality.ArgSource, len(args))
	for i, arg := range args {
		out[i] = x.classifyArg(arg)
	}
	return out
}

func (x *extractor) classifyArg(arg ast.Expr) quality.ArgSource {
	peeled := peelArg(arg)
	if source := x.classifyIdentArg(peeled); source != nil {
		return *source
	}
	if _, ok := peeled.(*ast.BasicLit); ok {
		return quality.ArgSource{Kind: quality.ArgConst}
	}
	if sel, ok := peeled.(*ast.SelectorExpr); ok && x.localFieldRoot(sel.X) {
		return quality.ArgSource{Kind: quality.ArgField}
	}
	return quality.ArgSource{Kind: quality.ArgOther}
}

// classifyIdentArg handles identifier arguments: parameters by index,
// named constants, or nil when neither.
func (x *extractor) classifyIdentArg(arg ast.Expr) *quality.ArgSource {
	id, ok := arg.(*ast.Ident)
	if !ok {
		return nil
	}
	if index, ok := x.paramIndex[x.pkg.TypesInfo.Uses[id]]; ok {
		return &quality.ArgSource{Kind: quality.ArgParam, Param: index}
	}
	if _, ok := x.pkg.TypesInfo.Uses[id].(*types.Const); ok {
		return &quality.ArgSource{Kind: quality.ArgConst}
	}
	return nil
}

// peelArg strips &, *, and parens: they change how a value is passed, not
// what it is.
func peelArg(expr ast.Expr) ast.Expr {
	for {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
		case *ast.UnaryExpr:
			if e.Op == token.AND || e.Op == token.MUL {
				expr = e.X
			} else {
				return expr
			}
		default:
			return expr
		}
	}
}

// localFieldRoot reports whether expr is rooted at a parameter, the receiver,
// or a function-local variable.
func (x *extractor) localFieldRoot(expr ast.Expr) bool {
	switch e := peelArg(expr).(type) {
	case *ast.Ident:
		return x.isFieldRoot(e)
	case *ast.SelectorExpr:
		return x.localFieldRoot(e.X)
	default:
		return false
	}
}

func (x *extractor) isFieldRoot(id *ast.Ident) bool {
	obj := x.pkg.TypesInfo.Uses[id]
	if _, ok := x.paramIndex[obj]; ok {
		return true
	}
	if obj != nil && obj == x.receiver {
		return true
	}
	v, ok := obj.(*types.Var)
	return ok && isLocalVar(v)
}

func isLocalVar(v *types.Var) bool {
	pkg := v.Pkg()
	return pkg != nil && v.Parent() != pkg.Scope()
}

func (x *extractor) conversion(call *ast.CallExpr) {
	basic, ok := underlying(x.pkg.TypesInfo.TypeOf(call)).(*types.Basic)
	if ok && basic.Kind() == types.UnsafePointer {
		x.effect(quality.Unsafe, "unsafe pointer conversion", call.Pos())
	}
}

func unwrappedCallee(expr ast.Expr) ast.Expr {
	switch expr := expr.(type) {
	case *ast.ParenExpr:
		return unwrappedCallee(expr.X)
	case *ast.IndexExpr:
		return unwrappedCallee(expr.X)
	case *ast.IndexListExpr:
		return unwrappedCallee(expr.X)
	default:
		return expr
	}
}

func (x *extractor) calledObject(expr ast.Expr) types.Object {
	switch expr := expr.(type) {
	case *ast.Ident:
		return x.pkg.TypesInfo.ObjectOf(expr)
	case *ast.SelectorExpr:
		return x.pkg.TypesInfo.ObjectOf(expr.Sel)
	default:
		return nil
	}
}

func interfaceCall(info *types.Info, expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := info.Selections[selector]
	if selection == nil {
		return false
	}
	_, ok = selection.Recv().Underlying().(*types.Interface)
	return ok
}

func (x *extractor) builtin(call *ast.CallExpr, name string) {
	switch name {
	case "append", "copy", "delete", "clear":
		if len(call.Args) > 0 {
			x.mutate(call.Args[0], call.Pos(), true)
		}
	case "print", "println":
		x.effect(quality.IO, "builtin."+name, call.Pos())
	case "panic", "recover":
		x.effect(quality.Panic, "builtin."+name, call.Pos())
	case "close":
		x.effect(quality.UnknownEffect, "channel close", call.Pos())
	default:
		x.unsafeBuiltin(call, name)
	}
}

func (x *extractor) unsafeBuiltin(call *ast.CallExpr, name string) {
	if slices.Contains([]string{"Add", "Slice", "SliceData", "String", "StringData"}, name) {
		x.effect(quality.Unsafe, "unsafe."+name, call.Pos())
	}
}
