package gopatterns

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/tools/go/ast/astutil"
)

// Value-object auto-fix: extract data clumps into immutable structs.
// When the same primitive params travel together across functions, they
// describe a domain concept that wants to be a value object. This fixer
// performs the extraction mechanically: no LLM, no tokens.

// ClumpParam is a parameter's name and primitive type name.
type ClumpParam struct {
	Name string
	Type string
}

// ValueClump is a data clump: params shared by functions.
type ValueClump struct {
	Params []ClumpParam
	Funcs  []*ast.FuncDecl
}

// ValueFix describes one applied value-object extraction.
type ValueFix struct {
	// Line is the line of the generated struct definition.
	Line int
	// TypeName is the generated struct name, e.g. "AmountCurrency".
	TypeName string
	// Kind is always "value_object".
	Kind string
}

// valueObjectMinClump is the minimum params in a fixable clump.
const valueObjectMinClump = 2

// valueObjectMinFuncs is the minimum functions sharing a clump.
const valueObjectMinFuncs = 3

// FixValueObjects extracts data clumps into value-object structs.
// It returns the rewritten source (gofmt-clean) and the fixes applied.
// The transform is behavior-preserving when all safety checks pass;
// clumps that fail any check are skipped.
func FixValueObjects(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []ValueFix, error) {
	clumps := FindValueClumps(f)
	if len(clumps) == 0 {
		return src, nil, nil
	}
	var fixes []ValueFix
	for _, clump := range clumps {
		fix, ok := applyValueClump(fset, f, clump)
		if !ok {
			continue
		}
		fixes = append(fixes, fix)
	}
	if len(fixes) == 0 {
		return src, nil, nil
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, nil, fmt.Errorf("format after value_object fix: %w", err)
	}
	return buf.Bytes(), fixes, nil
}

// FindValueClumps detects data clumps in f: groups of primitive params
// shared by at least valueObjectMinFuncs functions.
func FindValueClumps(f *ast.File) []ValueClump {
	infos := collectFuncParams(f)
	byKey := make(map[string]*ValueClump)
	for i := 0; i < len(infos); i++ {
		for j := i + 1; j < len(infos); j++ {
			recordClumpIntersection(byKey, infos[i], infos[j])
		}
	}
	var clumps []ValueClump
	for _, c := range byKey {
		if len(c.Funcs) >= valueObjectMinFuncs {
			clumps = append(clumps, *c)
		}
	}
	return maximalValueClumps(clumps)
}

// funcParamInfo pairs a function with its primitive param set.
type funcParamInfo struct {
	decl *ast.FuncDecl
	keys map[string]ClumpParam
}

// collectFuncParams gathers primitive params for each function with enough.
func collectFuncParams(f *ast.File) []funcParamInfo {
	var infos []funcParamInfo
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || !hasParamList(fn) {
			return true
		}
		if keys := primitiveParamKeys(fn); len(keys) >= valueObjectMinClump {
			infos = append(infos, funcParamInfo{decl: fn, keys: keys})
		}
		return true
	})
	return infos
}

// hasParamList reports whether fn has a non-nil param list and a body.
func hasParamList(fn *ast.FuncDecl) bool {
	return fn.Body != nil && fn.Type.Params != nil
}

// primitiveParamKeys builds the name:type key set for a function's params.
func primitiveParamKeys(fn *ast.FuncDecl) map[string]ClumpParam {
	keys := make(map[string]ClumpParam)
	for _, field := range fn.Type.Params.List {
		typeName, ok := primitiveTypeName(field.Type)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			key := name.Name + ":" + typeName
			keys[key] = ClumpParam{Name: name.Name, Type: typeName}
		}
	}
	return keys
}

// primitiveTypeName returns the type name if expr is a primitive type.
func primitiveTypeName(expr ast.Expr) (string, bool) {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return "", false
	}
	switch ident.Name {
	case "bool", "string", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128", "byte", "rune":
		return ident.Name, true
	}
	return "", false
}

// recordClumpIntersection adds the param intersection of two functions.
func recordClumpIntersection(byKey map[string]*ValueClump, a, b funcParamInfo) {
	keys := intersectSorted(a.keys, b.keys)
	if len(keys) < valueObjectMinClump {
		return
	}
	c := ensureClump(byKey, keys, a.keys)
	c.Funcs = appendUniqueFunc(c.Funcs, a.decl)
	c.Funcs = appendUniqueFunc(c.Funcs, b.decl)
}

// intersectSorted returns the sorted keys present in both sets.
func intersectSorted(a, b map[string]ClumpParam) []string {
	var keys []string
	for k := range a {
		if _, ok := b[k]; ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// ensureClump returns the clump for key, creating it if needed.
func ensureClump(byKey map[string]*ValueClump, keys []string, params map[string]ClumpParam) *ValueClump {
	key := strings.Join(keys, ",")
	if c, ok := byKey[key]; ok {
		return c
	}
	var plist []ClumpParam
	for _, k := range keys {
		plist = append(plist, params[k])
	}
	c := &ValueClump{Params: plist}
	byKey[key] = c
	return c
}

// appendUniqueFunc appends fn if not already present.
func appendUniqueFunc(funcs []*ast.FuncDecl, fn *ast.FuncDecl) []*ast.FuncDecl {
	for _, f := range funcs {
		if f == fn {
			return funcs
		}
	}
	return append(funcs, fn)
}

// maximalValueClumps drops clumps that are strict subsets of another.
func maximalValueClumps(clumps []ValueClump) []ValueClump {
	var out []ValueClump
	for i, c := range clumps {
		if !isSubsetOfAnother(c, clumps, i) {
			out = append(out, c)
		}
	}
	return out
}

// isSubsetOfAnother reports whether c is a strict subset of another clump.
func isSubsetOfAnother(c ValueClump, clumps []ValueClump, idx int) bool {
	for j, other := range clumps {
		if idx != j && isClumpSubset(c.Params, other.Params) {
			return true
		}
	}
	return false
}

// isClumpSubset reports whether a's params are a strict subset of b's.
func isClumpSubset(a, b []ClumpParam) bool {
	if len(a) >= len(b) {
		return false
	}
	bkeys := make(map[string]bool, len(b))
	for _, p := range b {
		bkeys[p.Name+":"+p.Type] = true
	}
	for _, p := range a {
		if !bkeys[p.Name+":"+p.Type] {
			return false
		}
	}
	return true
}

// applyValueClump extracts one clump into a struct. Returns false when any
// safety check fails.
func applyValueClump(fset *token.FileSet, f *ast.File, clump ValueClump) (ValueFix, bool) {
	typeName := valueTypeName(clump.Params)
	paramName := lowerFirst(typeName)
	if !clumpSafe(clump, paramName) {
		return ValueFix{}, false
	}
	if !rewriteClumpCallSites(f, clump, typeName) {
		return ValueFix{}, false
	}
	structDecl := buildValueStruct(typeName, clump.Params)
	insertValueStruct(f, structDecl)
	for _, fn := range clump.Funcs {
		if !rewriteFuncSignature(fn, clump.Params, paramName, typeName) {
			return ValueFix{}, false
		}
		rewriteBodyIdents(fn.Body, clump.Params, paramName)
	}
	line := fset.PositionFor(structDecl.Pos(), false).Line
	return ValueFix{Line: line, TypeName: typeName, Kind: "value_object"}, true
}

// clumpSafe runs all safety checks for a clump.
func clumpSafe(clump ValueClump, paramName string) bool {
	if !allUnexported(clump.Funcs) {
		return false
	}
	paramNames := clumpParamNames(clump.Params)
	for _, fn := range clump.Funcs {
		if !funcSafe(fn, paramNames, paramName) {
			return false
		}
	}
	return true
}

// funcSafe runs the per-function safety checks.
func funcSafe(fn *ast.FuncDecl, paramNames map[string]bool, paramName string) bool {
	if fn.Recv != nil {
		return false
	}
	if paramsModified(fn.Body, paramNames) {
		return false
	}
	if shadowsParam(fn.Body, paramNames) {
		return false
	}
	return !identUsed(fn.Body, paramName)
}

// rewriteClumpCallSites rewrites call sites for all functions in the clump.
// It must run before signatures are rewritten (needs original positions).
func rewriteClumpCallSites(f *ast.File, clump ValueClump, typeName string) bool {
	for _, fn := range clump.Funcs {
		positions := clumpParamPositions(fn, clump.Params)
		if positions == nil {
			return false
		}
		if !rewriteCallSites(f, fn.Name.Name, positions, clump, typeName) {
			return false
		}
	}
	return true
}

// clumpParamPositions returns the arg indices of clump params in fn's
// signature, in clump order. Returns nil if not found.
func clumpParamPositions(fn *ast.FuncDecl, params []ClumpParam) []int {
	if fn.Type.Params == nil {
		return nil
	}
	posByKey := paramPositionIndex(fn)
	var positions []int
	for _, p := range params {
		pos, ok := posByKey[p.Name+":"+p.Type]
		if !ok {
			return nil
		}
		positions = append(positions, pos)
	}
	return positions
}

// paramPositionIndex maps name:type keys to flat param positions.
func paramPositionIndex(fn *ast.FuncDecl) map[string]int {
	posByKey := make(map[string]int)
	idx := 0
	for _, field := range fn.Type.Params.List {
		idx = indexFieldParams(field, posByKey, idx)
	}
	return posByKey
}

// indexFieldParams records positions for one param field.
func indexFieldParams(field *ast.Field, posByKey map[string]int, idx int) int {
	typeName, ok := primitiveTypeName(field.Type)
	for _, name := range field.Names {
		if ok {
			posByKey[name.Name+":"+typeName] = idx
		}
		idx++
	}
	return idx
}

// allUnexported reports whether all functions are unexported.
func allUnexported(funcs []*ast.FuncDecl) bool {
	for _, fn := range funcs {
		if !isUnexportedFunc(fn) {
			return false
		}
	}
	return true
}

// valueTypeName builds "AmountCurrency" from params.
func valueTypeName(params []ClumpParam) string {
	var sb strings.Builder
	for _, p := range params {
		sb.WriteString(capitalize(p.Name))
	}
	return sb.String()
}

// capitalize uppercases the first rune.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// lowerFirst lowercases the first rune.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// clumpParamNames returns the param names as a set.
func clumpParamNames(params []ClumpParam) map[string]bool {
	out := make(map[string]bool, len(params))
	for _, p := range params {
		out[p.Name] = true
	}
	return out
}

// isUnexportedFunc reports whether the function name is unexported.
func isUnexportedFunc(fn *ast.FuncDecl) bool {
	name := fn.Name.Name
	if name == "" {
		return false
	}
	return unicode.IsLower([]rune(name)[0])
}

// paramsModified reports whether any param is assigned in the body.
func paramsModified(body *ast.BlockStmt, params map[string]bool) bool {
	modified := false
	ast.Inspect(body, func(n ast.Node) bool {
		if modified {
			return false
		}
		if modifiesParam(n, params) {
			modified = true
			return false
		}
		return true
	})
	return modified
}

// modifiesParam reports whether n assigns to a clump param.
func modifiesParam(n ast.Node, params map[string]bool) bool {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		return assignTargetsParam(stmt, params)
	case *ast.IncDecStmt:
		if ident, ok := stmt.X.(*ast.Ident); ok {
			return params[ident.Name]
		}
	}
	return false
}

// assignTargetsParam reports whether an assignment targets a clump param.
func assignTargetsParam(stmt *ast.AssignStmt, params map[string]bool) bool {
	for _, lhs := range stmt.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && params[ident.Name] {
			return true
		}
	}
	return false
}

// shadowsParam reports whether the body defines any identifier with a
// clump param name (via :=, var, nested func params, range vars, ...).
// Rewriting uses would be wrong under shadowing, so we skip.
func shadowsParam(body *ast.BlockStmt, params map[string]bool) bool {
	shadowed := false
	ast.Inspect(body, func(n ast.Node) bool {
		if shadowed {
			return false
		}
		if definesParamName(n, params) {
			shadowed = true
			return false
		}
		return true
	})
	return shadowed
}

// definesParamName reports whether n defines an identifier with a param name.
func definesParamName(n ast.Node, params map[string]bool) bool {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		return assignDefines(stmt, params)
	case *ast.ValueSpec:
		return specDefines(stmt, params)
	case *ast.FuncLit:
		return funcLitDefines(stmt, params)
	case *ast.RangeStmt:
		return rangeDefines(stmt, params)
	}
	return false
}

// assignDefines checks := assignments.
func assignDefines(stmt *ast.AssignStmt, params map[string]bool) bool {
	if stmt.Tok != token.DEFINE {
		return false
	}
	for _, lhs := range stmt.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && params[ident.Name] {
			return true
		}
	}
	return false
}

// specDefines checks var declarations.
func specDefines(stmt *ast.ValueSpec, params map[string]bool) bool {
	for _, name := range stmt.Names {
		if params[name.Name] {
			return true
		}
	}
	return false
}

// funcLitDefines checks nested function literal params.
func funcLitDefines(stmt *ast.FuncLit, params map[string]bool) bool {
	if stmt.Type.Params == nil {
		return false
	}
	for _, field := range stmt.Type.Params.List {
		for _, name := range field.Names {
			if params[name.Name] {
				return true
			}
		}
	}
	return false
}

// rangeDefines checks range variables.
func rangeDefines(stmt *ast.RangeStmt, params map[string]bool) bool {
	for _, e := range []ast.Expr{stmt.Key, stmt.Value} {
		if ident, ok := e.(*ast.Ident); ok && params[ident.Name] {
			return true
		}
	}
	return false
}

// identUsed reports whether name appears as an identifier in the body.
func identUsed(body *ast.BlockStmt, name string) bool {
	used := false
	ast.Inspect(body, func(n ast.Node) bool {
		if used {
			return false
		}
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
			used = true
			return false
		}
		return true
	})
	return used
}

// buildValueStruct creates the struct declaration.
func buildValueStruct(typeName string, params []ClumpParam) *ast.GenDecl {
	var fields []*ast.Field
	for _, p := range params {
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{{Name: capitalize(p.Name)}},
			Type:  &ast.Ident{Name: p.Type},
		})
	}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: &ast.Ident{Name: typeName},
				Type: &ast.StructType{Fields: &ast.FieldList{List: fields}},
			},
		},
	}
}

// insertValueStruct inserts the struct after imports, before first func.
func insertValueStruct(f *ast.File, decl *ast.GenDecl) {
	idx := 0
	for i, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if ok && gd.Tok == token.IMPORT {
			idx = i + 1
		}
	}
	f.Decls = append(f.Decls[:idx], append([]ast.Decl{decl}, f.Decls[idx:]...)...)
}

// rewriteFuncSignature replaces clump params with the struct param.
// Returns false if the params aren't found.
func rewriteFuncSignature(fn *ast.FuncDecl, params []ClumpParam, paramName, structType string) bool {
	if fn.Type.Params == nil {
		return false
	}
	clumpKeys := make(map[string]bool, len(params))
	for _, p := range params {
		clumpKeys[p.Name+":"+p.Type] = true
	}
	builder := &sigBuilder{clumpKeys: clumpKeys, paramName: paramName, structType: structType}
	for _, field := range fn.Type.Params.List {
		builder.visitField(field)
	}
	if !builder.inserted {
		return false
	}
	fn.Type.Params.List = builder.newList
	return true
}

// sigBuilder accumulates the rewritten param list.
type sigBuilder struct {
	clumpKeys  map[string]bool
	paramName  string
	structType string
	newList    []*ast.Field
	inserted   bool
}

// visitField processes one param field, dropping clump params.
func (b *sigBuilder) visitField(field *ast.Field) {
	primType, ok := primitiveTypeName(field.Type)
	var keep []*ast.Ident
	for _, name := range field.Names {
		if ok && b.clumpKeys[name.Name+":"+primType] {
			b.insertStructParam()
			continue
		}
		keep = append(keep, name)
	}
	if len(keep) > 0 {
		field.Names = keep
		b.newList = append(b.newList, field)
	}
}

// insertStructParam adds the struct param once.
func (b *sigBuilder) insertStructParam() {
	if b.inserted {
		return
	}
	b.newList = append(b.newList, &ast.Field{
		Names: []*ast.Ident{{Name: b.paramName}},
		Type:  &ast.Ident{Name: b.structType},
	})
	b.inserted = true
}

// rewriteBodyIdents replaces param uses with struct field accesses.
// E.g. `amount` becomes `amountCurrency.Amount`.
func rewriteBodyIdents(body *ast.BlockStmt, params []ClumpParam, paramName string) {
	fieldByParam := make(map[string]string, len(params))
	for _, p := range params {
		fieldByParam[p.Name] = capitalize(p.Name)
	}
	astutil.Apply(body, func(c *astutil.Cursor) bool {
		ident, ok := c.Node().(*ast.Ident)
		if !ok {
			return true
		}
		field, ok := fieldByParam[ident.Name]
		if !ok {
			return true
		}
		// Don't replace if this ident is a field name in a struct literal
		// or a selector (already qualified).
		if isSelectorOrFieldName(c, ident) {
			return true
		}
		c.Replace(&ast.SelectorExpr{
			X:   &ast.Ident{Name: paramName},
			Sel: &ast.Ident{Name: field},
		})
		return true
	}, nil)
}

// isSelectorOrFieldName reports whether the ident is a selector or field name.
func isSelectorOrFieldName(c *astutil.Cursor, ident *ast.Ident) bool {
	parent := c.Parent()
	if parent == nil {
		return false
	}
	// If parent is a SelectorExpr and we're the Sel, don't replace.
	if sel, ok := parent.(*ast.SelectorExpr); ok && sel.Sel == ident {
		return true
	}
	// If parent is a KeyValueExpr and we're the Key, don't replace.
	if kv, ok := parent.(*ast.KeyValueExpr); ok && kv.Key == ident {
		return true
	}
	return false
}

// rewriteCallSites updates calls to funcName in f. positions are the arg
// indices of the clump params. Returns false if any call cannot be safely
// rewritten.
func rewriteCallSites(f *ast.File, funcName string, positions []int, clump ValueClump, typeName string) bool {
	var calls []*ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == funcName {
			calls = append(calls, call)
		}
		return true
	})
	for _, call := range calls {
		if !rewriteOneCall(call, positions, clump, typeName) {
			return false
		}
	}
	return true
}

// rewriteOneCall rewrites a single call site, replacing the clump args with
// a struct literal. Returns false if unsafe.
func rewriteOneCall(call *ast.CallExpr, positions []int, clump ValueClump, typeName string) bool {
	if !callArgsSafe(call, positions) {
		return false
	}
	lit := buildStructLiteral(call, positions, clump, typeName)
	call.Args = spliceStructArg(call.Args, positions, lit)
	return true
}

// callArgsSafe reports whether the call's args can be safely rewritten.
func callArgsSafe(call *ast.CallExpr, positions []int) bool {
	if call.Ellipsis.IsValid() {
		return false
	}
	maxPos := 0
	for _, p := range positions {
		if p > maxPos {
			maxPos = p
		}
	}
	return len(call.Args) > maxPos
}

// buildStructLiteral builds TypeName{Field: arg, ...} from the clump args.
func buildStructLiteral(call *ast.CallExpr, positions []int, clump ValueClump, typeName string) *ast.CompositeLit {
	var elts []ast.Expr
	for i, p := range positions {
		elts = append(elts, &ast.KeyValueExpr{
			Key:   &ast.Ident{Name: capitalize(clump.Params[i].Name)},
			Value: call.Args[p],
		})
	}
	return &ast.CompositeLit{
		Type: &ast.Ident{Name: typeName},
		Elts: elts,
	}
}

// spliceStructArg replaces the clump args with the struct literal.
// The literal goes where the first clump arg was.
func spliceStructArg(args []ast.Expr, positions []int, lit *ast.CompositeLit) []ast.Expr {
	posSet := make(map[int]bool, len(positions))
	for _, p := range positions {
		posSet[p] = true
	}
	var newArgs []ast.Expr
	inserted := false
	for i, arg := range args {
		if posSet[i] {
			if !inserted {
				newArgs = append(newArgs, lit)
				inserted = true
			}
			continue
		}
		newArgs = append(newArgs, arg)
	}
	return newArgs
}

// rewriteBodyIdents replaces param uses with struct field accesses.
// E.g. `amount` becomes `amountCurrency.Amount`.
