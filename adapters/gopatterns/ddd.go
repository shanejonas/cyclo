package gopatterns

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// DDD structural extractors: aggregates, repositories, factories (Evans).

// findAggregateMods records the named struct types whose fields are
// mutated inside fn: `x.Field = v`, `x.Field++`, `x.Field += v`.
// FindAggregateMods is exported for the quality gate: it records the
// named struct types whose fields are mutated inside fn.
func FindAggregateMods(fn *ast.FuncDecl, info *types.Info) []patterns.AggregateModHit {
	seen := map[string]bool{}
	var out []patterns.AggregateModHit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		for _, lhs := range assignTargets(n) {
			id, ok := baseIdent(lhs)
			if !ok {
				continue
			}
			name, ok := namedStructName(info, id)
			if !ok || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, patterns.AggregateModHit{TypeName: name})
		}
		return true
	})
	return out
}

// assignTargets returns the mutated LHS expressions of an assignment:
// selector targets of `=`/`+=`/etc. and `++`/`--`.
func assignTargets(n ast.Node) []ast.Expr {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		var out []ast.Expr
		for _, lhs := range stmt.Lhs {
			if _, ok := lhs.(*ast.SelectorExpr); ok {
				out = append(out, lhs)
			}
		}
		return out
	case *ast.IncDecStmt:
		if _, ok := stmt.X.(*ast.SelectorExpr); ok {
			return []ast.Expr{stmt.X}
		}
	}
	return nil
}

// baseIdent unwraps selector/index/star expressions to the base identifier:
// `a.B[i].C` -> `a`.
func baseIdent(e ast.Expr) (*ast.Ident, bool) {
	for {
		inner, done := unwrapOne(e)
		if done {
			id, ok := inner.(*ast.Ident)
			return id, ok
		}
		e = inner
	}
}

// unwrapOne strips one wrapper expression, reporting done when e is not
// a wrapper.
func unwrapOne(e ast.Expr) (ast.Expr, bool) {
	switch n := e.(type) {
	case *ast.SelectorExpr:
		return n.X, false
	case *ast.IndexExpr:
		return n.X, false
	case *ast.StarExpr:
		return n.X, false
	case *ast.ParenExpr:
		return n.X, false
	}
	return e, true
}

// namedStructName returns the named struct type of id, or false.
func namedStructName(info *types.Info, id *ast.Ident) (string, bool) {
	t := info.TypeOf(id)
	if t == nil {
		return "", false
	}
	named, ok := derefNamed(t)
	if !ok {
		return "", false
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return "", false
	}
	return named.Obj().Name(), true
}

// derefNamed dereferences pointers to a named type.
func derefNamed(t types.Type) (*types.Named, bool) {
	for {
		p, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	return named, ok
}

// dbReceiverNames are heuristic variable names for database handles.
var dbReceiverNames = map[string]bool{
	"db": true, "DB": true, "tx": true, "Tx": true,
	"conn": true, "Conn": true, "sqlDB": true, "database": true,
}

// dbMethods are methods that indicate a database call.
var dbMethods = map[string]bool{
	"Query": true, "QueryRow": true, "QueryContext": true,
	"Exec": true, "ExecContext": true,
	"Prepare": true, "PrepareContext": true,
	"Begin": true, "BeginTx": true, "Commit": true, "Rollback": true,
}

// dbPackages are package names that indicate a database call.
var dbPackages = map[string]bool{
	"sql": true, "gorm": true, "pgx": true, "sqlx": true,
	"ent": true, "bun": true, "pop": true,
}

// isRepositoryFile reports whether path looks like a repository file:
// the db calls there are the repository itself, not a violation.
// Per Shane's convention, repositories live in adapters/ — any file
// under an adapters/ directory is infrastructure, not business logic.
func isRepositoryFile(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	for _, seg := range strings.Split(lower, "/") {
		seg = strings.TrimSuffix(seg, ".go")
		if seg == "adapters" {
			return true
		}
		if isRepoSegment(seg) {
			return true
		}
	}
	return false
}

// isRepoSegment reports whether a path segment names a repository location.
func isRepoSegment(seg string) bool {
	switch seg {
	case "repository", "repositories", "repo", "repos",
		"dao", "daos", "store", "stores", "storage":
		return true
	}
	return hasRepoAffix(seg)
}

// hasRepoAffix reports whether seg has a repo/repository prefix or suffix.
func hasRepoAffix(seg string) bool {
	for _, affix := range []string{"repo_", "_repo", "repository_", "_repository"} {
		if strings.HasPrefix(seg, affix) || strings.HasSuffix(seg, affix) {
			return true
		}
	}
	return false
}

// findDbCalls records direct database calls in fn, skipping repository
// files. Detection uses types when available, with name heuristics as
// fallback.
// FindDbCalls is exported for the quality gate: it records direct
// database calls in fn, skipping repository files.
func FindDbCalls(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info, filePath string) []patterns.DbCallHit {
	if isRepositoryFile(filePath) {
		return nil
	}
	var out []patterns.DbCallHit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if isDbCall(sel, info) {
			out = append(out, patterns.DbCallHit{
				Line: fset.Position(call.Pos()).Line,
				Call: selectorText(sel),
			})
		}
		return true
	})
	return out
}

// isDbCall reports whether sel looks like a database call.
func isDbCall(sel *ast.SelectorExpr, info *types.Info) bool {
	if id, ok := sel.X.(*ast.Ident); ok && isDbIdentCall(id, sel.Sel.Name, info) {
		return true
	}
	return isSqlHandleType(info.TypeOf(sel.X))
}

// isDbIdentCall reports whether id.Method is a db call: a known db
// package, a sql handle type, or a db-named receiver with a db method.
func isDbIdentCall(id *ast.Ident, method string, info *types.Info) bool {
	return isDbPackageRef(id, info) ||
		isSqlHandleType(info.TypeOf(id)) ||
		(dbReceiverNames[id.Name] && dbMethods[method])
}

// isDbPackageRef reports whether id refers to a known db package.
func isDbPackageRef(id *ast.Ident, info *types.Info) bool {
	obj, ok := info.Uses[id]
	if !ok {
		return false
	}
	pn, ok := obj.(*types.PkgName)
	return ok && dbPackages[pn.Name()]
}

// isSqlHandleType reports whether t is from database/sql.
func isSqlHandleType(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := derefNamed(t)
	if !ok {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && pkg.Path() == "database/sql"
}

// selectorText renders `db.Query` from a selector expression.
func selectorText(sel *ast.SelectorExpr) string {
	if id, ok := sel.X.(*ast.Ident); ok {
		return id.Name + "." + sel.Sel.Name
	}
	return sel.Sel.Name
}

// findFactoryLits records struct literals with 5+ fields. Only literals
// of named structs declared in the current package are recorded: the
// factory must live with the type. HasLogic is set when the enclosing
// function has construction logic (validation, defaults, error handling)
// that a factory could encapsulate — plain field assignment doesn't qualify.
func findFactoryLits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info, pkg *types.Package) []patterns.FactoryHit {
	var out []patterns.FactoryHit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || len(lit.Elts) < 5 {
			return true
		}
		name, declFile, ok := litStructTarget(lit, fset, info, pkg)
		if !ok {
			return true
		}
		ctx := constructionCtx{fset: fset, info: info, typeName: name}
		out = append(out, patterns.FactoryHit{
			Line:      fset.Position(lit.Pos()).Line,
			TypeName:  name,
			NumFields: len(lit.Elts),
			DeclFile:  declFile,
			HasLogic:  litHasConstructionLogic(fn, lit, ctx),
		})
		return true
	})
	return out
}

// constructionLogicWindow is how close (in lines) an if statement must be
// to a struct literal to count as construction logic.
const constructionLogicWindow = 5

// constructionCtx bundles the type-checking context for
// construction-logic detection (keeps fn_params within the gate).
type constructionCtx struct {
	fset     *token.FileSet
	info     *types.Info
	typeName string
}

// litHasConstructionLogic reports whether the struct literal's construction
// has logic that a factory could encapsulate. The logic must be ABOUT the
// construction, not just anywhere in the function:
//   - an if statement within 5 lines of the literal (validation/defaults), or
//   - an if statement referencing the struct type, or
//   - the function returns (T, error) where T is the struct type.
func litHasConstructionLogic(fn *ast.FuncDecl, lit *ast.CompositeLit, ctx constructionCtx) bool {
	if fn.Body == nil {
		return false
	}
	if funcReturnsStructAndError(fn, ctx) {
		return true
	}
	litLine := ctx.fset.Position(lit.Pos()).Line
	logic := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if ctx.ifIsLogic(ifStmt, litLine) {
			logic = true
			return false
		}
		return true
	})
	return logic
}

// ifIsLogic reports whether the if statement is construction logic for the
// literal: close to it in the source, or referencing the struct type.
func (ctx constructionCtx) ifIsLogic(ifStmt *ast.IfStmt, litLine int) bool {
	ifLine := ctx.fset.Position(ifStmt.Pos()).Line
	if ifLine >= litLine-constructionLogicWindow && ifLine <= litLine+constructionLogicWindow {
		return true
	}
	return ctx.ifRefsType(ifStmt)
}

// ifRefsType reports whether the if statement mentions the struct type,
// either by name or via a variable of that type.
func (ctx constructionCtx) ifRefsType(ifStmt *ast.IfStmt) bool {
	refs := false
	ast.Inspect(ifStmt, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Name == ctx.typeName {
			refs = true
			return false
		}
		if isStructNamed(ctx.info.TypeOf(id), ctx.typeName) {
			refs = true
			return false
		}
		return true
	})
	return refs
}

// funcReturnsStructAndError reports whether fn returns the named struct type
// and an error: fallible construction wants a factory.
func funcReturnsStructAndError(fn *ast.FuncDecl, ctx constructionCtx) bool {
	if fn.Type.Results == nil {
		return false
	}
	var hasT, hasErr bool
	for _, field := range fn.Type.Results.List {
		t := ctx.info.TypeOf(field.Type)
		switch {
		case isErrorType(t):
			hasErr = true
		case isStructNamed(t, ctx.typeName):
			hasT = true
		}
	}
	return hasT && hasErr
}

// isStructNamed reports whether t is the named struct type.
func isStructNamed(t types.Type, typeName string) bool {
	named, ok := litNamedStruct(t)
	return ok && named.Obj() != nil && named.Obj().Name() == typeName
}

// isErrorType reports whether t is the builtin error type.
func isErrorType(t types.Type) bool {
	named, ok := t.(*types.Named)
	return ok && named.Obj() != nil && named.Obj().Name() == "error"
}

// litStructTarget returns the named struct type of a composite literal and
// the file declaring it. Only same-package named structs qualify.
func litStructTarget(lit *ast.CompositeLit, fset *token.FileSet, info *types.Info, pkg *types.Package) (string, string, bool) {
	named, ok := litNamedStruct(info.TypeOf(lit))
	if !ok {
		return "", "", false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() != pkg {
		return "", "", false
	}
	declFile := fset.Position(obj.Pos()).Filename
	if declFile == "" {
		return "", "", false
	}
	return obj.Name(), declFile, true
}

// litNamedStruct returns the named struct type of t, or false.
func litNamedStruct(t types.Type) (*types.Named, bool) {
	if t == nil {
		return nil, false
	}
	named, ok := derefNamed(t)
	if !ok {
		return nil, false
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return nil, false
	}
	return named, true
}

// specMinOperands is the minimum number of && / || operands for a boolean
// expression to count as a business rule worth a Specification.
const specMinOperands = 2

// findSpecificationHits records boolean business-rule expressions: if
// conditions with 2+ && / || operands. The rule key normalizes field
// accesses so the same rule in different functions groups together even
// when variable names differ.
func findSpecificationHits(fn *ast.FuncDecl, fset *token.FileSet, info *types.Info) []patterns.SpecificationHit {
	var out []patterns.SpecificationHit
	if fn.Body == nil {
		return out
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		operands := flattenBoolOps(ifStmt.Cond)
		if len(operands) < specMinOperands {
			return true
		}
		ruleKey, varName, typeName := specRuleKey(ifStmt.Cond, info)
		if ruleKey == "" {
			return true
		}
		var condBuf strings.Builder
		if err := printer.Fprint(&condBuf, fset, ifStmt.Cond); err != nil {
			return true
		}
		out = append(out, patterns.SpecificationHit{
			Line:     fset.Position(ifStmt.Pos()).Line,
			RuleKey:  ruleKey,
			CondText: condBuf.String(),
			VarName:  varName,
			TypeName: typeName,
		})
		return true
	})
	return out
}

// flattenBoolOps flattens a && / || chain into its leaf operands.
func flattenBoolOps(e ast.Expr) []ast.Expr {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok || (bin.Op != token.LAND && bin.Op != token.LOR) {
		return []ast.Expr{e}
	}
	return append(flattenBoolOps(bin.X), flattenBoolOps(bin.Y)...)
}

// specRuleKey builds a normalized grouping key from a boolean condition:
// the sorted field accesses with their comparison operators, plus the
// tested variable's type. Returns the key, variable name, and type name.
func specRuleKey(cond ast.Expr, info *types.Info) (key, varName, typeName string) {
	operands := flattenBoolOps(cond)
	if len(operands) < specMinOperands {
		return "", "", ""
	}
	parts, seenVar, seenType, ok := specKeyParts(operands, info)
	if !ok {
		return "", "", ""
	}
	sort.Strings(parts)
	return seenType + "|" + strings.Join(parts, ","), seenVar, seenType
}

// specKeyParts extracts the normalized parts, base variable, and type from
// boolean operands. Reports false when operands don't form a single-subject
// rule. The key includes literal values so `x < 0.60` and `x < 0.90` are
// different rules (merging them would change behavior).
func specKeyParts(operands []ast.Expr, info *types.Info) (parts []string, seenVar, seenType string, ok bool) {
	for _, op := range operands {
		field, opStr, base, value := specOperandParts(op, info)
		if field == "" {
			return nil, "", "", false
		}
		parts = append(parts, field+":"+opStr+":"+value)
		if seenVar == "" {
			seenVar = base
		} else if seenVar != base {
			// Rule spans multiple variables; not a single-subject rule.
			return nil, "", "", false
		}
		if seenType == "" {
			seenType = specBaseType(op, info)
		}
	}
	return parts, seenVar, seenType, true
}

// specOperandParts extracts the field name, operator, base variable, and
// compared value from one boolean operand like `user.Age > 18` or `user.Active`.
func specOperandParts(op ast.Expr, info *types.Info) (field, opStr, base, value string) {
	op = specUnwrapParens(op)
	if field, base, ok := specBareSelector(op); ok {
		return field, "truthy", base, ""
	}
	return specComparisonParts(op)
}

// specUnwrapParens strips parenthesized wrappers from an expression.
func specUnwrapParens(op ast.Expr) ast.Expr {
	for {
		paren, ok := op.(*ast.ParenExpr)
		if !ok {
			return op
		}
		op = paren.X
	}
}

// specBareSelector extracts field and base from `user.Active`.
func specBareSelector(op ast.Expr) (field, base string, ok bool) {
	sel, ok := op.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return sel.Sel.Name, id.Name, true
}

// specComparisonParts extracts field, operator, base, and compared value
// from `user.Age > 18`. Reports empty when the comparison isn't a
// single-subject rule: both sides must resolve to the same base variable
// (or a literal), otherwise a rule like `c.x != first.x` would extract
// with a dangling `first` reference.
func specComparisonParts(op ast.Expr) (field, opStr, base, value string) {
	bin, ok := op.(*ast.BinaryExpr)
	if !ok {
		return "", "", "", ""
	}
	sel := specSideSelector(bin)
	if sel == nil {
		return "", "", "", ""
	}
	id := sel.X.(*ast.Ident)
	base = id.Name
	// The other side must be the same variable or a non-variable expression.
	var other ast.Expr
	if isSelectorOfIdent(bin.X) && bin.X.(*ast.SelectorExpr) == sel {
		other = bin.Y
	} else {
		other = bin.X
	}
	if !specSideIsSameOrLiteral(other, base) {
		return "", "", "", ""
	}
	return sel.Sel.Name, bin.Op.String(), base, specValueString(other)
}

// specValueString renders the compared value for the rule key: literals
// by their source text, nil as "nil", anything else as its kind. Two rules
// with different literals (0.60 vs 0.90) must not share a key.
func specValueString(e ast.Expr) string {
	e = specUnwrapParens(e)
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Value
	case *ast.Ident:
		if v.Name == "nil" {
			return "nil"
		}
		return "ident:" + v.Name
	default:
		return "expr"
	}
}

// specSideIsSameOrLiteral reports whether e is a selector on the same base
// variable, or an expression that doesn't reference a different variable
// (literal, call, etc.).
func specSideIsSameOrLiteral(e ast.Expr, base string) bool {
	e = specUnwrapParens(e)
	if sel, ok := e.(*ast.SelectorExpr); ok {
		if id, ok := sel.X.(*ast.Ident); ok {
			return id.Name == base
		}
		return false
	}
	if id, ok := e.(*ast.Ident); ok {
		// A bare identifier: same variable is fine (e.g. `c != nil`
		// handled elsewhere); a different variable is not.
		return id.Name == base || id.Name == "nil"
	}
	// Literals, calls, conversions: no variable reference to leak.
	return true
}

// specSideSelector returns the `ident.Field` selector on either side of a
// binary expression, or nil.
func specSideSelector(bin *ast.BinaryExpr) *ast.SelectorExpr {
	if isSelectorOfIdent(bin.X) {
		return bin.X.(*ast.SelectorExpr)
	}
	if isSelectorOfIdent(bin.Y) {
		return bin.Y.(*ast.SelectorExpr)
	}
	return nil
}

// isSelectorOfIdent reports whether e is `ident.Field`.
func isSelectorOfIdent(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	_, ok = sel.X.(*ast.Ident)
	return ok
}

// specBaseType returns the named type of the base variable in an operand.
func specBaseType(op ast.Expr, info *types.Info) string {
	if info == nil {
		return ""
	}
	sel := specFirstSelector(op)
	if sel == nil {
		return ""
	}
	return specNamedType(info.TypeOf(sel.X.(*ast.Ident)))
}

// specFirstSelector returns the first `ident.Field` selector in the expression.
func specFirstSelector(op ast.Expr) *ast.SelectorExpr {
	var sel *ast.SelectorExpr
	ast.Inspect(op, func(n ast.Node) bool {
		if s, ok := n.(*ast.SelectorExpr); ok {
			if _, ok := s.X.(*ast.Ident); ok {
				sel = s
				return false
			}
		}
		return true
	})
	return sel
}

// specNamedType returns the named type's name, or "".
func specNamedType(t types.Type) string {
	if t == nil {
		return ""
	}
	if named, ok := derefNamed(t); ok {
		return named.Obj().Name()
	}
	return ""
}
