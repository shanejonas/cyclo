package gopatterns

import (
	"go/ast"
	"go/types"
	"strconv"

	"github.com/shanejonas/cyclo/domain/pdg"
)

func (b *irBuilder) interfaces(fn *ast.FuncDecl) {
	b.fields(fn.Recv, pdg.ReceiverRole, false)
	b.fields(fn.Type.Params, pdg.ParameterRole, false)
	b.fields(fn.Type.Results, pdg.ReturnRole, true)
}

func (b *irBuilder) fields(fields *ast.FieldList, role pdg.Role, output bool) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		b.field(field, role, output)
	}
}

func (b *irBuilder) field(field *ast.Field, role pdg.Role, output bool) {
	if len(field.Names) == 0 {
		b.parameter(field, nil, role, output)
		return
	}
	for _, name := range field.Names {
		b.parameter(field, name, role, output)
	}
}

func (b *irBuilder) parameter(field *ast.Field, name *ast.Ident, role pdg.Role, output bool) {
	params := &b.graph.Function.Inputs
	category := pdg.FormalInput
	if output {
		params = &b.graph.Function.Outputs
		category = pdg.FormalOutput
	}
	param := b.parameterFacts(field, name, role)
	param.Position = uint32(len(*params))
	*params = append(*params, param)
	attrs := pdg.Attributes{Position: pdg.Position(param.Position + 1)}
	if param.Symbol != 0 {
		attrs.Symbols = []pdg.Ref{param.Symbol}
	}
	node := b.appendNode(field, category, attrs)
	if output {
		b.outputs = append(b.outputs, node)
	}
	if !output || name != nil {
		b.definition(node, param.Symbol)
	}

}

func (b *irBuilder) parameterFacts(field *ast.Field, name *ast.Ident, role pdg.Role) pdg.Parameter {
	_, variadic := field.Type.(*ast.Ellipsis)
	p := pdg.Parameter{
		Role: role, Name: b.pool.Known(""), Type: b.typeFact(field.Type),
		Variadic: b.pool.Known(strconv.FormatBool(variadic)),
	}
	if name != nil {
		p.Name = b.pool.Known(name.Name)
		p.Symbol = b.symbol(name, symbolRole(role))
	}
	return p
}

func symbolRole(role pdg.Role) pdg.Role {
	if role == pdg.ReturnRole {
		return pdg.OutputRole
	}
	return role
}

func (b *irBuilder) typeFact(expr ast.Expr) pdg.Fact {
	typ := b.info.TypeOf(expr)
	if typ == nil {
		return b.pool.Missing(pdg.Unknown, "Type is unresolved.")
	}
	return b.pool.Known(irTypeString(typ))
}

func irTypeString(typ types.Type) string {
	return types.TypeString(typ, func(pkg *types.Package) string { return pkg.Path() })
}
