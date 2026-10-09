package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/shanejonas/cyclo/domain/patterns"
	"golang.org/x/tools/go/packages"
)

// findDomainServices scans pkg for free functions operating on the
// fields of two or more distinct struct types. Such a function is
// stateless coordination between domain types — a Domain Service per
// Evans — not an anemic-model violation. It returns one hit per service
// function.
func findDomainServices(pkg *packages.Package, root string) []patterns.DomainServiceHit {
	structs := collectStructs(pkg, root)
	if len(structs) == 0 {
		return nil
	}
	names := structNameSet(structs)
	globals := collectGlobalVars(pkg)
	var out []patterns.DomainServiceHit
	for _, file := range pkg.Syntax {
		rel, ok := fileRelPath(pkg, file, root)
		if !ok {
			continue
		}
		out = append(out, fileDomainServices(pkg, file, rel, names, globals)...)
	}
	return out
}

// structNameSet returns the set of struct type names.
func structNameSet(structs []structDef) map[string]bool {
	out := make(map[string]bool, len(structs))
	for _, s := range structs {
		out[s.name] = true
	}
	return out
}

// fileDomainServices returns domain-service hits for one file.
func fileDomainServices(pkg *packages.Package, file *ast.File, rel string, names, globals map[string]bool) []patterns.DomainServiceHit {
	var out []patterns.DomainServiceHit
	for _, decl := range file.Decls {
		fn, ok := freeFuncDecl(decl)
		if !ok {
			continue
		}
		types := accessedStructTypes(fn, pkg.TypesInfo, names)
		if len(types) < 2 {
			continue
		}
		if assignsGlobals(fn.Body, globals) {
			continue
		}
		pos := pkg.Fset.PositionFor(fn.Pos(), false)
		end := pkg.Fset.PositionFor(fn.End(), false)
		out = append(out, patterns.DomainServiceHit{
			FuncName: fn.Name.Name,
			Path:     rel,
			Line:     pos.Line,
			EndLine:  end.Line,
			Types:    types,
		})
	}
	return out
}

// accessedStructTypes returns the sorted distinct struct type names
// whose fields fn accesses via its parameters. Only names in the
// known struct set count.
func accessedStructTypes(fn *ast.FuncDecl, info *types.Info, names map[string]bool) []string {
	paramTypes := paramStructTypes(fn, info, names)
	if len(paramTypes) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		tname, ok := paramTypes[ident.Name]
		if !ok || seen[tname] {
			return true
		}
		seen[tname] = true
		out = append(out, tname)
		return true
	})
	return out
}

// paramStructTypes maps parameter names to their struct type names,
// for params whose type is a known struct (by value or pointer).
func paramStructTypes(fn *ast.FuncDecl, info *types.Info, names map[string]bool) map[string]string {
	out := map[string]string{}
	if fn.Type.Params == nil {
		return out
	}
	for _, field := range fn.Type.Params.List {
		tname := namedTypeName(info.TypeOf(field.Type))
		if tname == "" || !names[tname] {
			continue
		}
		for _, name := range field.Names {
			out[name.Name] = tname
		}
	}
	return out
}

// namedTypeName returns the type name for t, unwrapping one pointer
// level. Empty when t is not a named type.
func namedTypeName(t types.Type) string {
	if t == nil {
		return ""
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

// collectGlobalVars returns the names of package-level variables.
func collectGlobalVars(pkg *packages.Package) map[string]bool {
	out := map[string]bool{}
	for _, file := range pkg.Syntax {
		collectFileGlobals(file, out)
	}
	return out
}

// collectFileGlobals adds package-level var names from one file.
func collectFileGlobals(file *ast.File, out map[string]bool) {
	for _, decl := range file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			addVarNames(gd, out)
		}
	}
}

// addVarNames adds the declared names from a var block.
func addVarNames(gd *ast.GenDecl, out map[string]bool) {
	for _, spec := range gd.Specs {
		if vs, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range vs.Names {
				out[name.Name] = true
			}
		}
	}
}

// assignsGlobals reports whether body assigns to any package-level
// variable. A domain service is stateless: it coordinates its inputs
// without hidden global mutation.
func assignsGlobals(body *ast.BlockStmt, globals map[string]bool) bool {
	if len(globals) == 0 {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if assign, ok := n.(*ast.AssignStmt); ok && assignHitsGlobal(assign, globals) {
			found = true
			return false
		}
		return true
	})
	return found
}

// assignHitsGlobal reports whether an assignment writes to a global.
func assignHitsGlobal(assign *ast.AssignStmt, globals map[string]bool) bool {
	for _, lhs := range assign.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && globals[ident.Name] {
			return true
		}
	}
	return false
}
