package gopatterns

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"github.com/shanejonas/cyclo/domain/patterns"
	"github.com/shanejonas/cyclo/domain/pdg"
	"golang.org/x/tools/go/packages"
)

type irBuilder struct {
	attributes                                          []pdg.Attributes
	outputs                                             []pdg.Ref
	pool                                                *pdg.Builder
	graph                                               pdg.Graph
	info                                                *types.Info
	fset                                                *token.FileSet
	symbols                                             map[types.Object]pdg.Ref
	nodes                                               map[ast.Node]pdg.Ref
	writeNames                                          map[*ast.Ident]bool
	stack                                               []ast.Node
	policy, sourceEvidence, sourceOrigin                pdg.Ref
	executionPolicy, executionEvidence, executionOrigin pdg.Ref
}

func newIRBuilder(pkg *packages.Package, fn *ast.FuncDecl, path string, pool *pdg.Builder) *irBuilder {
	b := &irBuilder{
		pool: pool, info: pkg.TypesInfo, fset: pkg.Fset,
		symbols: map[types.Object]pdg.Ref{}, nodes: map[ast.Node]pdg.Ref{},
		writeNames: map[*ast.Ident]bool{},
	}
	b.policy = pool.Policy("cyclo.abstraction", "0.1.0")
	b.sourceEvidence = pool.Evidence(pdg.Approximated, "Operation identified in typed source; dependency completeness is separate.", b.policy)
	b.sourceOrigin = pool.Origin(pdg.SourceOrigin, "Go source operation; abstraction lowering is approximate.", b.policy)
	b.header(pkg, fn, path)
	return b
}

func (b *irBuilder) header(pkg *packages.Package, fn *ast.FuncDecl, path string) {
	p := b.pool
	qualified := b.functionName(fn)
	identity := p.Text(pkg.PkgPath + "." + fn.Name.Name)
	if qualified.Status == pdg.Known {
		identity = qualified.Value
	}
	b.graph = pdg.Graph{
		Tables: p.Tables, Profile: b.policy,
		Producer: pdg.Producer{
			Name: p.Text("cyclo"), Version: p.Text("pdg-ir-0.1.0"), Language: p.Text("go"),
			LanguageVersion: p.Missing(pdg.Unknown, "Compiler language version is not recorded."), Configuration: p.Text("{}"),
		},
		Sources: []pdg.Source{{ID: p.Text(path), Path: p.Text(path), ContentIdentity: p.Missing(pdg.Unknown, "Source content digest is not computed.")}},
		Function: pdg.Function{
			ID: identity, Name: p.Known(fn.Name.Name), QualifiedName: qualified, Span: b.span(fn),
			Interface:      p.Evidence(pdg.Supported, "Complete typed function interface.", b.policy),
			ReferenceCount: pdg.CountFact{Status: pdg.Unknown, Reason: p.Text("Reference-count policy is not selected.")},
		},
	}
}

func (b *irBuilder) functionName(fn *ast.FuncDecl) pdg.Fact {
	object, ok := b.info.Defs[fn.Name].(*types.Func)
	if !ok {
		return b.pool.Missing(pdg.Unknown, "Function identity is unresolved.")
	}
	name := patterns.FuncID(object)
	return b.pool.Known(name)
}

func (b *irBuilder) span(n ast.Node) pdg.Span {
	if n == nil {
		return b.graph.Function.Span
	}
	start := b.fset.PositionFor(n.Pos(), false).Offset
	end := b.fset.PositionFor(n.End(), false).Offset
	return pdg.Span{Source: 1, Start: uint32(start), End: uint32(end)}
}

func (b *irBuilder) addNode(n ast.Node, category pdg.Category, attrs pdg.Attributes) pdg.Ref {
	if ref := b.nodes[n]; ref != 0 {
		node := &b.graph.Nodes[ref-1]
		node.Category = category
		b.attributes[node.Attributes-1] = attrs
		return ref
	}
	return b.appendNode(n, category, attrs)
}

func (b *irBuilder) appendNode(n ast.Node, category pdg.Category, attrs pdg.Attributes) pdg.Ref {
	id := pdg.Ref(len(b.graph.Nodes) + 1)
	b.graph.Spans = append(b.graph.Spans, b.span(n))
	b.attributes = append(b.attributes, attrs)
	b.graph.Nodes = append(b.graph.Nodes, pdg.Node{
		ID: b.pool.Text("n" + strconv.Itoa(int(id))), Category: category,
		OriginalKind: b.pool.Known(fmt.Sprintf("%T", n)), Span: pdg.Ref(len(b.graph.Spans)),
		Attributes: pdg.Ref(len(b.attributes)), Evidence: b.sourceEvidence, Provenance: b.sourceOrigin,
	})
	if n != nil {
		b.nodes[n] = id
	}
	return id
}

func (b *irBuilder) symbol(id *ast.Ident, role pdg.Role) pdg.Ref {
	obj := b.info.ObjectOf(id)
	if obj == nil || id.Name == "_" {
		return 0
	}
	if ref, ok := b.symbols[obj]; ok {
		return ref
	}
	if obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope() {
		role = pdg.GlobalRole
	}
	ref := pdg.Ref(len(b.graph.Symbols) + 1)
	b.graph.Symbols = append(b.graph.Symbols, pdg.Symbol{
		ID: b.pool.Text("s" + strconv.Itoa(int(ref))), Scope: b.scope(obj),
		Name: b.pool.Known(obj.Name()), Type: b.pool.Known(irTypeString(obj.Type())), Role: role,
	})
	b.symbols[obj] = ref
	return ref
}

func (b *irBuilder) scope(obj types.Object) pdg.Text {
	if obj.Parent() == nil {
		return b.graph.Function.ID
	}
	return b.pool.Text("scope:" + strconv.Itoa(int(obj.Parent().Pos())))
}

func (b *irBuilder) definition(node, symbol pdg.Ref) {
	if symbol == 0 {
		return
	}
	id := pdg.Ref(len(b.graph.Definitions) + 1)
	b.graph.Definitions = append(b.graph.Definitions, pdg.Definition{
		ID: b.pool.Text("d" + strconv.Itoa(int(id))), Node: node, Symbol: symbol,
	})
	attrs := &b.attributes[b.graph.Nodes[node-1].Attributes-1]
	attrs.Defines = append(attrs.Defines, id)
}

// buildGraph is the sole production graph construction path.
func buildGraph(pkg *packages.Package, fn *ast.FuncDecl, path string, pool *pdg.Builder) (pdg.Graph, error) {
	file := pkg.Fset.File(fn.Pos())
	if file == nil || uint64(file.Size()) > uint64(^uint32(0)) {
		return pdg.Graph{}, fmt.Errorf("PDG source exceeds 32-bit offset range")
	}
	b := &builder{info: pkg.TypesInfo, fset: pkg.Fset, binds: map[types.Object]int{}, ir: newIRBuilder(pkg, fn, path, pool)}
	b.ir.interfaces(fn)
	b.params(fn)
	b.stmt(fn.Body)
	b.ir.enrich(fn.Body)
	b.ir.returnValues(fn.Body)
	b.ir.references()
	b.ir.execution(fn.Body)
	b.ir.capabilities()
	if err := b.ir.pack(); err != nil {
		return pdg.Graph{}, err
	}
	return b.ir.graph, nil
}
