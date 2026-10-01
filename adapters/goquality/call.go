package goquality

import (
	"go/ast"
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
	var fact quality.Call
	switch object := object.(type) {
	case *types.Func:
		fact = quality.Call{Line: line, Callee: resolvedName(object), Local: object.Pkg() == x.pkg.Types, Dynamic: interfaceCall(x.pkg.TypesInfo, fun)}
	case *types.Var:
		fact = quality.Call{Line: line, Callee: object.Name(), Dynamic: true}
	default:
		fact = quality.Call{Line: line, Callee: "<dynamic>", Dynamic: true}
	}
	x.fact.Calls = append(x.fact.Calls, fact)
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
