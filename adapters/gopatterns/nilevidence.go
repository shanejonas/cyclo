package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"
)

// stableNilVars records local nil values whose bindings are not reassigned or
// exposed through their address. Unknown calls and parameters are not evidence.
func stableNilVars(body ast.Node, info *types.Info) map[types.Object]bool {
	nilVars := map[types.Object]bool{}
	writes := bindingWrites(body, info)
	ast.Inspect(body, func(n ast.Node) bool {
		collectNilDeclarations(n, info, nilVars, writes)
		return true
	})
	return nilVars
}

func collectNilDeclarations(n ast.Node, info *types.Info, nilVars map[types.Object]bool, writes map[types.Object]int) {
	vs, ok := n.(*ast.ValueSpec)
	if !ok {
		return
	}
	for i, name := range vs.Names {
		object := info.ObjectOf(name)
		if object == nil || writes[object] > 0 {
			continue
		}
		nilVars[object] = declarationNilEvidence(vs, i, info, nilVars)
	}
}

// declarationNilEvidence handles zero values and explicit nil initializers.
func declarationNilEvidence(vs *ast.ValueSpec, index int, info *types.Info, nilVars map[types.Object]bool) bool {
	if len(vs.Values) == 0 {
		return concreteNilable(info.ObjectOf(vs.Names[index]).Type())
	}
	if index >= len(vs.Values) {
		return false
	}
	return hasNilEvidence(vs.Values[index], info, nilVars)
}

func concreteNilable(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature:
		return true
	}
	return false
}

func hasNilEvidence(e ast.Expr, info *types.Info, nilVars map[types.Object]bool) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return isNilIdent(e) || nilVars[info.ObjectOf(e)]
	case *ast.ParenExpr:
		return hasNilEvidence(e.X, info, nilVars)
	case *ast.CallExpr:
		return nilConversionEvidence(e, info, nilVars)
	}
	return false
}

func nilConversionEvidence(call *ast.CallExpr, info *types.Info, nilVars map[types.Object]bool) bool {
	if len(call.Args) != 1 || !info.Types[call.Fun].IsType() {
		return false
	}
	return hasNilEvidence(call.Args[0], info, nilVars)
}

// bindingWrites also invalidates address-taken bindings because callees can
// replace them. Definitions in short declarations count as initial writes.
func bindingWrites(body ast.Node, info *types.Info) map[types.Object]int {
	writes := map[types.Object]int{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				countBindingWrite(lhs, info, writes)
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				countBindingWrite(n.X, info, writes)
			}
		}
		return true
	})
	return writes
}

func countBindingWrite(e ast.Expr, info *types.Info, writes map[types.Object]int) {
	id, ok := e.(*ast.Ident)
	if !ok {
		return
	}
	if object := info.ObjectOf(id); object != nil {
		writes[object]++
	}
}

// interfaceWrites includes initializers so a later reassignment cannot leave
// stale boxing evidence. A declaration without an initializer is not a write.
func interfaceWrites(body ast.Node, info *types.Info) map[types.Object]int {
	writes := bindingWrites(body, info)
	ast.Inspect(body, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Values) == 0 {
			return true
		}
		for _, name := range vs.Names {
			countBindingWrite(name, info, writes)
		}
		return true
	})
	return writes
}
