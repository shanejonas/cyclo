package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
	"golang.org/x/tools/go/packages"
)

// idFieldNames are the recognized identity field names (case-insensitive).
var idFieldNames = map[string]bool{
	"id": true, "uuid": true, "guid": true,
}

// isIDField reports whether name looks like an identity field.
func isIDField(name string) bool {
	return idFieldNames[strings.ToLower(name)]
}

// structIDField returns the ID-like field name of a struct type, or "".
func structIDField(st *types.Struct) string {
	for i := 0; i < st.NumFields(); i++ {
		if isIDField(st.Field(i).Name()) {
			return st.Field(i).Name()
		}
	}
	return ""
}

// findEntityIdentities scans fn for attribute-based equality: boolean
// expressions comparing 2+ fields of the same struct type when the struct
// has an ID field. Example: `if a.Name == b.Name && a.Email == b.Email`
// where User has an ID field — should be `a.ID == b.ID`.
func findEntityIdentities(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.EntityIdentityHit {
	var out []patterns.EntityIdentityHit
	seen := map[int]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cond := boolExpr(n)
		if cond == nil {
			return true
		}
		if hit, ok := checkAttrEquality(cond, fset, info); ok {
			addUniqueHit(&out, seen, hit, fset, cond)
		}
		return true
	})
	return out
}

// boolExpr extracts a boolean expression from if conditions or returns.
func boolExpr(n ast.Node) ast.Expr {
	switch node := n.(type) {
	case *ast.IfStmt:
		return node.Cond
	case *ast.ReturnStmt:
		if len(node.Results) == 1 {
			return node.Results[0]
		}
	}
	return nil
}

// addUniqueHit appends a hit if its line hasn't been seen.
func addUniqueHit(out *[]patterns.EntityIdentityHit, seen map[int]bool, hit patterns.EntityIdentityHit, fset *token.FileSet, cond ast.Expr) {
	pos := fset.PositionFor(cond.Pos(), false)
	if !seen[pos.Line] {
		seen[pos.Line] = true
		*out = append(*out, hit)
	}
}

// fieldComparison is one `a.Field == b.Field` comparison.
type fieldComparison struct {
	left     string // "a"
	right    string // "b"
	field    string // "Name"
	typeName string // "User"
	idField  string // "ID"
	line     int
}

// checkAttrEquality checks if cond is an &&-chain of field == comparisons
// on the same struct type that has an ID field.
func checkAttrEquality(cond ast.Expr, fset *token.FileSet, info *types.Info) (patterns.EntityIdentityHit, bool) {
	var comps []fieldComparison
	collectComparisons(cond, &comps, fset, info)
	if len(comps) < 2 {
		return patterns.EntityIdentityHit{}, false
	}
	first := comps[0]
	if !allSameType(comps, first) {
		return patterns.EntityIdentityHit{}, false
	}
	if !allSamePair(comps, first) {
		return patterns.EntityIdentityHit{}, false
	}
	if comparesIDField(comps, first.idField) {
		return patterns.EntityIdentityHit{}, false
	}
	return buildEntityHit(comps, first), true
}

// allSameType checks that all comparisons are on the same struct type.
func allSameType(comps []fieldComparison, first fieldComparison) bool {
	for _, c := range comps[1:] {
		if c.typeName != first.typeName || c.idField != first.idField {
			return false
		}
	}
	return true
}

// allSamePair checks that all comparisons use the same variable pair.
func allSamePair(comps []fieldComparison, first fieldComparison) bool {
	for _, c := range comps[1:] {
		if c.left != first.left || c.right != first.right {
			return false
		}
	}
	return true
}

// comparesIDField checks if any comparison is already on the ID field.
func comparesIDField(comps []fieldComparison, idField string) bool {
	for _, c := range comps {
		if c.field == idField {
			return true
		}
	}
	return false
}

// buildEntityHit constructs the hit from validated comparisons.
func buildEntityHit(comps []fieldComparison, first fieldComparison) patterns.EntityIdentityHit {
	var fields []string
	for _, c := range comps {
		fields = append(fields, c.field)
	}
	return patterns.EntityIdentityHit{
		Line:     first.line,
		TypeName: first.typeName,
		IDField:  first.idField,
		Fields:   fields,
		Left:     first.left,
		Right:    first.right,
	}
}

// collectComparisons gathers `x.Field == y.Field` comparisons from an
// &&-chain (or a single comparison).
func collectComparisons(expr ast.Expr, out *[]fieldComparison, fset *token.FileSet, info *types.Info) {
	if bin, ok := expr.(*ast.BinaryExpr); ok && bin.Op == token.LAND {
		collectComparisons(bin.X, out, fset, info)
		collectComparisons(bin.Y, out, fset, info)
		return
	}
	if comp, ok := fieldEqualComparison(expr, fset, info); ok {
		*out = append(*out, comp)
	}
}

// fieldEqualComparison checks if expr is `a.Field == b.Field` where a and b
// are the same struct type with an ID field.
func fieldEqualComparison(expr ast.Expr, fset *token.FileSet, info *types.Info) (fieldComparison, bool) {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return fieldComparison{}, false
	}
	leftSel, rightSel, leftIdent, rightIdent, ok := comparisonOperands(bin)
	if !ok {
		return fieldComparison{}, false
	}
	if leftSel.Sel.Name != rightSel.Sel.Name {
		return fieldComparison{}, false
	}
	typeName, idField, ok := structTypeWithID(leftIdent, rightIdent, info)
	if !ok {
		return fieldComparison{}, false
	}
	pos := fset.PositionFor(bin.Pos(), false)
	return fieldComparison{
		left:     leftIdent.Name,
		right:    rightIdent.Name,
		field:    leftSel.Sel.Name,
		typeName: typeName,
		idField:  idField,
		line:     pos.Line,
	}, true
}

// comparisonOperands extracts selectors and idents from a binary comparison.
func comparisonOperands(bin *ast.BinaryExpr) (*ast.SelectorExpr, *ast.SelectorExpr, *ast.Ident, *ast.Ident, bool) {
	leftSel, rightSel, ok := selectorPair(bin)
	if !ok {
		return nil, nil, nil, nil, false
	}
	leftIdent, rightIdent, ok := identPair(leftSel, rightSel)
	if !ok {
		return nil, nil, nil, nil, false
	}
	return leftSel, rightSel, leftIdent, rightIdent, true
}

// selectorPair extracts the two selector expressions from a binary expr.
func selectorPair(bin *ast.BinaryExpr) (*ast.SelectorExpr, *ast.SelectorExpr, bool) {
	leftSel, ok := bin.X.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}
	rightSel, ok := bin.Y.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}
	return leftSel, rightSel, true
}

// identPair extracts the identifiers from two selector expressions.
func identPair(leftSel, rightSel *ast.SelectorExpr) (*ast.Ident, *ast.Ident, bool) {
	leftIdent, ok := leftSel.X.(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	rightIdent, ok := rightSel.X.(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	return leftIdent, rightIdent, true
}

// structTypeWithID checks that both idents are the same named struct type
// with an ID field. Returns the type name and ID field name.
func structTypeWithID(leftIdent, rightIdent *ast.Ident, info *types.Info) (string, string, bool) {
	leftType, rightType, ok := identTypes(leftIdent, rightIdent, info)
	if !ok {
		return "", "", false
	}
	leftStruct, ok := derefStruct(leftType)
	if !ok || !isStructType(rightType) {
		return "", "", false
	}
	typeName, ok := sameNamedType(leftType, rightType)
	if !ok {
		return "", "", false
	}
	idField := structIDField(leftStruct)
	if idField == "" {
		return "", "", false
	}
	return typeName, idField, true
}

// identTypes returns the types of two identifiers.
func identTypes(left, right *ast.Ident, info *types.Info) (types.Type, types.Type, bool) {
	lt := info.TypeOf(left)
	rt := info.TypeOf(right)
	if lt == nil || rt == nil {
		return nil, nil, false
	}
	return lt, rt, true
}

// isStructType reports whether t is (a pointer to) a struct.
func isStructType(t types.Type) bool {
	_, ok := derefStruct(t)
	return ok
}

// sameNamedType checks that two types are the same named type.
func sameNamedType(leftType, rightType types.Type) (string, bool) {
	leftNamed, ok := leftType.(*types.Named)
	if !ok {
		return "", false
	}
	rightNamed, ok := rightType.(*types.Named)
	if !ok {
		return "", false
	}
	if leftNamed.Obj().Name() != rightNamed.Obj().Name() {
		return "", false
	}
	return leftNamed.Obj().Name(), true
}

// derefStruct returns the struct type, dereferencing pointers.
func derefStruct(t types.Type) (*types.Struct, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, false
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	return st, true
}

// findMutableIdentities scans fn for assignments to ID-like fields outside
// constructors (functions named New* or Create*).
// FindMutableIdentities is exported for the quality gate: it scans fn
// for assignments to ID-like fields outside constructors.
func FindMutableIdentities(fn *ast.FuncDecl, fset *token.FileSet) []patterns.MutableIdentityHit {
	name := fn.Name.Name
	if isConstructorName(name) {
		return nil
	}
	var out []patterns.MutableIdentityHit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		out = append(out, idAssignments(assign, fset, name)...)
		return true
	})
	return out
}

// isConstructorName reports whether fn is a constructor/factory.
func isConstructorName(name string) bool {
	return strings.HasPrefix(name, "New") || strings.HasPrefix(name, "Create")
}

// idAssignments returns hits for ID field assignments in an assign stmt.
func idAssignments(assign *ast.AssignStmt, fset *token.FileSet, funcName string) []patterns.MutableIdentityHit {
	var out []patterns.MutableIdentityHit
	for _, lhs := range assign.Lhs {
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok || !isIDField(sel.Sel.Name) {
			continue
		}
		pos := fset.PositionFor(assign.Pos(), false)
		out = append(out, patterns.MutableIdentityHit{
			Line:     pos.Line,
			Field:    sel.Sel.Name,
			FuncName: funcName,
		})
	}
	return out
}

// identityScan carries the per-package facts missingIdentityFor needs.
type identityScan struct {
	uses    map[string]int
	mutated map[string]bool
	pkg     *packages.Package
}

// findMissingIdentities scans pkg for struct types used as entities (in
// maps, slices, or as function parameters 3+ times) that lack an ID field.
// Value objects (never mutated) are skipped: in DDD they are immutable and
// identified by their attributes, not by identity.
func findMissingIdentities(pkg *packages.Package, root string) []patterns.MissingIdentityHit {
	structs := collectStructs(pkg, root)
	if len(structs) == 0 {
		return nil
	}
	scan := identityScan{
		uses:    countEntityUses(pkg),
		mutated: collectMutatedStructs(pkg),
		pkg:     pkg,
	}
	var out []patterns.MissingIdentityHit
	for _, sd := range structs {
		if hit, ok := missingIdentityFor(sd, scan); ok {
			out = append(out, hit)
		}
	}
	return out
}

// collectMutatedStructs returns the set of struct type names with at least
// one field assignment in pkg. Value objects are never mutated.
func collectMutatedStructs(pkg *packages.Package) map[string]bool {
	out := map[string]bool{}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			for _, e := range assignedExprs(n) {
				if name := selectorStructName(e, pkg.TypesInfo); name != "" {
					out[name] = true
				}
			}
			return true
		})
	}
	return out
}

// assignedExprs returns the assigned-to expressions of n if it is an
// assignment statement, else nil.
func assignedExprs(n ast.Node) []ast.Expr {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		return stmt.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{stmt.X}
	}
	return nil
}

// selectorStructName returns the type name if e is a field access on a
// value of a named struct type, else "".
func selectorStructName(e ast.Expr, info *types.Info) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	t := info.TypeOf(sel.X)
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return ""
	}
	return named.Obj().Name()
}

// countEntityUses counts entity-like uses (maps, slices, params) per type name.
func countEntityUses(pkg *packages.Package) map[string]int {
	uses := map[string]int{}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			countNodeUses(n, uses)
			return true
		})
	}
	return uses
}

// countNodeUses counts one AST node's entity-like type uses.
func countNodeUses(n ast.Node, uses map[string]int) {
	switch node := n.(type) {
	case *ast.MapType:
		if ident := typeIdent(node.Value); ident != "" {
			uses[ident]++
		}
	case *ast.ArrayType:
		if ident := typeIdent(node.Elt); ident != "" {
			uses[ident]++
		}
	case *ast.FuncDecl:
		countParamUses(node, uses)
	}
}

// countParamUses counts entity-like param types in a function declaration.
func countParamUses(fn *ast.FuncDecl, uses map[string]int) {
	if fn.Type.Params == nil {
		return
	}
	for _, field := range fn.Type.Params.List {
		if ident := typeIdent(field.Type); ident != "" {
			uses[ident]++
		}
	}
}

// missingIdentityFor returns a hit if the struct is used as an entity
// 3+ times without an ID field. Value objects (never mutated) are skipped:
// in DDD they are immutable and identified by their attributes, not by
// identity. Methods don't matter — value objects can have behavior.
func missingIdentityFor(sd structDef, scan identityScan) (patterns.MissingIdentityHit, bool) {
	if scan.uses[sd.name] < 3 {
		return patterns.MissingIdentityHit{}, false
	}
	if !scan.mutated[sd.name] {
		return patterns.MissingIdentityHit{}, false
	}
	if hasIDField(scan.pkg, sd.name) {
		return patterns.MissingIdentityHit{}, false
	}
	return patterns.MissingIdentityHit{
		TypeName: sd.name,
		Path:     sd.path,
		Line:     sd.line,
		EndLine:  sd.endLine,
		UseCount: scan.uses[sd.name],
	}, true
}

// typeIdent returns the type name for simple identifier or pointer types.
func typeIdent(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeIdent(t.X)
	}
	return ""
}

// hasIDField checks if the named struct type has an ID-like field.
func hasIDField(pkg *packages.Package, typeName string) bool {
	for _, file := range pkg.Syntax {
		if fileHasIDField(file, typeName) {
			return true
		}
	}
	return false
}

// fileHasIDField checks one file for the struct's ID field.
func fileHasIDField(file *ast.File, typeName string) bool {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			if specHasIDField(spec, typeName) {
				return true
			}
		}
	}
	return false
}

// specHasIDField checks one type spec for an ID field.
func specHasIDField(spec ast.Spec, typeName string) bool {
	ts, ok := spec.(*ast.TypeSpec)
	if !ok || ts.Name.Name != typeName {
		return false
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		return false
	}
	return structHasIDField(st)
}

// structHasIDField checks a struct type for ID-like field names.
func structHasIDField(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if isIDField(name.Name) {
				return true
			}
		}
	}
	return false
}
