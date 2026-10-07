package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"unicode"

	"github.com/shanejonas/cyclo/domain/patterns"
	"golang.org/x/tools/go/packages"
)

// structDef is an exported struct type declaration.
type structDef struct {
	name    string
	path    string
	line    int
	endLine int
}

// findAnemicModels scans pkg for exported structs with no methods that
// have at least 3 functions operating on their fields. It returns one
// hit per anemic struct.
func findAnemicModels(pkg *packages.Package, root string) []patterns.AnemicModelHit {
	structs := collectStructs(pkg, root)
	if len(structs) == 0 {
		return nil
	}
	methods := collectMethodReceivers(pkg)
	var out []patterns.AnemicModelHit
	for _, sd := range structs {
		if methods[sd.name] > 0 {
			continue
		}
		funcs := fieldOperatingFuncs(pkg, sd.name)
		if len(funcs) >= 3 {
			out = append(out, patterns.AnemicModelHit{
				TypeName: sd.name,
				Path:     sd.path,
				Line:     sd.line,
				EndLine:  sd.endLine,
				Funcs:    funcs,
			})
		}
	}
	return out
}

// collectStructs returns the exported struct types declared in pkg.
func collectStructs(pkg *packages.Package, root string) []structDef {
	var out []structDef
	for _, file := range pkg.Syntax {
		rel, ok := fileRelPath(pkg, file, root)
		if !ok {
			continue
		}
		out = append(out, fileStructs(pkg, file, rel)...)
	}
	return out
}

// fileRelPath returns the slash-separated path of file relative to root.
func fileRelPath(pkg *packages.Package, file *ast.File, root string) (string, bool) {
	filename := pkg.Fset.PositionFor(file.Pos(), false).Filename
	rel, err := filepath.Rel(root, filename)
	if err != nil {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// fileStructs returns the exported struct types declared in one file.
func fileStructs(pkg *packages.Package, file *ast.File, rel string) []structDef {
	var out []structDef
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			if sd, ok := structSpec(pkg, spec, rel); ok {
				out = append(out, sd)
			}
		}
	}
	return out
}

// structSpec returns the structDef for an exported struct type spec.
func structSpec(pkg *packages.Package, spec ast.Spec, rel string) (structDef, bool) {
	ts, ok := spec.(*ast.TypeSpec)
	if !ok {
		return structDef{}, false
	}
	if _, ok := ts.Type.(*ast.StructType); !ok {
		return structDef{}, false
	}
	name := ts.Name.Name
	if name == "" || !unicode.IsUpper(rune(name[0])) {
		return structDef{}, false
	}
	pos := pkg.Fset.PositionFor(ts.Pos(), false)
	end := pkg.Fset.PositionFor(ts.End(), false)
	return structDef{name: name, path: rel, line: pos.Line, endLine: end.Line}, true
}

// collectMethodReceivers counts methods by their receiver's type name.
func collectMethodReceivers(pkg *packages.Package) map[string]int {
	out := map[string]int{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if name, ok := methodReceiverName(pkg, decl); ok {
				out[name]++
			}
		}
	}
	return out
}

// methodReceiverName returns the receiver type name if decl is a method.
func methodReceiverName(pkg *packages.Package, decl ast.Decl) (string, bool) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv == nil || fn.Body == nil {
		return "", false
	}
	name := receiverTypeName(fn.Recv, pkg.TypesInfo)
	return name, name != ""
}

// receiverTypeName extracts the type name from a method receiver,
// handling both T and *T forms.
func receiverTypeName(recv *ast.FieldList, info *types.Info) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := info.TypeOf(recv.List[0].Type)
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

// fieldOperatingFuncs returns the names of functions in pkg that take
// the named struct as a parameter and access its fields via that param.
func fieldOperatingFuncs(pkg *packages.Package, typeName string) []string {
	seen := map[string]bool{}
	var out []string
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if name, ok := operatingFuncName(pkg, decl, typeName, seen); ok {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

// operatingFuncName returns the function name if decl is a free function
// taking typeName as a param and accessing its fields, and not seen.
func operatingFuncName(pkg *packages.Package, decl ast.Decl, typeName string, seen map[string]bool) (string, bool) {
	fn, ok := freeFuncDecl(decl)
	if !ok {
		return "", false
	}
	params := structParams(fn, pkg.TypesInfo, typeName)
	if len(params) == 0 || !accessesFields(fn.Body, params) {
		return "", false
	}
	return unseenName(fn.Name.Name, seen)
}

// freeFuncDecl returns the FuncDecl if decl is a free function with a body.
func freeFuncDecl(decl ast.Decl) (*ast.FuncDecl, bool) {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Body == nil || fn.Recv != nil {
		return nil, false
	}
	return fn, true
}

// unseenName returns the name if not already seen.
func unseenName(name string, seen map[string]bool) (string, bool) {
	if seen[name] {
		return "", false
	}
	return name, true
}

// structParams returns the parameter names whose type is the named
// struct (by value or pointer).
func structParams(fn *ast.FuncDecl, info *types.Info, typeName string) []string {
	if fn.Type.Params == nil {
		return nil
	}
	var out []string
	for _, field := range fn.Type.Params.List {
		if isNamedType(info.TypeOf(field.Type), typeName) {
			for _, name := range field.Names {
				out = append(out, name.Name)
			}
		}
	}
	return out
}

// isNamedType reports whether t is the named type typeName, unwrapping
// one pointer level.
func isNamedType(t types.Type, typeName string) bool {
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	return ok && named.Obj().Name() == typeName
}

// accessesFields reports whether body contains a selector expression
// on any of the named params (e.g. o.Items where o is a param).
func accessesFields(body *ast.BlockStmt, params []string) bool {
	paramSet := make(map[string]bool, len(params))
	for _, p := range params {
		paramSet[p] = true
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if paramSet[ident.Name] {
			found = true
			return false
		}
		return true
	})
	return found
}
