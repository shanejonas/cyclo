package gopatterns

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// applyGuardFix applies a guard_clause FixSpec: invert the if statement at
// the spec's line into a guard clause.
func applyGuardFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	ifLine, err := guardIfLine(spec)
	if err != nil {
		return nil, err
	}
	target := findIfAtLine(fset, f, ifLine)
	if target == nil {
		return nil, fmt.Errorf("guard_clause: no if statement at line %d", ifLine)
	}
	edit, _, ok := guardEdit(fset, target, src)
	if !ok {
		return nil, fmt.Errorf("guard_clause: cannot fix if at line %d", ifLine)
	}
	out := applyEdits(src, []textEdit{edit})
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("gofmt after guard fix: %w", err)
	}
	return formatted, nil
}

// guardIfLine extracts the if_line param from a FixSpec.
func guardIfLine(spec *patterns.FixSpec) (int, error) {
	ifLine, err := strconv.Atoi(spec.Params["if_line"])
	if err != nil {
		return 0, fmt.Errorf("guard_clause FixSpec missing if_line: %w", err)
	}
	return ifLine, nil
}

// findIfAtLine locates the if statement at the given line.
func findIfAtLine(fset *token.FileSet, f *ast.File, line int) *ast.IfStmt {
	var target *ast.IfStmt
	ast.Inspect(f, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if fset.Position(ifStmt.Pos()).Line == line {
			target = ifStmt
			return false
		}
		return true
	})
	return target
}

// applyValueObjectFix applies a value_object FixSpec: extract the param
// group into a struct.
func applyValueObjectFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	// The value fixer re-detects within the file but scoped by the spec's
	// group param. For now, delegate to the existing fixer which finds
	// the clump matching the spec's group.
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	group := spec.Params["group"]
	if group == "" {
		return nil, fmt.Errorf("value_object FixSpec missing group")
	}
	// Use the existing fixer; it will find and fix the matching clump.
	out, _, err := FixValueObjects(fset, f, src)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// applyParameterizeFix applies a parameterize FixSpec.
func applyParameterizeFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	// Delegate to existing fixer; it finds parameterizable pairs.
	out, _, err := FixParameterize(fset, f, src, nil)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// applyInterfaceFix generates a Go interface for trait_method and
// capability_set candidates. The fix emits the interface definition;
// the types already implement it implicitly.
func applyInterfaceFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	if len(spec.Definitions) == 0 {
		return nil, fmt.Errorf("interface fixer: no definitions in FixSpec")
	}
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	methods := collectMethodSigs(fset, f, spec.Definitions, src)
	if len(methods) == 0 {
		return nil, fmt.Errorf("interface fixer: no methods found")
	}
	ifaceName := interfaceNameFor(spec.Kind)
	if declaresType(f, ifaceName) {
		// Idempotent: a previous fix already generated this interface.
		// Returning src unchanged keeps re-runs (and --phased re-mining)
		// from redeclaring it.
		return src, nil
	}
	out := insertAfterImports(fset, f, src, buildInterface(ifaceName, spec.Kind, methods))
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("interface fixer: format generated code: %w", err)
	}
	return formatted, nil
}

// declaresType reports whether the file already declares a type with the
// given name.
func declaresType(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSpec); ok && ts.Name.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// collectMethodSigs extracts unique method signatures from definitions.
func collectMethodSigs(fset *token.FileSet, f *ast.File, defs []string, src []byte) []string {
	var methods []string
	seen := make(map[string]bool)
	for _, def := range defs {
		sig := methodSigFromDef(fset, f, def, src, seen)
		if sig != "" {
			methods = append(methods, sig)
		}
	}
	return methods
}

// methodSigFromDef parses a "path:line:name" definition and returns the
// method signature, or "" if it can't be found or was already seen.
// Definitions carry qualified names ("pkg.Type.method"); the AST lookup
// needs the bare method name, so the qualifier is stripped.
func methodSigFromDef(fset *token.FileSet, f *ast.File, def string, src []byte, seen map[string]bool) string {
	parts := strings.SplitN(def, ":", 3)
	if len(parts) != 3 {
		return ""
	}
	line, err := strconv.Atoi(parts[1])
	if err != nil {
		return ""
	}
	name := parts[2]
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if seen[name] {
		return ""
	}
	seen[name] = true
	return findMethodSig(fset, f, line, name, src)
}

// interfaceNameFor returns the generated interface name for a kind.
func interfaceNameFor(kind patterns.CandidateKind) string {
	if kind == patterns.CapabilitySet {
		return "Capabilities"
	}
	return "Trait"
}

// buildInterface renders the interface definition.
func buildInterface(name string, kind patterns.CandidateKind, methods []string) []byte {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("\n// %s is generated by cyclo fix from a %s candidate.\n", name, kind))
	buf.WriteString(fmt.Sprintf("type %s interface {\n", name))
	for _, m := range methods {
		buf.WriteString("\t" + m + "\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes()
}

// findMethodSig finds the method at line and returns its signature
// as "Name(params) results".
func findMethodSig(fset *token.FileSet, f *ast.File, line int, name string, src []byte) string {
	fn := findFuncDecl(fset, f, line, name)
	if fn == nil {
		return ""
	}
	var buf bytes.Buffer
	buf.WriteString(name)
	buf.WriteString("(")
	buf.WriteString(renderParams(fset, fn.Type.Params, src))
	buf.WriteString(")")
	if fn.Type.Results != nil {
		buf.WriteString(" ")
		buf.WriteString(renderResults(fset, fn.Type.Results, src))
	}
	return buf.String()
}

// findFuncDecl locates the FuncDecl with the given name at the given line.
func findFuncDecl(fset *token.FileSet, f *ast.File, line int, name string) *ast.FuncDecl {
	var found *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name {
			return true
		}
		if fset.Position(fn.Pos()).Line != line {
			return true
		}
		found = fn
		return false
	})
	return found
}

// renderParams renders a FieldList of params as "a int, b string".
func renderParams(fset *token.FileSet, params *ast.FieldList, src []byte) string {
	if params == nil {
		return ""
	}
	var buf bytes.Buffer
	for i, p := range params.List {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(renderFieldNames(p.Names))
		if len(p.Names) > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(exprToString(fset, p.Type, src))
	}
	return buf.String()
}

// renderFieldNames joins field names with ", ".
func renderFieldNames(names []*ast.Ident) string {
	var buf bytes.Buffer
	for j, n := range names {
		if j > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(n.Name)
	}
	return buf.String()
}

// renderResults renders a FieldList of results as "int" or "(int, error)".
func renderResults(fset *token.FileSet, results *ast.FieldList, src []byte) string {
	if len(results.List) == 1 && len(results.List[0].Names) == 0 {
		return exprToString(fset, results.List[0].Type, src)
	}
	var buf bytes.Buffer
	buf.WriteString("(")
	for i, r := range results.List {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(exprToString(fset, r.Type, src))
	}
	buf.WriteString(")")
	return buf.String()
}

// exprToString renders an AST expression as source text.
func exprToString(fset *token.FileSet, e ast.Expr, src []byte) string {
	start := fset.Position(e.Pos()).Offset
	end := fset.Position(e.End()).Offset
	if start < 0 || end > len(src) || start >= end {
		return ""
	}
	return string(src[start:end])
}

// insertAfterImports inserts text after the package clause and imports.
// fset is needed to convert token.Pos to byte offsets.
func insertAfterImports(fset *token.FileSet, f *ast.File, src []byte, text []byte) []byte {
	offset := importEndOffset(fset, f, src)
	out := make([]byte, 0, len(src)+len(text))
	out = append(out, src[:offset]...)
	out = append(out, text...)
	out = append(out, src[offset:]...)
	return out
}

// importEndOffset returns the byte offset after the last import,
// or after the package clause if there are no imports.
func importEndOffset(fset *token.FileSet, f *ast.File, src []byte) int {
	if pos := lastImportEnd(f, f.Imports); pos != 0 {
		return fset.Position(pos).Offset
	}
	return packageEndOffset(fset, f, src)
}

// lastImportEnd finds the end position of the GenDecl containing the last import.
func lastImportEnd(f *ast.File, imports []*ast.ImportSpec) token.Pos {
	if len(imports) == 0 {
		return 0
	}
	last := imports[len(imports)-1]
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if genContainsImport(gen, last) {
			return gen.End()
		}
	}
	return 0
}

// genContainsImport reports whether gen declares the given import.
func genContainsImport(gen *ast.GenDecl, target *ast.ImportSpec) bool {
	for _, spec := range gen.Specs {
		if imp, ok := spec.(*ast.ImportSpec); ok && imp == target {
			return true
		}
	}
	return false
}

// packageEndOffset returns the offset after the package clause newline.
func packageEndOffset(fset *token.FileSet, f *ast.File, src []byte) int {
	offset := fset.Position(f.Name.End()).Offset
	for offset < len(src) && src[offset] != '\n' {
		offset++
	}
	return offset + 1
}



// applyAnemicModelFix applies an anemic_model FixSpec.
func applyAnemicModelFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	out, _, err := FixAnemicModels(fset, f, src)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// applyPrimitiveObsessionFix applies a primitive_obsession FixSpec.
func applyPrimitiveObsessionFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	out, _, err := FixPrimitiveObsession(fset, f, src)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// applyEntityIdentityFix applies an entity_identity FixSpec.
// Two modes:
// - Standard: replaces attribute-based equality (a.Name == b.Name &&
//   a.Email == b.Email) with identity comparison (a.ID == b.ID).
// - Add-ID (paired with missing_identity): adds an ID field to a struct
//   that is compared by attributes but has no identity. After re-mining,
//   the standard rewrite fires.
func applyEntityIdentityFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	if spec.Params["add_id"] == "true" {
		return applyEntityAddIDFix(spec, src)
	}
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	params, err := parseEntityFixParams(spec)
	if err != nil {
		return nil, err
	}
	target := findComparisonAtLine(fset, f, params.line)
	if target == nil {
		return nil, fmt.Errorf("entity_identity: no comparison at line %d", params.line)
	}
	replacement := []byte(fmt.Sprintf("%s.%s == %s.%s", params.left, params.idField, params.right, params.idField))
	start := fset.Position(target.Pos()).Offset
	end := fset.Position(target.End()).Offset
	edit := textEdit{start: start, end: end, replacement: replacement}
	out := applyEdits(src, []textEdit{edit})
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("gofmt after entity_identity fix: %w", err)
	}
	return formatted, nil
}

// applyEntityAddIDFix adds an ID field to the struct. This is the first
// step of the paired fix: missing_identity (detection) flags the problem,
// entity_identity adds the ID, then a later phase rewrites the comparison.
func applyEntityAddIDFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	typeName := spec.Params["type"]
	if typeName == "" {
		return nil, fmt.Errorf("entity_identity FixSpec missing type for add_id")
	}
	insertPos, err := structInsertPos(fset, f, typeName)
	if err != nil {
		return nil, err
	}
	edit := textEdit{
		start:       insertPos,
		end:         insertPos,
		replacement: []byte("\n\tID string `json:\"id\"`"),
	}
	out := applyEdits(src, []textEdit{edit})
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("gofmt after entity_identity add_id fix: %w", err)
	}
	return formatted, nil
}

// entityFixParams extracts and validates entity_identity FixSpec params.
type entityFixParams struct {
	line    int
	idField string
	left    string
	right   string
}

func parseEntityFixParams(spec *patterns.FixSpec) (entityFixParams, error) {
	line, err := strconv.Atoi(spec.Params["line"])
	if err != nil {
		return entityFixParams{}, fmt.Errorf("entity_identity FixSpec missing line: %w", err)
	}
	idField := spec.Params["id_field"]
	left := spec.Params["left"]
	right := spec.Params["right"]
	if idField == "" || left == "" || right == "" {
		return entityFixParams{}, fmt.Errorf("entity_identity FixSpec missing params")
	}
	return entityFixParams{line: line, idField: idField, left: left, right: right}, nil
}

// findComparisonAtLine locates a boolean comparison expression at the given line.
func findComparisonAtLine(fset *token.FileSet, f *ast.File, line int) ast.Expr {
	var target ast.Expr
	ast.Inspect(f, func(n ast.Node) bool {
		var expr ast.Expr
		switch node := n.(type) {
		case *ast.IfStmt:
			expr = node.Cond
		case *ast.ReturnStmt:
			if len(node.Results) == 1 {
				expr = node.Results[0]
			}
		}
		if expr == nil {
			return true
		}
		if fset.Position(expr.Pos()).Line == line {
			target = expr
			return false
		}
		return true
	})
	return target
}


// structInsertPos finds the byte offset after the last field of a struct.
func structInsertPos(fset *token.FileSet, f *ast.File, typeName string) (int, error) {
	target := findStructDecl(f, typeName)
	if target == nil {
		return 0, fmt.Errorf("missing_identity: no struct %q found", typeName)
	}
	st, ok := target.Type.(*ast.StructType)
	if !ok {
		return 0, fmt.Errorf("missing_identity: %q is not a struct", typeName)
	}
	if st.Fields == nil || len(st.Fields.List) == 0 {
		return 0, fmt.Errorf("missing_identity: struct %q has no fields", typeName)
	}
	lastField := st.Fields.List[len(st.Fields.List)-1]
	return fset.Position(lastField.End()).Offset, nil
}

// findStructDecl locates the type declaration for the named struct.
func findStructDecl(f *ast.File, typeName string) *ast.TypeSpec {
	var target *ast.TypeSpec
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		if ts.Name.Name == typeName {
			target = ts
			return false
		}
		return true
	})
	return target
}

// applyFactoryFix applies a factory FixSpec: extracts a NewT factory
// function for a struct type built with 5+ fields in multiple places,
// and rewrites full composite literals to factory calls.
func applyFactoryFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	typeName := spec.Params["type"]
	if typeName == "" {
		return nil, fmt.Errorf("factory: FixSpec missing type")
	}
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	ctx, ok := newFactoryContext(fset, f, src, typeName)
	if !ok {
		return src, nil
	}
	edits := factoryEdits(ctx)
	out := applyEdits(src, edits)
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("gofmt after factory fix: %w", err)
	}
	return formatted, nil
}

// factoryContext carries everything the factory fix needs: the struct
// fields, the literal rewrites, and where to insert the factory.
type factoryContext struct {
	fset        *token.FileSet
	src         []byte
	typeName    string
	factoryName string
	fields      []factoryField
	rewrites    []textEdit
	typeEnd     int
}

// newFactoryContext builds the fix context, or false when the fix should
// be skipped.
func newFactoryContext(fset *token.FileSet, f *ast.File, src []byte, typeName string) (factoryContext, bool) {
	fields, typeEnd, ok := factoryStructFields(fset, f, src, typeName)
	if !ok {
		return factoryContext{}, false
	}
	factoryName := "New" + typeName
	if hasFuncDecl(f, factoryName) {
		return factoryContext{}, false
	}
	rewrites := factoryLitRewrites(fset, f, src, typeName, factoryName, fields)
	if len(rewrites) == 0 {
		return factoryContext{}, false
	}
	return factoryContext{
		fset: fset, src: src, typeName: typeName,
		factoryName: factoryName, fields: fields,
		rewrites: rewrites, typeEnd: typeEnd,
	}, true
}

// litRewriteCtx carries the parameters for literal rewrite helpers.
type litRewriteCtx struct {
	fset        *token.FileSet
	src         []byte
	typeName    string
	factoryName string
	fields      []factoryField
}

// factoryEdits combines the literal rewrites with the factory insertion.
func factoryEdits(ctx factoryContext) []textEdit {
	edits := make([]textEdit, 0, len(ctx.rewrites)+1)
	edits = append(edits, ctx.rewrites...)
	edits = append(edits, textEdit{
		start:       ctx.typeEnd,
		end:         ctx.typeEnd,
		replacement: []byte("\n" + factoryFuncText(ctx.factoryName, ctx.typeName, ctx.fields)),
	})
	return edits
}

// factoryField is one struct field in declaration order.
type factoryField struct {
	name     string
	typeText string
}

// factoryStructFields returns the struct's fields in declaration order and
// the end offset of the type declaration. It skips generic structs,
// embedded fields, and multi-name field lines.
func factoryStructFields(fset *token.FileSet, f *ast.File, src []byte, typeName string) ([]factoryField, int, bool) {
	for _, decl := range f.Decls {
		if fields, end, ok := typeDeclFields(fset, decl, src, typeName); ok {
			return fields, end, true
		}
	}
	return nil, 0, false
}

// typeDeclFields extracts struct fields from a type declaration if it
// declares typeName as a non-generic struct with named fields.
func typeDeclFields(fset *token.FileSet, decl ast.Decl, src []byte, typeName string) ([]factoryField, int, bool) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || gen.Tok != token.TYPE {
		return nil, 0, false
	}
	for _, s := range gen.Specs {
		if fields, end, ok := typeSpecFields(fset, gen, s, src, typeName); ok {
			return fields, end, true
		}
	}
	return nil, 0, false
}

// typeSpecFields extracts fields from one type spec.
func typeSpecFields(fset *token.FileSet, gen *ast.GenDecl, s ast.Spec, src []byte, typeName string) ([]factoryField, int, bool) {
	ts, ok := matchingTypeSpec(s, typeName)
	if !ok {
		return nil, 0, false
	}
	fields := structFieldList(fset, src, ts)
	if len(fields) == 0 {
		return nil, 0, false
	}
	return fields, fset.Position(gen.End()).Offset, true
}

// matchingTypeSpec returns the type spec if it declares typeName as a
// non-generic struct.
func matchingTypeSpec(s ast.Spec, typeName string) (*ast.StructType, bool) {
	ts, ok := s.(*ast.TypeSpec)
	if !ok || ts.Name.Name != typeName || ts.TypeParams != nil {
		return nil, false
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil, false
	}
	return st, true
}

// structFieldList converts struct field AST to factoryFields, skipping
// embedded or multi-name fields.
func structFieldList(fset *token.FileSet, src []byte, st *ast.StructType) []factoryField {
	var fields []factoryField
	for _, fld := range st.Fields.List {
		if len(fld.Names) != 1 {
			return nil
		}
		start := fset.Position(fld.Type.Pos()).Offset
		end := fset.Position(fld.Type.End()).Offset
		fields = append(fields, factoryField{
			name:     fld.Names[0].Name,
			typeText: string(src[start:end]),
		})
	}
	return fields
}

// hasFuncDecl reports whether f declares a function with the given name.
func hasFuncDecl(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok && fd.Name.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// factoryLitRewrites finds composite literals of typeName that set every
// field and returns text edits rewriting them to factory calls.
func factoryLitRewrites(fset *token.FileSet, f *ast.File, src []byte, typeName, factoryName string, fields []factoryField) []textEdit {
	rc := litRewriteCtx{fset: fset, src: src, typeName: typeName, factoryName: factoryName, fields: fields}
	var edits []textEdit
	ast.Inspect(f, func(n ast.Node) bool {
		if edit, ok, skip := unaryLitEdit(rc, n); ok || skip {
			if ok {
				edits = append(edits, edit)
			}
			return !skip
		}
		if edit, ok := plainLitEdit(rc, n); ok {
			edits = append(edits, edit)
		}
		return true
	})
	return edits
}

// unaryLitEdit rewrites &T{...} to &NewT(...). skip is true when n was a
// &T{...} unary (handled or not) so the walker does not descend into it.
func unaryLitEdit(rc litRewriteCtx, n ast.Node) (textEdit, bool, bool) {
	ue, ok := n.(*ast.UnaryExpr)
	if !ok || ue.Op != token.AND {
		return textEdit{}, false, false
	}
	lit, ok := ue.X.(*ast.CompositeLit)
	if !ok || !isTargetLit(lit, rc.typeName) {
		return textEdit{}, false, false
	}
	args, ok := litArgs(rc.fset, rc.src, lit, rc.fields)
	if !ok {
		return textEdit{}, false, true
	}
	return textEdit{
		start:       rc.fset.Position(ue.Pos()).Offset,
		end:         rc.fset.Position(ue.End()).Offset,
		replacement: []byte("&" + rc.factoryName + "(" + args + ")"),
	}, true, true
}

// plainLitEdit rewrites T{...} to NewT(...).
func plainLitEdit(rc litRewriteCtx, n ast.Node) (textEdit, bool) {
	lit, ok := n.(*ast.CompositeLit)
	if !ok || !isTargetLit(lit, rc.typeName) {
		return textEdit{}, false
	}
	args, ok := litArgs(rc.fset, rc.src, lit, rc.fields)
	if !ok {
		return textEdit{}, false
	}
	return textEdit{
		start:       rc.fset.Position(lit.Pos()).Offset,
		end:         rc.fset.Position(lit.End()).Offset,
		replacement: []byte(rc.factoryName + "(" + args + ")"),
	}, true
}

// isTargetLit reports whether lit is a composite literal of typeName.
// Only unqualified identifiers match: the type is declared in this file.
func isTargetLit(lit *ast.CompositeLit, typeName string) bool {
	id, ok := lit.Type.(*ast.Ident)
	return ok && id.Name == typeName
}

// litArgs builds the factory call arguments in field-declaration order.
// It returns false unless the literal sets every field.
func litArgs(fset *token.FileSet, src []byte, lit *ast.CompositeLit, fields []factoryField) (string, bool) {
	if isKeyedLit(lit) {
		return keyedLitArgs(fset, src, lit, fields)
	}
	if len(lit.Elts) != len(fields) {
		return "", false
	}
	parts := make([]string, 0, len(lit.Elts))
	for _, e := range lit.Elts {
		parts = append(parts, srcText(fset, e, src))
	}
	return strings.Join(parts, ", "), true
}

// isKeyedLit reports whether the literal uses field: value form.
func isKeyedLit(lit *ast.CompositeLit) bool {
	for _, e := range lit.Elts {
		if _, ok := e.(*ast.KeyValueExpr); ok {
			return true
		}
	}
	return false
}

// keyedLitArgs builds args from a keyed literal; all fields must be set.
func keyedLitArgs(fset *token.FileSet, src []byte, lit *ast.CompositeLit, fields []factoryField) (string, bool) {
	vals := map[string]string{}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return "", false
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return "", false
		}
		vals[key.Name] = srcText(fset, kv.Value, src)
	}
	parts := make([]string, 0, len(fields))
	for _, fld := range fields {
		v, ok := vals[fld.name]
		if !ok {
			return "", false
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, ", "), true
}

// factoryFuncText builds the factory function source.
func factoryFuncText(factoryName, typeName string, fields []factoryField) string {
	params := make([]string, 0, len(fields))
	assigns := make([]string, 0, len(fields))
	used := map[string]bool{}
	for _, fld := range fields {
		pn := paramName(fld.name, used)
		used[pn] = true
		params = append(params, pn+" "+fld.typeText)
		assigns = append(assigns, fld.name+": "+pn)
	}
	return fmt.Sprintf("func %s(%s) %s {\n\treturn %s{%s}\n}\n",
		factoryName, strings.Join(params, ", "), typeName,
		typeName, strings.Join(assigns, ", "))
}

// paramName converts a field name to a parameter name, avoiding keywords
// and collisions.
func paramName(field string, used map[string]bool) string {
	base := lowerFirst(field)
	if isGoKeyword(base) {
		base += "Arg"
	}
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	return name
}

// isGoKeyword reports whether s is a Go keyword.
func isGoKeyword(s string) bool {
	switch s {
	case "break", "case", "chan", "const", "continue", "default",
		"defer", "else", "fallthrough", "for", "func", "go", "goto",
		"if", "import", "interface", "map", "package", "range",
		"return", "select", "struct", "switch", "type", "var":
		return true
	}
	return false
}

// applySpecificationFix applies a Specification FixSpec: extracts a repeated
// boolean business rule into an idiomatic Go predicate — a method on the type
// when it's declared locally, a plain function otherwise — and rewrites
// matching if conditions to predicate calls.
func applySpecificationFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	typeName, ruleKey, ok := specParams(spec)
	if !ok {
		return nil, fmt.Errorf("specification: FixSpec missing type or rulekey")
	}
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	matches := findSpecMatches(f, ruleKey)
	if len(matches) == 0 {
		return src, nil
	}
	predName, isLocal, ok := specFixPlan(f, typeName, matches)
	if !ok {
		return src, nil
	}
	ctx := specEditCtx{fset: fset, f: f, src: src, predName: predName, typeName: typeName, isLocal: isLocal}
	edits := predEdits(ctx, matches)
	out := applyEdits(src, edits)
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("gofmt after specification fix: %w", err)
	}
	return formatted, nil
}

// specFixPlan derives the predicate name and determines whether to generate
// a method (local type) or function (external type). Returns false when the
// fix cannot be generated safely.
func specFixPlan(f *ast.File, typeName string, matches []specMatch) (predName string, isLocal, ok bool) {
	// Derive the predicate name from the first match's condition.
	predName = predicateName(matches[0].stmt.Cond)
	if predName == "" {
		return "", false, false
	}
	predName = uniquePredName(f, predName)
	// Method on local type, function for external types.
	// If the type isn't local and has no package qualifier, we can't
	// generate a correct signature — skip rather than emit broken code.
	// Strip pointer for the local-type check: `*User` is local if `User` is.
	isLocal = hasTypeDecl(f, strings.TrimPrefix(baseTypeName(typeName), "*"))
	if !isLocal && !strings.Contains(typeName, ".") {
		return "", false, false
	}
	return predName, isLocal, true
}

// baseTypeName strips a package qualifier: "apidomain.AgentArgs" -> "AgentArgs".
// Preserves a leading "*" for pointer types: "*http.Response" -> "*Response".
func baseTypeName(typeName string) string {
	star := ""
	if strings.HasPrefix(typeName, "*") {
		star = "*"
		typeName = typeName[1:]
	}
	if i := strings.LastIndex(typeName, "."); i >= 0 {
		return star + typeName[i+1:]
	}
	return star + typeName
}

// uniquePredName ensures the predicate name doesn't collide with an existing
// declaration in the file.
func uniquePredName(f *ast.File, name string) string {
	if !hasFuncDecl(f, name) && !hasTypeDecl(f, name) {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", name, i)
		if !hasFuncDecl(f, candidate) && !hasTypeDecl(f, candidate) {
			return candidate
		}
	}
}

// predicateName derives an idiomatic predicate name from a boolean condition:
// `u.Age > 18 && u.Active` -> `IsAgeOver18AndActive`.
func predicateName(cond ast.Expr) string {
	operands, connector := splitBoolOps(cond)
	if len(operands) == 0 {
		return ""
	}
	var parts []string
	for _, op := range operands {
		part := operandPredName(op)
		if part == "" {
			return ""
		}
		parts = append(parts, part)
	}
	sep := "And"
	if connector == token.LOR {
		sep = "Or"
	}
	return "Is" + strings.Join(parts, sep)
}

// splitBoolOps splits a boolean condition into operands and returns the
// connecting operator (LAND or LOR).
func splitBoolOps(cond ast.Expr) ([]ast.Expr, token.Token) {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.LAND && bin.Op != token.LOR) {
		return []ast.Expr{cond}, token.ILLEGAL
	}
	left, _ := splitBoolOps(bin.X)
	right, _ := splitBoolOps(bin.Y)
	return append(left, right...), bin.Op
}

// operandPredName converts one boolean operand to a name fragment:
// `u.Age > 18` -> `AgeOver18`, `u.Active` -> `Active`, `!u.Active` -> `NotActive`.
func operandPredName(op ast.Expr) string {
	op = specUnwrapParens(op)
	if name, ok := negatedPredName(op); ok {
		return name
	}
	// Bare selector: u.Active -> Active
	if field, _, ok := specBareSelector(op); ok {
		return capitalize(field)
	}
	return comparisonPredName(op)
}

// negatedPredName handles `!u.Active` -> `NotActive`.
func negatedPredName(op ast.Expr) (string, bool) {
	unary, ok := op.(*ast.UnaryExpr)
	if !ok || unary.Op != token.NOT {
		return "", false
	}
	inner := operandPredName(unary.X)
	if inner == "" {
		return "", false
	}
	return "Not" + inner, true
}

// comparisonPredName handles `u.Age > 18` -> `AgeOver18`.
func comparisonPredName(op ast.Expr) string {
	field, opStr, base, value := specOperandParts(op, nil)
	if field == "" {
		return ""
	}
	_ = base
	if name, ok := emptyNilName(field, opStr, value); ok {
		return name
	}
	return capitalize(field) + cmpWord(opStr) + sanitizeLiteral(value)
}

// emptyNilName returns a natural name for empty/nil comparisons:
// `s == ""` -> `SEmpty`, `s != ""` -> `SNotEmpty`, `p == nil` -> `PNil`.
func emptyNilName(field, opStr, value string) (string, bool) {
	suffixes := map[string]map[string]string{
		`""`:  {"==": "Empty", "!=": "NotEmpty"},
		"nil": {"==": "Nil", "!=": "NotNil"},
	}
	if ops, ok := suffixes[value]; ok {
		if suffix, ok := ops[opStr]; ok {
			return capitalize(field) + suffix, true
		}
	}
	return "", false
}

// cmpWord maps a comparison operator to a name fragment.
func cmpWord(op string) string {
	words := map[string]string{
		">":  "Over",
		">=": "AtLeast",
		"<":  "Under",
		"<=": "AtMost",
		"==": "Is",
		"!=": "IsNot",
	}
	if w, ok := words[op]; ok {
		return w
	}
	return "Cmp"
}

// sanitizeLiteral converts a literal value to a name-safe fragment:
// "18" -> "18", `"foo"` -> "Foo", "0.60" -> "060", "nil" -> "Nil".
func sanitizeLiteral(value string) string {
	if value == "" || value == "nil" {
		return capitalize(value)
	}
	return capitalize(stripNonAlnum(value))
}

// stripNonAlnum removes quotes and non-alphanumeric characters.
func stripNonAlnum(value string) string {
	var out strings.Builder
	for _, r := range value {
		if isAlnum(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// isAlnum reports whether r is an ASCII letter or digit.
func isAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// specParams extracts the type name and rule key from a FixSpec.
func specParams(spec *patterns.FixSpec) (typeName, ruleKey string, ok bool) {
	typeName = spec.Params["type"]
	ruleKey = spec.Params["rulekey"]
	return typeName, ruleKey, typeName != "" && ruleKey != ""
}

// findSpecMatches returns the if statements whose condition matches the rule.
func findSpecMatches(f *ast.File, ruleKey string) []specMatch {
	var matches []specMatch
	ast.Inspect(f, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if !isMultiOperand(ifStmt.Cond) {
			return true
		}
		key, varName := specMatchKey(ifStmt.Cond)
		if !specKeysMatch(key, ruleKey) {
			return true
		}
		matches = append(matches, specMatch{stmt: ifStmt, varName: varName})
		return true
	})
	return matches
}

// isMultiOperand reports whether the expression has 2+ boolean operands.
func isMultiOperand(cond ast.Expr) bool {
	return len(flattenBoolOps(cond)) >= specMinOperands
}

// specMatchKey computes the match key for an if condition, falling back
// to structural matching when type info is unavailable.
func specMatchKey(cond ast.Expr) (key, varName string) {
	key, varName, _ = specRuleKey(cond, nil)
	if key == "" {
		key = specStructKey(cond)
	}
	return key, varName
}

// specMatch is one if statement matching a business rule.
type specMatch struct {
	stmt    *ast.IfStmt
	varName string
}

// hasTypeDecl reports whether the file declares a type with the given name.
func hasTypeDecl(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if ok && ts.Name.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// specStructKey builds a type-agnostic structural key for matching.
// Used when types.Info is unavailable in the fixer.
func specStructKey(cond ast.Expr) string {
	operands := flattenBoolOps(cond)
	var parts []string
	for _, op := range operands {
		field, opStr, base, value := specOperandParts(op, nil)
		if field == "" {
			return ""
		}
		// Normalize the base variable to "v" for cross-site matching.
		// Include the value: different literals are different rules.
		parts = append(parts, "v."+field+":"+opStr+":"+value)
		_ = base
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// specKeysMatch reports whether two rule keys describe the same rule,
// ignoring the type prefix and variable names.
func specKeysMatch(a, b string) bool {
	aParts := strings.Split(a, "|")
	bParts := strings.Split(b, "|")
	aKey := aParts[len(aParts)-1]
	bKey := bParts[len(bParts)-1]
	// Normalize variable prefixes: "user.Age" -> "v.Age".
	normalize := func(s string) string {
		fields := strings.Split(s, ",")
		for i, f := range fields {
			if idx := strings.Index(f, "."); idx > 0 {
				fields[i] = "v" + f[idx:]
			}
		}
		sort.Strings(fields)
		return strings.Join(fields, ",")
	}
	return normalize(aKey) == normalize(bKey)
}

// specEditCtx bundles the shared context for building specification edits.
type specEditCtx struct {
	fset     *token.FileSet
	f        *ast.File
	src      []byte
	predName string
	typeName string
	isLocal  bool
}

// specEdits builds the text edits: insert the Specification type, rewrite
// matching conditions to IsSatisfiedBy calls.
// predEdits builds the text edits: rewrite matching conditions to predicate
// calls and insert the predicate declaration.
func predEdits(ctx specEditCtx, matches []specMatch) []textEdit {
	var edits []textEdit
	// Rewrite each matching if condition.
	for _, m := range matches {
		condStart := ctx.fset.Position(m.stmt.Cond.Pos()).Offset
		condEnd := ctx.fset.Position(m.stmt.Cond.End()).Offset
		var replacement string
		if ctx.isLocal {
			replacement = fmt.Sprintf("%s.%s()", m.varName, ctx.predName)
		} else {
			replacement = fmt.Sprintf("%s(%s)", unexported(ctx.predName), m.varName)
		}
		edits = append(edits, textEdit{start: condStart, end: condEnd, replacement: []byte(replacement)})
	}
	edits = append(edits, predDeclEdit(ctx, matches[0]))
	return edits
}

// unexported lowercases the first letter for a package-private function name.
func unexported(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// predDeclEdit builds the edit inserting the predicate declaration: a method
// on the local type, or a plain function for external types.
func predDeclEdit(ctx specEditCtx, first specMatch) textEdit {
	insertPos := specInsertPos(ctx.fset, ctx.f, ctx.src)
	firstCond := ctx.src[ctx.fset.Position(first.stmt.Cond.Pos()).Offset : ctx.fset.Position(first.stmt.Cond.End()).Offset]
	paramName := first.varName
	if paramName == "" {
		paramName = "v"
	}
	body := specRenameVar(string(firstCond), first.varName, paramName)
	var decl string
	if ctx.isLocal {
		recvName := baseTypeName(ctx.typeName)
		decl = fmt.Sprintf("\n// %s reports whether the business rule holds.\nfunc (%s %s) %s() bool {\n\treturn %s\n}\n", ctx.predName, paramName, recvName, ctx.predName, body)
	} else {
		funcName := unexported(ctx.predName)
		decl = fmt.Sprintf("\n// %s reports whether the business rule holds for %s.\nfunc %s(%s %s) bool {\n\treturn %s\n}\n", funcName, paramName, funcName, paramName, ctx.typeName, body)
	}
	return textEdit{start: insertPos, end: insertPos, replacement: []byte(decl)}
}

// specInsertPos finds where to insert the Specification type: after imports.
// Uses the end of the import declaration, not the last import spec: for
// parenthesized imports the spec ends before the closing paren.
func specInsertPos(fset *token.FileSet, f *ast.File, src []byte) int {
	if gen := importDecl(f); gen != nil {
		return lineEnd(src, fset.Position(gen.End()).Offset)
	}
	return lineEnd(src, fset.Position(f.Name.End()).Offset)
}

// importDecl returns the file's import declaration, or nil.
func importDecl(f *ast.File) *ast.GenDecl {
	for _, decl := range f.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			return gen
		}
	}
	return nil
}

// lineEnd returns the offset just past the newline at or after pos.
func lineEnd(src []byte, pos int) int {
	for pos < len(src) && src[pos] != '\n' {
		pos++
	}
	return pos + 1
}

// specRenameVar renames variable occurrences in a condition string.
// Uses word-boundary matching to avoid partial replacements.
func specRenameVar(cond, from, to string) string {
	if from == "" || from == to {
		return cond
	}
	var out strings.Builder
	i := 0
	for i < len(cond) {
		if n, ok := specVarAt(cond, i, from); ok {
			out.WriteString(to)
			i += n
			continue
		}
		out.WriteByte(cond[i])
		i++
	}
	return out.String()
}

// specVarAt reports whether the variable name starts at position i with
// word boundaries on both sides, returning its length.
func specVarAt(s string, i int, name string) (int, bool) {
	if !strings.HasPrefix(s[i:], name) {
		return 0, false
	}
	before := i == 0 || !isIdentChar(s[i-1])
	after := i+len(name) >= len(s) || !isIdentChar(s[i+len(name)])
	if !before || !after {
		return 0, false
	}
	return len(name), true
}

// isIdentChar reports whether c can be part of a Go identifier.
func isIdentChar(c byte) bool {
	return c == '_' || isASCIILetter(c) || isDigit(c)
}

// isASCIILetter reports whether c is an ASCII letter.
func isASCIILetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}
