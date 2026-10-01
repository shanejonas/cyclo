package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"

	"golang.org/x/tools/go/ast/astutil"
)

// Reduction may delete code independently from each member of a pair. Reject
// those candidates unless the remaining bodies still differ only by the
// generated, harmless transformations. Runtime equality alone is insufficient.
func equivalentSource(source []byte, cases []specimen) error {
	functions, err := canonicalFunctions(source)
	if err != nil {
		return fmt.Errorf("%w: %v", invalidCandidate, err)
	}
	for _, c := range cases {
		base, baseOK := functions[c.Base]
		variant, variantOK := functions[c.Name]
		if !baseOK || !variantOK || !bytes.Equal(base, variant) {
			return fmt.Errorf("%w: %s is no longer equivalent to %s", invalidCandidate, c.Name, c.Base)
		}
	}
	return nil
}

func canonicalFunctions(source []byte) (map[string][]byte, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "input.go", source, 0)
	if err != nil {
		return nil, err
	}
	normalized := map[string][]byte{}
	for _, decl := range file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := function.Name.Name
		normalize(function)
		function.Name.Name = "Target"
		var output bytes.Buffer
		if err := format.Node(&output, token.NewFileSet(), function); err != nil {
			return nil, err
		}
		normalized[name] = output.Bytes()
	}
	return normalized, nil
}

func normalize(function *ast.FuncDecl) {
	astutil.Apply(function, func(cursor *astutil.Cursor) bool {
		switch node := cursor.Node().(type) {
		case *ast.ParenExpr:
			cursor.Replace(ast.Unparen(node))
		case *ast.AssignStmt:
			if harmlessDiscard(node) {
				cursor.Delete()
				return false
			}
		}
		return true
	}, func(cursor *astutil.Cursor) bool {
		if id, ok := cursor.Node().(*ast.Ident); ok && id.Name == "ownedOrShared" {
			id.Name = "local"
		}
		return true
	})
}

func harmlessDiscard(statement *ast.AssignStmt) bool {
	if statement.Tok != token.ASSIGN || len(statement.Lhs) != 1 || len(statement.Rhs) != 1 {
		return false
	}
	return blankIdentifier(statement.Lhs[0]) && zeroLiteral(statement.Rhs[0])
}

func blankIdentifier(expression ast.Expr) bool {
	id, ok := ast.Unparen(expression).(*ast.Ident)
	return ok && id.Name == "_"
}

func zeroLiteral(expression ast.Expr) bool {
	literal, ok := ast.Unparen(expression).(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == "0"
}
