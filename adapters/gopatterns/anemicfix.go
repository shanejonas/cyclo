package gopatterns

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"unicode"
)

// Anemic-model auto-fix: move behavior into methods. When a methodless
// struct has free functions operating on its fields, those functions want
// to be methods. This fixer performs the conversion mechanically: no LLM,
// no tokens.
//
// For each function `func CalculateTotal(o *Order, pct int) int`, it
// produces `func (o *Order) CalculateTotal(pct int) int` — the struct param
// moves to receiver position keeping its name and type, so the body is
// untouched. Call sites `CalculateTotal(order, 10)` become
// `order.CalculateTotal(10)`.

// AnemicFix describes one applied anemic-model rewrite: a free function
// converted into a method on the struct it operates on.
type AnemicFix struct {
	// Line is the function definition line.
	Line int
	// FuncName is the converted function name.
	FuncName string
	// TypeName is the struct type name, e.g. "Order".
	TypeName string
	// Kind is always "anemic_model".
	Kind string
}

// FixLine implements Fix.
func (a AnemicFix) FixLine() int { return a.Line }

// FixKind implements Fix.
func (a AnemicFix) FixKind() string { return a.Kind }

// anemicMinFuncs is the minimum functions operating on a methodless
// struct for conversion, mirroring the detector's threshold.
const anemicMinFuncs = 3

// AnemicTarget is a methodless exported struct declared in one file and
// the free functions in that file to convert into its methods.
type AnemicTarget struct {
	// TypeName is the struct type name.
	TypeName string
	// Funcs are the free functions to convert, in source order.
	Funcs []*ast.FuncDecl
}

// FixAnemicModels converts free functions operating on methodless structs
// into methods. It returns the rewritten source (gofmt-clean) and the
// fixes applied. The transform is behavior-preserving when all safety
// checks pass; structs or functions that fail any check are skipped.
func FixAnemicModels(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []AnemicFix, error) {
	targets := FindAnemicTargets(f)
	if len(targets) == 0 {
		return src, nil, nil
	}
	var fixes []AnemicFix
	for _, t := range targets {
		for _, fn := range t.Funcs {
			fixes = append(fixes, applyAnemicFunc(fset, f, fn, t.TypeName))
		}
	}
	if len(fixes) == 0 {
		return src, nil, nil
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, nil, fmt.Errorf("format after anemic_model fix: %w", err)
	}
	return buf.Bytes(), fixes, nil
}

// FindAnemicTargets detects convertible structs in f: exported structs
// declared in this file with no methods here, having at least
// anemicMinFuncs safe free functions operating on their fields.
func FindAnemicTargets(f *ast.File) []AnemicTarget {
	structs := fileExportedStructs(f)
	if len(structs) == 0 {
		return nil
	}
	methoded := fileMethodReceivers(f)
	return targetsWithMinFuncs(groupAnemicFuncs(f, structs, methoded))
}

// groupAnemicFuncs collects safe convertible functions by struct type.
func groupAnemicFuncs(f *ast.File, structs, methoded map[string]bool) map[string][]*ast.FuncDecl {
	byType := make(map[string][]*ast.FuncDecl)
	for _, decl := range f.Decls {
		fn, typeName, ok := anemicCandidate(f, decl, structs, methoded)
		if !ok {
			continue
		}
		byType[typeName] = append(byType[typeName], fn)
	}
	return byType
}

// anemicCandidate returns the func and struct type if decl is a safe
// convertible function operating on a methodless exported struct.
func anemicCandidate(f *ast.File, decl ast.Decl, structs, methoded map[string]bool) (*ast.FuncDecl, string, bool) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv != nil || fn.Body == nil {
		return nil, "", false
	}
	recvName, typeName := anemicFirstParam(fn)
	if !anemicTargetType(recvName, typeName, structs, methoded) {
		return nil, "", false
	}
	if !anemicChecksPass(f, fn, recvName, structs) {
		return nil, "", false
	}
	return fn, typeName, true
}

// anemicChecksPass runs the safety checks for an anemic-model fix
// candidate: field access, no value use, safe call sites, and not a
// domain service (which must not become a method).
func anemicChecksPass(f *ast.File, fn *ast.FuncDecl, recvName string, structs map[string]bool) bool {
	if !anemicFuncSafe(f, fn, recvName) {
		return false
	}
	return !isServiceLike(fn, structs)
}

// isServiceLike reports whether fn accesses the fields of two or more
// distinct struct types via its parameters. Such a function is a
// domain service (Evans): stateless coordination between types, not a
// misplaced method. The anemic fixer must not convert it.
func isServiceLike(fn *ast.FuncDecl, structs map[string]bool) bool {
	paramTypes := structParamTypes(fn, structs)
	if len(paramTypes) < 2 {
		return false
	}
	return countAccessedTypes(fn.Body, paramTypes) >= 2
}

// structParamTypes maps parameter names to struct type names for params
// whose type is a known struct (identifier or pointer to identifier).
func structParamTypes(fn *ast.FuncDecl, structs map[string]bool) map[string]string {
	out := map[string]string{}
	if fn.Type.Params == nil {
		return out
	}
	for _, field := range fn.Type.Params.List {
		tname := derefTypeName(field.Type)
		if tname == "" || !structs[tname] {
			continue
		}
		for _, name := range field.Names {
			out[name.Name] = tname
		}
	}
	return out
}

// countAccessedTypes counts distinct struct types whose fields are
// accessed via the named params, stopping at 2.
func countAccessedTypes(body *ast.BlockStmt, paramTypes map[string]string) int {
	seen := map[string]bool{}
	count := 0
	ast.Inspect(body, func(n ast.Node) bool {
		if count >= 2 {
			return false
		}
		if tname, ok := selectorParamType(n, paramTypes); ok && !seen[tname] {
			seen[tname] = true
			count++
		}
		return true
	})
	return count
}

// selectorParamType returns the struct type name when n is a selector
// expression on a known param (e.g. o.Items where o is a param).
func selectorParamType(n ast.Node, paramTypes map[string]string) (string, bool) {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	tname, ok := paramTypes[ident.Name]
	return tname, ok
}

// anemicTargetType reports whether typeName is a convertible struct:
// exported, declared here, with no methods here.
func anemicTargetType(recvName, typeName string, structs, methoded map[string]bool) bool {
	return recvName != "" && structs[typeName] && !methoded[typeName]
}

// targetsWithMinFuncs builds targets for types with enough functions.
func targetsWithMinFuncs(byType map[string][]*ast.FuncDecl) []AnemicTarget {
	var out []AnemicTarget
	for typeName, funcs := range byType {
		if len(funcs) >= anemicMinFuncs {
			out = append(out, AnemicTarget{TypeName: typeName, Funcs: funcs})
		}
	}
	return out
}

// fileExportedStructs returns the names of exported struct types declared
// in f.
func fileExportedStructs(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		addExportedStructs(out, gd)
	}
	return out
}

// addExportedStructs records exported struct names from a type declaration.
func addExportedStructs(out map[string]bool, gd *ast.GenDecl) {
	for _, spec := range gd.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if _, ok := ts.Type.(*ast.StructType); !ok {
			continue
		}
		if name := ts.Name.Name; name != "" && unicode.IsUpper(rune(name[0])) {
			out[name] = true
		}
	}
}

// fileMethodReceivers returns the type names that have methods declared
// in f.
func fileMethodReceivers(f *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		if name := syntacticReceiverName(fn.Recv); name != "" {
			out[name] = true
		}
	}
	return out
}

// syntacticReceiverName extracts the receiver type name without type
// info, handling both (o T) and (o *T) forms.
func syntacticReceiverName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	return derefTypeName(recv.List[0].Type)
}

// derefTypeName returns the base type name for T or *T, or "" for any
// other shape (selectors, maps, etc.).
func derefTypeName(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	if ident, ok := e.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// anemicFirstParam checks that fn's first parameter is a named param of a
// bare struct type (T or *T). It returns the param name and type name, or
// "" when the first param is not a named struct-typed param.
func anemicFirstParam(fn *ast.FuncDecl) (string, string) {
	if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return "", ""
	}
	field := fn.Type.Params.List[0]
	if len(field.Names) == 0 {
		return "", ""
	}
	typeName := derefTypeName(field.Type)
	if typeName == "" {
		return "", ""
	}
	return field.Names[0].Name, typeName
}

// anemicFuncSafe runs all safety checks for converting fn to a method on
// the struct its first param names.
func anemicFuncSafe(f *ast.File, fn *ast.FuncDecl, recvName string) bool {
	if !isUnexportedFunc(fn) {
		return false
	}
	if !accessesFields(fn.Body, []string{recvName}) {
		return false
	}
	if funcUsedAsValue(f, fn) {
		return false
	}
	return anemicCallSitesSafe(f, fn.Name.Name)
}

// funcUsedAsValue reports whether fn's name appears as a reference other
// than a direct call or the declaration itself — e.g. `fn := CalculateTotal`
// or passing it as an argument. Converting to a method would break those.
func funcUsedAsValue(f *ast.File, fn *ast.FuncDecl) bool {
	return hasUnsafeRef(f, fn, safeIdents(f))
}

// safeIdents collects idents that are call targets or selector names —
// positions where a name reference is not a value use of a free function.
func safeIdents(f *ast.File) map[*ast.Ident]bool {
	safe := map[*ast.Ident]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if ident, ok := x.Fun.(*ast.Ident); ok {
				safe[ident] = true
			}
		case *ast.SelectorExpr:
			safe[x.Sel] = true
		}
		return true
	})
	return safe
}

// hasUnsafeRef reports whether fn's name is referenced outside the safe
// positions (its declaration, direct calls, selector names).
func hasUnsafeRef(f *ast.File, fn *ast.FuncDecl, safe map[*ast.Ident]bool) bool {
	name := fn.Name.Name
	used := false
	ast.Inspect(f, func(n ast.Node) bool {
		if used {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Name != name || ident == fn.Name || safe[ident] {
			return true
		}
		used = true
		return false
	})
	return used
}

// anemicCallSitesSafe reports whether every direct call to funcName in f
// can be rewritten as a method call.
func anemicCallSitesSafe(f *ast.File, funcName string) bool {
	ok := true
	ast.Inspect(f, func(n ast.Node) bool {
		if !ok {
			return false
		}
		if !callSiteRewritable(n, funcName) {
			ok = false
			return false
		}
		return true
	})
	return ok
}

// callSiteRewritable reports whether n, if a direct call to funcName, has
// a form that becomes a valid method call: no ellipsis, at least one arg
// to become the receiver.
func callSiteRewritable(n ast.Node, funcName string) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return true
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != funcName {
		return true
	}
	return !call.Ellipsis.IsValid() && len(call.Args) >= 1
}

// applyAnemicFunc converts one function to a method: call sites are
// rewritten first (they reference the original name), then the signature
// gains a receiver and loses its first param.
func applyAnemicFunc(fset *token.FileSet, f *ast.File, fn *ast.FuncDecl, typeName string) AnemicFix {
	rewriteAnemicCallSites(f, fn.Name.Name)
	convertToMethod(fn)
	line := fset.PositionFor(fn.Pos(), false).Line
	return AnemicFix{Line: line, FuncName: fn.Name.Name, TypeName: typeName, Kind: "anemic_model"}
}

// rewriteAnemicCallSites turns f(a, rest...) into a.f(rest...) for every
// direct call to funcName in f.
func rewriteAnemicCallSites(f *ast.File, funcName string) {
	var calls []*ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != funcName {
			return true
		}
		calls = append(calls, call)
		return true
	})
	for _, call := range calls {
		recv := call.Args[0]
		call.Fun = &ast.SelectorExpr{X: recv, Sel: &ast.Ident{Name: funcName}}
		call.Args = call.Args[1:]
	}
}

// convertToMethod moves fn's first param into receiver position, keeping
// its name and exact type. The body is untouched: receiver references
// keep working because the name is unchanged.
func convertToMethod(fn *ast.FuncDecl) {
	field := fn.Type.Params.List[0]
	recvName := field.Names[0].Name
	recvType := field.Type
	if len(field.Names) == 1 {
		fn.Type.Params.List = fn.Type.Params.List[1:]
	} else {
		field.Names = field.Names[1:]
	}
	fn.Recv = &ast.FieldList{
		List: []*ast.Field{{
			Names: []*ast.Ident{{Name: recvName}},
			Type:  recvType,
		}},
	}
}
