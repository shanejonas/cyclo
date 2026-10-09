package gopatterns

import (
	"go/ast"
	"go/token"
)

// GuardClauseHit is one inverted conditional: an if/else where the else
// branch is just an early return while the happy path is nested in the if
// body. Inverting the condition into a guard clause lets the happy path
// flow at top level.
type GuardClauseHit struct {
	// Line is the line of the if statement.
	Line int
	// BodyStmts counts statements in the if body (the trapped happy path).
	BodyStmts int
}

// findGuardClauses scans fn for inverted conditionals that want to be
// guard clauses.
func findGuardClauses(fn *ast.FuncDecl, fset *token.FileSet) []GuardClauseHit {
	var hits []GuardClauseHit
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if isInvertedGuard(ifStmt) {
			hits = append(hits, GuardClauseHit{
				Line:      fset.PositionFor(ifStmt.Pos(), false).Line,
				BodyStmts: len(ifStmt.Body.List),
			})
		}
		return true
	})
	return hits
}

// isInvertedGuard reports whether ifStmt traps the happy path: a plain else
// block (not an else-if chain) that ends in a return, while the if body
// does not end in a return and holds a nontrivial happy path.
func isInvertedGuard(ifStmt *ast.IfStmt) bool {
	if ifStmt.Else == nil {
		return false
	}
	elseBlock, ok := ifStmt.Else.(*ast.BlockStmt)
	if !ok {
		return false
	}
	if len(ifStmt.Body.List) < 2 {
		return false
	}
	return blockEndsReturn(elseBlock) && !blockEndsReturn(ifStmt.Body)
}

// blockEndsReturn reports whether the block's last statement is a return.
func blockEndsReturn(b *ast.BlockStmt) bool {
	if b == nil || len(b.List) == 0 {
		return false
	}
	_, ok := b.List[len(b.List)-1].(*ast.ReturnStmt)
	return ok
}
