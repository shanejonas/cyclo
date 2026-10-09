package gopatterns

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// applyEnumDispatchFix converts an enum-value switch to a dispatch table.
// It handles the tractable subset: switch on a single variable, 2+ cases,
// each case body is a single function call with no arguments.
//
// Example:
//
//	switch t {
//	case TypeA:
//		doA()
//	case TypeB:
//		doB()
//	}
//
// becomes:
//
//	var typeDispatch = map[Type]func(){
//		TypeA: doA,
//		TypeB: doB,
//	}
//	...
//	if fn, ok := typeDispatch[t]; ok {
//		fn()
//	}
//
// caseEntry is one case in a dispatch switch.
type caseEntry struct {
	value string
	fn    string
}

func applyEnumDispatchFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	sw, err := validatedSwitch(fset, f, spec)
	if err != nil {
		return nil, err
	}
	cases, keyType, err := collectSwitchCases(fset, sw, src)
	if err != nil {
		return nil, err
	}
	return buildDispatchFix(fset, f, src, dispatchPlan{sw: sw, cases: cases, keyType: keyType})
}

// validatedSwitch finds and validates the switch statement.
func validatedSwitch(fset *token.FileSet, f *ast.File, spec *patterns.FixSpec) (*ast.SwitchStmt, error) {
	sw := findSwitchAtLine(fset, f, spec.Line)
	if sw == nil {
		return nil, fmt.Errorf("enum_dispatch: no switch at line %d", spec.Line)
	}
	if sw.Tag == nil {
		return nil, fmt.Errorf("enum_dispatch: switch has no tag")
	}
	if _, ok := sw.Tag.(*ast.TypeAssertExpr); ok {
		return nil, fmt.Errorf("enum_dispatch: type switches not supported")
	}
	return sw, nil
}

// collectSwitchCases extracts case entries and infers the key type.
func collectSwitchCases(fset *token.FileSet, sw *ast.SwitchStmt, src []byte) ([]caseEntry, string, error) {
	var cases []caseEntry
	var keyType string
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		entry, kt, err := parseCaseClause(fset, cc, src, keyType)
		if err != nil {
			return nil, "", err
		}
		keyType = kt
		cases = append(cases, entry)
	}
	if len(cases) < 2 {
		return nil, "", fmt.Errorf("enum_dispatch: need at least 2 cases")
	}
	if keyType == "" {
		return nil, "", fmt.Errorf("enum_dispatch: cannot infer key type")
	}
	return cases, keyType, nil
}

// parseCaseClause parses one case clause into a caseEntry.
func parseCaseClause(fset *token.FileSet, cc *ast.CaseClause, src []byte, keyType string) (caseEntry, string, error) {
	if len(cc.List) == 0 {
		return caseEntry{}, keyType, fmt.Errorf("enum_dispatch: default not supported")
	}
	if len(cc.Body) != 1 {
		return caseEntry{}, keyType, fmt.Errorf("enum_dispatch: case must be single statement")
	}
	fnName, err := caseCallTarget(cc)
	if err != nil {
		return caseEntry{}, keyType, err
	}
	val := cc.List[0]
	valText := srcText(fset, val, src)
	keyType = inferKeyType(val, keyType)
	return caseEntry{value: valText, fn: fnName}, keyType, nil
}

// caseCallTarget extracts the function name from a case body.
func caseCallTarget(cc *ast.CaseClause) (string, error) {
	exprStmt, ok := cc.Body[0].(*ast.ExprStmt)
	if !ok {
		return "", fmt.Errorf("enum_dispatch: case must be expression")
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return "", fmt.Errorf("enum_dispatch: case must be zero-arg call")
	}
	fnIdent, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf("enum_dispatch: only simple calls supported")
	}
	return fnIdent.Name, nil
}

// inferKeyType infers the map key type from a case value.
func inferKeyType(val ast.Expr, keyType string) string {
	sel, ok := val.(*ast.SelectorExpr)
	if !ok {
		return keyType
	}
	xIdent, ok := sel.X.(*ast.Ident)
	if !ok || (keyType != "" && keyType != xIdent.Name) {
		return keyType
	}
	return xIdent.Name
}

// dispatchPlan bundles a validated switch with its derived cases.
type dispatchPlan struct {
	sw      *ast.SwitchStmt
	cases   []caseEntry
	keyType string
}

// buildDispatchFix creates the text edits for the dispatch table.
func buildDispatchFix(fset *token.FileSet, f *ast.File, src []byte, plan dispatchPlan) ([]byte, error) {
	tagText := srcText(fset, plan.sw.Tag, src)
	tableName := strings.ToLower(plan.keyType) + "Dispatch"
	table := buildTable(tableName, plan.keyType, plan.cases)
	repl := buildLookup(tableName, tagText)
	fnDecl := findEnclosingFunc(f, plan.sw)
	if fnDecl == nil {
		return nil, fmt.Errorf("enum_dispatch: switch not in function")
	}
	fnStart := fset.Position(fnDecl.Pos()).Offset
	swStart := fset.Position(plan.sw.Pos()).Offset
	swEnd := fset.Position(plan.sw.End()).Offset
	edits := []textEdit{
		{start: fnStart, end: fnStart, replacement: []byte(table + "\n")},
		{start: swStart, end: swEnd, replacement: []byte(repl)},
	}
	return applyEdits(src, edits), nil
}

// buildTable generates the dispatch table source.
func buildTable(tableName, keyType string, cases []caseEntry) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("var %s = map[%s]func(){\n", tableName, keyType))
	for _, c := range cases {
		b.WriteString(fmt.Sprintf("\t%s: %s,\n", c.value, c.fn))
	}
	b.WriteString("}\n")
	return b.String()
}

// buildLookup generates the map lookup replacement.
func buildLookup(tableName, tagText string) string {
	return fmt.Sprintf("if fn, ok := %s[%s]; ok {\n\tfn()\n}\n", tableName, tagText)
}

// applyGenericFnFix converts identical functions differing only by type
// into a single generic function.
//
// Example:
//
//	func maxInt(a, b int) int { if a > b { return a }; return b }
//	func maxFloat(a, b float64) float64 { if a > b { return a }; return b }
//
// becomes:
//
//	func max[T cmp.Ordered](a, b T) T { if a > b { return a }; return b }
func applyGenericFnFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	// Find candidate function pairs: same structure, different types.
	// We look for functions with a common name prefix (e.g., maxInt/maxFloat).
	funcs := collectFuncDecls(f)
	// Group by normalized body.
	groups := groupByNormalizedBody(fset, funcs, src)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		// Try to generalize this group.
		out, err := generalizeGroup(fset, f, group, src)
		if err == nil {
			return out, nil
		}
		// If this group fails, try the next.
	}
	return nil, fmt.Errorf("generic_fn: no generalizable function pair found")
}

// collectFuncDecls returns all function declarations in the file.
func collectFuncDecls(f *ast.File) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			out = append(out, fn)
		}
		return true
	})
	return out
}

// groupByNormalizedBody groups functions with identical bodies modulo type names.
func groupByNormalizedBody(fset *token.FileSet, funcs []*ast.FuncDecl, src []byte) [][]*ast.FuncDecl {
	// Normalize each function body by replacing type names with placeholders.
	// For simplicity, we compare the printed body with type idents normalized.
	groups := make(map[string][]*ast.FuncDecl)
	for _, fn := range funcs {
		if fn.Recv != nil {
			continue // skip methods
		}
		key := normalizedBodyKey(fset, fn, src)
		groups[key] = append(groups[key], fn)
	}
	var out [][]*ast.FuncDecl
	for _, g := range groups {
		if len(g) >= 2 {
			out = append(out, g)
		}
	}
	return out
}

// normalizedBodyKey returns a string key for the function body with type
// names replaced by placeholders.
func normalizedBodyKey(fset *token.FileSet, fn *ast.FuncDecl, src []byte) string {
	typeNames := collectTypeNames(fset, fn, src)
	bodyText := srcText(fset, fn.Body, src)
	paramNames := collectParamNames(fn)
	key := normalizeTypes(bodyText, typeNames)
	return fmt.Sprintf("%d:%s:%s", len(paramNames), strings.Join(paramNames, ","), key)
}

// collectTypeNames gathers param and result type names.
func collectTypeNames(fset *token.FileSet, fn *ast.FuncDecl, src []byte) []string {
	var out []string
	for _, fl := range []*ast.FieldList{fn.Type.Params, fn.Type.Results} {
		if fl == nil {
			continue
		}
		for _, p := range fl.List {
			out = append(out, srcText(fset, p.Type, src))
		}
	}
	return out
}

// collectParamNames gathers parameter names.
func collectParamNames(fn *ast.FuncDecl) []string {
	var out []string
	if fn.Type.Params == nil {
		return out
	}
	for _, p := range fn.Type.Params.List {
		for _, n := range p.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

// normalizeTypes replaces type names with placeholder.
func normalizeTypes(bodyText string, typeNames []string) string {
	for _, tn := range typeNames {
		bodyText = strings.ReplaceAll(bodyText, tn, "T")
	}
	return bodyText
}

// generalizeGroup converts a group of identical-modulo-types functions
// into a single generic function.
func generalizeGroup(fset *token.FileSet, f *ast.File, group []*ast.FuncDecl, src []byte) ([]byte, error) {
	params, err := extractGeneralizeParams(fset, group, src)
	if err != nil {
		return nil, err
	}
	edits := buildGeneralizeEdits(fset, group, params, src)
	out := applyEdits(src, edits)
	if params.constraint == "cmp.Ordered" {
		out = ensureImport(out, "cmp")
	}
	return out, nil
}

// generalizeParams extracts the parameters for generalization.
type generalizeParams struct {
	baseName   string
	typeVars   map[string]string
	constraint string
	genericFn  string
}

func extractGeneralizeParams(fset *token.FileSet, group []*ast.FuncDecl, src []byte) (*generalizeParams, error) {
	if len(group) < 2 {
		return nil, fmt.Errorf("need at least 2 functions")
	}
	baseName := commonFuncPrefix(group)
	if baseName == "" {
		return nil, fmt.Errorf("no common name prefix")
	}
	typeVars := collectTypeVars(fset, group, src)
	if len(typeVars) == 0 {
		return nil, fmt.Errorf("no type variation found")
	}
	constraint := inferConstraint(fset, group[0], src)
	params := &generalizeParams{baseName: baseName, typeVars: typeVars, constraint: constraint}
	params.genericFn = buildGenericFunc(fset, group[0], src, params)
	return params, nil
}

// buildGeneralizeEdits creates the text edits for generalization.
func buildGeneralizeEdits(fset *token.FileSet, group []*ast.FuncDecl, params *generalizeParams, src []byte) []textEdit {
	var edits []textEdit
	first := group[0]
	firstStart := fset.Position(first.Pos()).Offset
	firstEnd := fset.Position(first.End()).Offset
	edits = append(edits, textEdit{start: firstStart, end: firstEnd, replacement: []byte(params.genericFn)})
	for i := len(group) - 1; i >= 1; i-- {
		edits = append(edits, deleteFuncEdit(fset, group[i], src))
	}
	return edits
}

// deleteFuncEdit creates an edit to delete a function.
func deleteFuncEdit(fset *token.FileSet, fn *ast.FuncDecl, src []byte) textEdit {
	start := fset.Position(fn.Pos()).Offset
	end := fset.Position(fn.End()).Offset
	for end < len(src) && (src[end] == '\n' || src[end] == '\r') {
		end++
		break
	}
	return textEdit{start: start, end: end, replacement: []byte("")}
}

// commonPrefix finds the longest common prefix of function names,
// stripped of trailing type suffixes.
func commonFuncPrefix(funcs []*ast.FuncDecl) string {
	names := funcNames(funcs)
	if len(names) == 0 {
		return ""
	}
	return longestPrefix(names)
}

// funcNames extracts function names.
func funcNames(funcs []*ast.FuncDecl) []string {
	names := make([]string, len(funcs))
	for i, fn := range funcs {
		names[i] = fn.Name.Name
	}
	return names
}

// longestPrefix finds the longest common prefix.
func longestPrefix(names []string) string {
	prefix := names[0]
	for _, n := range names[1:] {
		prefix = shrinkPrefix(prefix, n)
	}
	return prefix
}

// shrinkPrefix shrinks prefix until n has it as prefix.
func shrinkPrefix(prefix, n string) string {
	for !strings.HasPrefix(n, prefix) && prefix != "" {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix
}

// collectTypeVars collects type variable mappings.
// Returns map from concrete type -> "T" placeholder.
func collectTypeVars(fset *token.FileSet, group []*ast.FuncDecl, src []byte) map[string]string {
	types := collectDistinctTypes(fset, group, src)
	if len(types) < 2 {
		return nil
	}
	out := make(map[string]string)
	for t := range types {
		out[t] = "T"
	}
	return out
}

// collectDistinctTypes gathers distinct type names from func signatures.
func collectDistinctTypes(fset *token.FileSet, group []*ast.FuncDecl, src []byte) map[string]bool {
	types := make(map[string]bool)
	for _, fn := range group {
		addFieldTypes(fset, fn.Type.Params, src, types)
		addFieldTypes(fset, fn.Type.Results, src, types)
	}
	return types
}

// addFieldTypes adds type names from a field list to the set.
func addFieldTypes(fset *token.FileSet, fl *ast.FieldList, src []byte, types map[string]bool) {
	if fl == nil {
		return
	}
	for _, p := range fl.List {
		types[srcText(fset, p.Type, src)] = true
	}
}

// inferConstraint infers the type constraint from function body usage.
// If the body uses comparison operators, use cmp.Ordered.
// If it uses arithmetic, use a numeric constraint.
func inferConstraint(fset *token.FileSet, fn *ast.FuncDecl, src []byte) string {
	bodyText := srcText(fset, fn.Body, src)
	if usesComparison(bodyText) {
		return "cmp.Ordered"
	}
	return "any"
}

// usesComparison reports whether the body uses comparison operators.
func usesComparison(bodyText string) bool {
	ops := []string{">", "<", "==", "!="}
	for _, op := range ops {
		if strings.Contains(bodyText, op) {
			return true
		}
	}
	return false
}

// buildGenericFunc constructs the generic function source.
func buildGenericFunc(fset *token.FileSet, template *ast.FuncDecl, src []byte, params *generalizeParams) string {
	// Get the template source.
	fnText := srcText(fset, template, src)
	// Replace type names with T.
	for concrete, placeholder := range params.typeVars {
		fnText = strings.ReplaceAll(fnText, concrete, placeholder)
	}
	// Replace function name with base name and add type params.
	// Find "func Name(" and replace with "func baseName[T constraint](".
	oldDecl := "func " + template.Name.Name + "("
	newDecl := fmt.Sprintf("func %s[T %s](", params.baseName, params.constraint)
	fnText = strings.Replace(fnText, oldDecl, newDecl, 1)
	return fnText
}

// ensureImport adds an import if not already present.
func ensureImport(src []byte, pkg string) []byte {
	if bytes.Contains(src, []byte(`"`+pkg+`"`)) {
		return src
	}
	// Find the import block and add to it, or create one.
	// Simplified: look for "import (" and add after it.
	importDecl := []byte("import (")
	idx := bytes.Index(src, importDecl)
	if idx >= 0 {
		insertAt := idx + len(importDecl)
		newSrc := make([]byte, 0, len(src)+len(pkg)+10)
		newSrc = append(newSrc, src[:insertAt]...)
		newSrc = append(newSrc, []byte(fmt.Sprintf("\n\t\"%s\"", pkg))...)
		newSrc = append(newSrc, src[insertAt:]...)
		return newSrc
	}
	// No import block: add after package clause.
	lines := bytes.SplitN(src, []byte("\n"), 2)
	if len(lines) == 2 {
		out := make([]byte, 0, len(src)+50)
		out = append(out, lines[0]...)
		out = append(out, []byte(fmt.Sprintf("\n\nimport \"%s\"\n", pkg))...)
		out = append(out, lines[1]...)
		return out
	}
	return src
}

// findEnclosingFunc finds the function declaration containing a node.
func findEnclosingFunc(f *ast.File, target ast.Node) *ast.FuncDecl {
	var result *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			// Check if target is within fn.
			if fn.Pos() <= target.Pos() && target.End() <= fn.End() {
				result = fn
				return false
			}
		}
		return true
	})
	return result
}

// findSwitchAtLine locates the switch statement at the given line.
func findSwitchAtLine(fset *token.FileSet, f *ast.File, line int) *ast.SwitchStmt {
	var target *ast.SwitchStmt
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		if fset.Position(sw.Pos()).Line == line {
			target = sw
			return false
		}
		return true
	})
	return target
}

// srcText extracts source text for a node.
func srcText(fset *token.FileSet, n ast.Node, src []byte) string {
	start := fset.Position(n.Pos()).Offset
	end := fset.Position(n.End()).Offset
	if start < 0 || end > len(src) || start >= end {
		return ""
	}
	return string(src[start:end])
}
