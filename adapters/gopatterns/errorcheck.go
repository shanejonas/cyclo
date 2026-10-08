package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// findErrorCheckSites finds call sites and tracks whether the error was checked.
// Used for deviant_behavior mining (Engler et al., SOSP 2001).
func findErrorCheckSites(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.ErrorCheckSite {
	if fn.Body == nil {
		return nil
	}
	var sites []patterns.ErrorCheckSite
	// Use a custom walker that tracks parents.
	w := &errorCheckWalker{
		fset:  fset,
		info:  info,
		sites: &sites,
	}
	ast.Walk(w, fn.Body)
	return sites
}

type errorCheckWalker struct {
	fset  *token.FileSet
	info  *types.Info
	sites *[]patterns.ErrorCheckSite
	stack []ast.Node // Parent stack
}

func (w *errorCheckWalker) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		// Pop the stack on exit.
		w.stack = w.stack[:len(w.stack)-1]
		return nil
	}
	// Push current node.
	w.stack = append(w.stack, n)
	// Check if this is a call expression.
	if call, ok := n.(*ast.CallExpr); ok {
		w.visitCall(call)
	}
	return w
}

func (w *errorCheckWalker) visitCall(call *ast.CallExpr) {
	callee := calleeName(call, w.info)
	if callee == "" {
		return
	}
	if !returnsError(call, w.info) {
		return
	}
	checked := w.isChecked(call)
	line := w.fset.Position(call.Pos()).Line
	*w.sites = append(*w.sites, patterns.ErrorCheckSite{
		Line:    line,
		Callee:  callee,
		Checked: checked,
		// FuncID is set by the caller (needs the full ID).
	})
}

// isChecked determines if the error from a call was checked.
// Returns false if assigned to `_`, true otherwise (heuristic).
func (w *errorCheckWalker) isChecked(call *ast.CallExpr) bool {
	if len(w.stack) < 2 {
		return true
	}
	parent := w.stack[len(w.stack)-2]
	// Check if parent is an assignment to `_`.
	if assign, ok := parent.(*ast.AssignStmt); ok {
		// Find which result corresponds to the error.
		// For simplicity: if any LHS is `_`, and the call returns error,
		// assume the error was discarded.
		for _, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "_" {
				return false
			}
		}
	}
	// Check if parent is an ExprStmt (result ignored): `f()` without assignment.
	if _, ok := parent.(*ast.ExprStmt); ok {
		return false
	}
	return true
}

// calleeName extracts the called function name from a call expression.
func calleeName(call *ast.CallExpr, info *types.Info) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			return ident.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	}
	return ""
}

// returnsError checks if a call returns an error (via type info).
func returnsError(call *ast.CallExpr, info *types.Info) bool {
	if info == nil {
		return false
	}
	tv, ok := info.Types[call]
	if !ok {
		return false
	}
	if tup, ok := tv.Type.(*types.Tuple); ok {
		for i := 0; i < tup.Len(); i++ {
			if isErrorTypeCheck(tup.At(i).Type()) {
				return true
			}
		}
	}
	return isErrorTypeCheck(tv.Type)
}

// isErrorTypeCheck checks if a type is the error interface.
func isErrorTypeCheck(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		return obj.Name() == "error" && obj.Pkg() == nil
	}
	return false
}
