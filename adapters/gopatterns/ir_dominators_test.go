package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/cfg"
	"testing"
)

func TestExecutionDominatorsMatchIndependentReachability(t *testing.T) {
	source := `package example
 func f() { a(); for c { if d { b(); continue }; if e { break }; z() }; if g { h() } else { i() }; goto L; unreachable(); L: end() }`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	flow := cfg.New(file.Decls[0].(*ast.FuncDecl).Body, func(*ast.CallExpr) bool { return true })
	dom := newExecutionDominators(flow)
	for _, a := range dom.order {
		for _, b := range dom.order {
			expected := !executionReachableWithout(flow.Blocks[0], b, a, map[int32]bool{})
			actual := executionDominates(dom, a, b)
			if actual != expected {
				t.Fatalf("block %d dominates %d=%t want %t", a, b, actual, expected)
			}
		}
	}
}

func executionReachableWithout(block *cfg.Block, target, excluded int, seen map[int32]bool) bool {
	if int(block.Index) == excluded || seen[block.Index] {
		return false
	}
	if int(block.Index) == target {
		return true
	}
	seen[block.Index] = true
	for _, next := range block.Succs {
		if executionReachableWithout(next, target, excluded, seen) {
			return true
		}
	}
	return false
}

func executionDominates(dom *executionDominators, a, b int) bool {
	for b != a && b != 0 {
		b = dom.idom[b]
	}
	return a == b
}
