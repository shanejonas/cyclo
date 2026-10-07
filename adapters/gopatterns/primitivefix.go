package gopatterns

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
)

// Primitive-obsession auto-fix: introduce named types for domain concepts
// hiding behind raw string/int params. When a param named "email" (or
// "userId", "phoneNumber", etc.) is a raw string, the fixer generates
// `type Email string`, updates the signature, and wraps call-site args.
//
// No LLM, no tokens — pure AST mechanics. The transform is
// behavior-preserving when all safety checks pass; concepts that fail any
// check are skipped.

// PrimitiveFix describes one applied primitive-obsession fix.
type PrimitiveFix struct {
	// Line is the line of the generated type declaration.
	Line int
	// TypeName is the generated type name, e.g. "Email".
	TypeName string
	// Kind is always "primitive_obsession".
	Kind string
}

// FixLine implements Fix.
func (p PrimitiveFix) FixLine() int { return p.Line }

// FixKind implements Fix.
func (p PrimitiveFix) FixKind() string { return p.Kind }

// primitiveConceptInfo is one fixable concept occurrence.
type primitiveConceptInfo struct {
	fn       *ast.FuncDecl
	param    *ast.Ident
	concept  string
	typeName string
	typ      string // "string" or "int"
}

// FixPrimitiveObsession introduces named types for domain-concept params.
// It returns the rewritten source (gofmt-clean) and the fixes applied.
func FixPrimitiveObsession(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []PrimitiveFix, error) {
	infos := findPrimitiveConcepts(f)
	if len(infos) == 0 {
		return src, nil, nil
	}
	fixes := applyConcepts(fset, f, groupByConcept(infos))
	if len(fixes) == 0 {
		return src, nil, nil
	}
	return formatFixed(fset, f, fixes)
}

// groupByConcept groups concept infos by concept name.
func groupByConcept(infos []primitiveConceptInfo) map[string][]primitiveConceptInfo {
	byConcept := make(map[string][]primitiveConceptInfo)
	for _, info := range infos {
		byConcept[info.concept] = append(byConcept[info.concept], info)
	}
	return byConcept
}

// applyConcepts applies the fix for each concept with 2+ safe occurrences.
func applyConcepts(fset *token.FileSet, f *ast.File, byConcept map[string][]primitiveConceptInfo) []PrimitiveFix {
	var fixes []PrimitiveFix
	for concept, group := range byConcept {
		if fix, ok := applyOneConcept(fset, f, concept, group); ok {
			fixes = append(fixes, fix)
		}
	}
	return fixes
}

// applyOneConcept fixes a single concept group. Returns false to skip.
func applyOneConcept(fset *token.FileSet, f *ast.File, concept string, group []primitiveConceptInfo) (PrimitiveFix, bool) {
	var zero PrimitiveFix
	if len(group) < 2 {
		return zero, false // need 2+ functions, like the detector
	}
	typeName := primitiveFixTypeName(concept)
	if !groupSafe(group) {
		return zero, false
	}
	if !applyPrimitiveConcept(fset, f, group, typeName) {
		return zero, false
	}
	return PrimitiveFix{
		Line:     fset.Position(f.Pos()).Line,
		TypeName: typeName,
		Kind:     "primitive_obsession",
	}, true
}

// groupSafe reports whether every occurrence in the group is fixable.
func groupSafe(group []primitiveConceptInfo) bool {
	names := make(map[string]bool, len(group))
	for _, info := range group {
		names[info.fn.Name.Name] = true
	}
	for _, info := range group {
		if !primitiveParamSafe(info.fn, info.param.Name, names) {
			return false
		}
	}
	return true
}

// formatFixed renders the fixed AST, normalizing positions via re-parse.
func formatFixed(fset *token.FileSet, f *ast.File, fixes []PrimitiveFix) ([]byte, []PrimitiveFix, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, nil, fmt.Errorf("format after primitive_obsession fix: %w", err)
	}
	// Re-parse and re-format to normalize stale positions (e.g. trailing
	// commas from replaced types). format.Node preserves position-based
	// commas; a fresh parse gives clean positions.
	clean, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("re-format after primitive_obsession fix: %w", err)
	}
	return clean, fixes, nil
}

// findPrimitiveConcepts finds string/int params with domain-concept names.
func findPrimitiveConcepts(f *ast.File) []primitiveConceptInfo {
	var out []primitiveConceptInfo
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type.Params == nil {
			continue
		}
		out = append(out, funcConcepts(fn)...)
	}
	return out
}

// funcConcepts finds concept params in one function.
func funcConcepts(fn *ast.FuncDecl) []primitiveConceptInfo {
	var out []primitiveConceptInfo
	for _, field := range fn.Type.Params.List {
		out = append(out, fieldConcepts(fn, field)...)
	}
	return out
}

// fieldConcepts finds concept params in one param field.
func fieldConcepts(fn *ast.FuncDecl, field *ast.Field) []primitiveConceptInfo {
	typ := primitiveFieldType(field)
	if typ == "" {
		return nil
	}
	var out []primitiveConceptInfo
	for _, name := range field.Names {
		if info, ok := makeConceptInfo(fn, name, typ); ok {
			out = append(out, info)
		}
	}
	return out
}

// primitiveFieldType returns "string" or "int" for plain basic-typed
// fields, "" otherwise (named types are already domain types).
func primitiveFieldType(field *ast.Field) string {
	ident, ok := field.Type.(*ast.Ident)
	if !ok {
		return ""
	}
	if ident.Name == "string" || ident.Name == "int" {
		return ident.Name
	}
	return ""
}

// makeConceptInfo builds the info for one param, or false if no concept.
func makeConceptInfo(fn *ast.FuncDecl, name *ast.Ident, typ string) (primitiveConceptInfo, bool) {
	var zero primitiveConceptInfo
	concept := primitiveFixConcept(name.Name)
	if concept == "" {
		return zero, false
	}
	return primitiveConceptInfo{
		fn:       fn,
		param:    name,
		concept:  concept,
		typeName: primitiveFixTypeName(concept),
		typ:      typ,
	}, true
}

// primitiveFixConcept extracts the domain concept from a param name.
// Mirrors domain/patterns primitiveConcept (word-boundary matching).
func primitiveFixConcept(name string) string {
	words := splitFixWords(name)
	for _, w := range words {
		for _, kw := range primitiveFixKeywords {
			if w == kw {
				return kw
			}
		}
	}
	return ""
}

// primitiveFixKeywords mirrors the detector's keyword list.
var primitiveFixKeywords = []string{
	"email", "phone", "url", "uri", "uuid", "guid", "id", "name",
	"address", "street", "city", "state", "zip", "postal", "country",
	"currency", "price", "amount", "quantity", "ssn", "passport",
	"license", "username", "password", "token", "apikey", "secret",
	"creditcard", "iban", "isbn", "sku", "serial",
}

// splitFixWords splits on camelCase and _/- boundaries, lowercased.
func splitFixWords(name string) []string {
	var words []string
	var cur strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r == '_' || r == '-' {
			words, cur = flushFixWord(words, &cur)
			continue
		}
		if isFixCamelBoundary(runes, i) {
			words, cur = flushFixWord(words, &cur)
		}
		cur.WriteRune(toFixLower(r))
	}
	words, _ = flushFixWord(words, &cur)
	return words
}

// flushFixWord appends the current word if non-empty.
func flushFixWord(words []string, cur *strings.Builder) ([]string, strings.Builder) {
	if cur.Len() > 0 {
		words = append(words, cur.String())
		cur.Reset()
	}
	return words, *cur
}

// isFixCamelBoundary reports a lowercase->uppercase transition.
func isFixCamelBoundary(runes []rune, i int) bool {
	if i == 0 || !isFixUpper(runes[i]) {
		return false
	}
	return isFixLower(runes[i-1]) || isFixDigit(runes[i-1])
}

// isFixUpper reports ASCII uppercase.
func isFixUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

// isFixLower reports ASCII lowercase.
func isFixLower(r rune) bool { return r >= 'a' && r <= 'z' }

// isFixDigit reports ASCII digit.
func isFixDigit(r rune) bool { return r >= '0' && r <= '9' }

// toFixLower lowercases an ASCII uppercase letter.
func toFixLower(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// primitiveFixInitialisms maps concepts to their Go initialism form.
var primitiveFixInitialisms = map[string]string{
	"id":   "ID",
	"url":  "URL",
	"uri":  "URI",
	"uuid": "UUID",
	"guid": "GUID",
	"ssn":  "SSN",
	"isbn": "ISBN",
	"iban": "IBAN",
	"sku":  "SKU",
}

// primitiveFixTypeName renders the type name, honoring initialisms.
func primitiveFixTypeName(concept string) string {
	if init, ok := primitiveFixInitialisms[concept]; ok {
		return init
	}
	if concept == "" {
		return ""
	}
	return strings.ToUpper(concept[:1]) + concept[1:]
}

// primitiveParamSafe reports whether the param can be safely converted to
// a named type. A named string type is neither comparable nor assignable
// with a plain string, so the fixer is conservative: every use of the param
// in the body must be either an argument to a group-function call (which the
// fixer rewrites with a conversion) or an operand of ==/!= against an
// untyped constant (assignable to the named type). Anything else —
// assignment, concatenation, typed comparisons, returns, other calls —
// would not compile, so the fix is skipped.
func primitiveParamSafe(fn *ast.FuncDecl, paramName string, groupFns map[string]bool) bool {
	if paramShadowed(fn, paramName) {
		return false
	}
	c := &paramUseChecker{paramName: paramName, groupFns: groupFns, safe: true}
	ast.Inspect(fn.Body, c.visit)
	return c.safe
}

// paramUseChecker walks a function body and reports whether every use of the
// named parameter stays valid after its type changes to a named type.
type paramUseChecker struct {
	paramName string
	groupFns  map[string]bool
	parents   []ast.Node
	safe      bool
}

func (c *paramUseChecker) visit(n ast.Node) bool {
	if n == nil {
		c.parents = c.parents[:len(c.parents)-1]
		return true
	}
	c.parents = append(c.parents, n)
	if !c.safe {
		return false
	}
	if id, ok := n.(*ast.Ident); ok && c.isUnsafeUse(id) {
		c.safe = false
		return false
	}
	return true
}

// isUnsafeUse reports whether one identifier use would not compile after
// the param's type changes.
func (c *paramUseChecker) isUnsafeUse(id *ast.Ident) bool {
	return id.Name == c.paramName && !paramUseSafe(c.parents, c.groupFns)
}

// paramUseSafe reports whether one use of the param (the innermost node on
// the parents stack) stays valid after the type change.
func paramUseSafe(parents []ast.Node, groupFns map[string]bool) bool {
	if len(parents) < 2 {
		return false
	}
	id := parents[len(parents)-1]
	switch parent := parents[len(parents)-2].(type) {
	case *ast.CallExpr:
		return isRewrittenCallArg(parent, id, groupFns)
	case *ast.BinaryExpr:
		return isConstComparison(parent, id)
	default:
		return false
	}
}

// isRewrittenCallArg reports whether the ident is an argument of a
// group-function call, which the fixer rewrites with a type conversion.
func isRewrittenCallArg(call *ast.CallExpr, id ast.Node, groupFns map[string]bool) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || !groupFns[ident.Name] {
		return false
	}
	for _, arg := range call.Args {
		if arg == id {
			return true
		}
	}
	return false
}

// isConstComparison reports whether the binary expression compares the
// ident against an untyped constant, which is assignable to the named type.
func isConstComparison(bin *ast.BinaryExpr, id ast.Node) bool {
	if bin.Op != token.EQL && bin.Op != token.NEQ {
		return false
	}
	for _, operand := range []ast.Expr{bin.X, bin.Y} {
		if operand == id {
			continue
		}
		if !isUntypedConstant(operand) {
			return false
		}
	}
	return true
}

// paramShadowed reports whether the param name is redeclared in the body.
func paramShadowed(fn *ast.FuncDecl, paramName string) bool {
	shadowed := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if shadowed {
			return false
		}
		if declaresName(n, paramName) {
			shadowed = true
			return false
		}
		return true
	})
	return shadowed
}

// declaresName reports whether the node declares the name via := or var.
func declaresName(n ast.Node, paramName string) bool {
	switch x := n.(type) {
	case *ast.AssignStmt:
		return x.Tok == token.DEFINE && lhsHasName(x.Lhs, paramName)
	case *ast.ValueSpec:
		return valueSpecHasName(x, paramName)
	}
	return false
}

// valueSpecHasName reports whether a var spec declares the name.
func valueSpecHasName(spec *ast.ValueSpec, paramName string) bool {
	for _, name := range spec.Names {
		if name.Name == paramName {
			return true
		}
	}
	return false
}

// lhsHasName reports whether any lhs expression is the named identifier.
func lhsHasName(lhs []ast.Expr, name string) bool {
	for _, e := range lhs {
		if ident, ok := e.(*ast.Ident); ok && ident.Name == name {
			return true
		}
	}
	return false
}

// applyPrimitiveConcept applies the transform for one concept:
// 1. Add `type TypeName string` after imports.
// 2. Update param types in signatures.
// 3. Wrap call-site args that aren't untyped constants.
func applyPrimitiveConcept(fset *token.FileSet, f *ast.File, group []primitiveConceptInfo, typeName string) bool {
	insertTypeDecl(f, group[0].typ, typeName)
	updateSignatures(group, typeName)
	return wrapGroupCallSites(f, group, typeName)
}

// insertTypeDecl adds `type TypeName typ` after the imports. The new
// declaration is anchored to the end of the import block (or the package
// clause when there are no imports) so the printer keeps the type keyword
// and its spec together: a position-less GenDecl lets a following doc
// comment split them apart, emitting `type` ... docs ... `Name string`.
func insertTypeDecl(f *ast.File, typ, typeName string) {
	anchor := f.Name.End()
	insertAt := 0
	for i, decl := range f.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			anchor = gen.End()
			insertAt = i + 1
		}
	}
	typeDecl := &ast.GenDecl{
		Tok:    token.TYPE,
		TokPos: anchor,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: &ast.Ident{Name: typeName, NamePos: anchor},
				Type: &ast.Ident{Name: typ},
			},
		},
	}
	f.Decls = append(f.Decls[:insertAt], append([]ast.Decl{typeDecl}, f.Decls[insertAt:]...)...)
}

// updateSignatures rewrites param types to the named type.
// Positions reset to NoPos so the printer doesn't emit stale commas.
func updateSignatures(group []primitiveConceptInfo, typeName string) {
	for _, info := range group {
		for _, field := range info.fn.Type.Params.List {
			for _, name := range field.Names {
				if name.Name == info.param.Name {
					newType := &ast.Ident{Name: typeName}
					newType.NamePos = token.NoPos
					field.Type = newType
				}
			}
		}
	}
}

// wrapGroupCallSites wraps call-site args for every function in the group.
func wrapGroupCallSites(f *ast.File, group []primitiveConceptInfo, typeName string) bool {
	for _, info := range group {
		if !wrapCallSites(f, info.fn.Name.Name, info.param.Name, info.fn, typeName) {
			return false
		}
	}
	return true
}

// wrapCallSites wraps args at call sites of funcName. args that are untyped
// constants (string/int literals) need no wrapping; others get TypeName(...).
// Returns false if any call cannot be safely rewritten.
func wrapCallSites(f *ast.File, funcName, paramName string, fn *ast.FuncDecl, typeName string) bool {
	targetPos, ok := paramPosition(fn, paramName)
	if !ok {
		return false
	}
	w := &callWrapper{funcName: funcName, pos: targetPos, typeName: typeName, ok: true}
	astutil.Apply(f, w.visit, nil)
	return w.ok
}

// paramPosition returns the arg index of the named param.
func paramPosition(fn *ast.FuncDecl, paramName string) (int, bool) {
	pos := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == paramName {
				return pos, true
			}
			pos++
		}
	}
	return 0, false
}

// callWrapper rewrites call-site args via astutil.
type callWrapper struct {
	funcName string
	pos      int
	typeName string
	ok       bool
}

// visit handles one node during the call-site rewrite.
func (w *callWrapper) visit(c *astutil.Cursor) bool {
	call, isCall := c.Node().(*ast.CallExpr)
	if !isCall || !isNamedCall(call, w.funcName) {
		return true
	}
	if w.pos >= len(call.Args) {
		w.ok = false
		return false
	}
	w.wrapArg(call)
	return true
}

// isNamedCall reports whether call is a direct call to funcName.
func isNamedCall(call *ast.CallExpr, funcName string) bool {
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == funcName
}

// wrapArg wraps the arg at the wrapper's pos with TypeName(...), unless
// it's an untyped constant (assignable without conversion).
func (w *callWrapper) wrapArg(call *ast.CallExpr) {
	arg := call.Args[w.pos]
	if isUntypedConstant(arg) {
		return
	}
	call.Args[w.pos] = &ast.CallExpr{
		Fun:  &ast.Ident{Name: w.typeName, NamePos: token.NoPos},
		Args: []ast.Expr{arg},
	}
}

// isUntypedConstant reports whether expr is a string/int literal (untyped
// constant, assignable to a named string/int type without conversion).
func isUntypedConstant(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok {
		return false
	}
	return lit.Kind == token.STRING || lit.Kind == token.INT
}
