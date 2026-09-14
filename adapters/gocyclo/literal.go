package gocyclo

import (
	"go/ast"
	"go/token"

	cyclo "github.com/fzipp/gocyclo"
)

// gocyclo recognizes only direct literals and names every initializer after the
// first variable. Normalize parentheses and index the real names in one pass.
func normalizeFunctionInitializers(file *ast.File, fileSet *token.FileSet) map[int]string {
	names := map[int]string{}
	for node := range ast.Preorder(file) {
		spec, ok := node.(*ast.ValueSpec)
		if ok {
			normalizeValueSpec(spec, fileSet, names)
		}
	}
	return names
}

func normalizeValueSpec(spec *ast.ValueSpec, fileSet *token.FileSet, names map[int]string) {
	for index, value := range spec.Values {
		literal, ok := ast.Unparen(value).(*ast.FuncLit)
		if !ok || index >= len(spec.Names) {
			continue
		}
		spec.Values[index] = literal
		names[fileSet.Position(literal.Pos()).Offset] = spec.Names[index].Name
	}
}

func nameFunctionLiterals(stats cyclo.Stats, names map[int]string) {
	for index := range stats {
		name, ok := names[stats[index].Pos.Offset]
		if ok {
			stats[index].FuncName = name
		}
	}
}
