package gopatterns

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
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
