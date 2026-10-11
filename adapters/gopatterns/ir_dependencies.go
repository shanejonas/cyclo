package gopatterns

import (
	"go/ast"
	"go/types"
)

func (b *irBuilder) dynamicMethod(expr ast.Expr) bool {
	selector, ok := unparen(expr).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	typ := b.info.TypeOf(selector.X)
	if typ == nil {
		return false
	}
	_, dynamic := typ.Underlying().(*types.Interface)
	return dynamic
}
