package gopatterns

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/shanejonas/cyclo/domain/pdg"
	"golang.org/x/tools/go/packages"
)

func parseIR(t testing.TB, source string, pool *pdg.Builder) pdg.Graph {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	pkg, err := (&types.Config{}).Check("example", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &packages.Package{PkgPath: "example", Fset: fset, TypesInfo: info, Types: pkg}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			extracted, err := extractFunc(loaded, fn, "fixture.go", extractCtx{pool: pool})
			if err != nil {
				t.Fatal(err)
			}
			g := extracted.Pdg
			return g
		}
	}
	t.Fatal("no function")
	return pdg.Graph{}
}

func TestIRReassignmentRecordsDefinitionsWithoutFalseDefUse(t *testing.T) {
	source := "package example\nfunc f(x int) int { x = x + 1; x = x + 2; return x }"
	g := parseIR(t, source, pdg.NewBuilder())
	if err := pdg.Validate(&g); err != nil {
		t.Fatal(err)
	}
	if len(g.Definitions) != 3 {
		t.Fatalf("definitions=%d, want 3", len(g.Definitions))
	}
	var uses []pdg.Ref
	for _, edge := range g.Edges {
		if edge.Definition != 0 {
			uses = append(uses, edge.Definition)
		}
	}
	if len(uses) != 0 {
		t.Fatalf("def-use=%v; first-binding mining edges must not claim reaching definitions", uses)
	}
	for _, node := range g.Nodes {
		span := g.Spans[node.Span-1]
		if span.End > uint32(len(source)) || span.Start >= span.End {
			t.Fatalf("invalid source span: %+v", span)
		}
	}
}

func TestIRBranchesNeverClaimReachingDefinitionCompleteness(t *testing.T) {
	g := parseIR(t, "package example\nfunc f(x int, c bool) int { if c { x = 1 }; return x }", pdg.NewBuilder())
	if err := pdg.Validate(&g); err != nil {
		t.Fatal(err)
	}
	for _, edge := range g.Edges {
		if edge.Definition != 0 {
			t.Fatalf("unsupported dependency emitted: %+v", edge)
		}
	}
	if g.Conformance[0].Status != pdg.NonConformant {
		t.Fatal("incomplete extraction claims conformance")
	}
}

func TestIRUnresolvedMemoryAndInterfaceDispatch(t *testing.T) {
	g := parseIR(t, "package example\ntype I interface{ Run() }; func f(p, q *int, action func(), receiver I) int { *p = 1; action(); receiver.Run(); return *q }", pdg.NewBuilder())
	if err := pdg.Validate(&g); err != nil {
		t.Fatal(err)
	}
	if len(g.Locations) != 0 {
		t.Fatal("invented alias location")
	}
	unknownReads, unknownWrites, indirectCalls := 0, 0, 0
	for _, node := range g.Nodes {
		attrs := g.NodeAttributes(node)
		for _, read := range attrs.Reads {
			if read.UnresolvedReason != 0 {
				unknownReads++
			}
		}
		for _, write := range attrs.Writes {
			if write.UnresolvedReason != 0 {
				unknownWrites++
			}
		}
		if attrs.CallForm == pdg.IndirectCall {
			indirectCalls++
			if attrs.Callee.Status != pdg.Unknown {
				t.Fatal("dynamic target marked known")
			}
		}
	}
	if unknownReads < 1 || unknownWrites != 1 || indirectCalls != 2 {
		t.Fatalf("reads=%d writes=%d indirect=%d", unknownReads, unknownWrites, indirectCalls)
	}
}

func TestIRInterfacesAndSharedTypes(t *testing.T) {
	pool := pdg.NewBuilder()
	source := "package example\ntype T struct{}; func (r T) f(a, b int, rest ...string) (result int, err error) { return a, nil }"
	g := parseIR(t, source, pool)
	if err := pdg.Validate(&g); err != nil {
		t.Fatal(err)
	}
	if len(g.Function.Inputs) != 4 || len(g.Function.Outputs) != 2 {
		t.Fatal("incomplete interface")
	}
	if g.Function.Inputs[0].Role != pdg.ReceiverRole {
		t.Fatal("receiver role lost")
	}
	if g.Function.Inputs[1].Type.Value != g.Function.Inputs[2].Type.Value {
		t.Fatal("type strings are not interned")
	}
	if pool.Tables.Text(g.Function.Inputs[3].Variadic.Value) != "true" {
		t.Fatal("variadic string fact lost")
	}
	if pool.Tables.Text(g.Function.QualifiedName.Value) != pool.Tables.Text(g.Function.ID) {
		t.Fatal("method identity lost")
	}
	other := parseIR(t, source, pool)
	if other.Tables != g.Tables {
		t.Fatal("functions do not share tables")
	}
}

func TestExtractUsesCanonicalIR(t *testing.T) {
	extraction, err := Extract(context.Background(), "testdata/shapes", []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	graphs := make([]pdg.Graph, len(extraction.Funcs))
	for i, f := range extraction.Funcs {
		graphs[i] = f.Pdg
	}
	if len(graphs) == 0 {
		t.Fatal("no graphs")
	}
	for i := range graphs {
		if err := pdg.Validate(&graphs[i]); err != nil {
			t.Fatalf("%s: %v", graphs[i].Tables.Text(graphs[i].Function.ID), err)
		}
		if graphs[i].Tables != graphs[0].Tables {
			t.Fatal("package tables are not shared")
		}
	}
}

func BenchmarkIRRepeatedTypes(b *testing.B) {
	source := "package example\nfunc f(a, b, c, d string) string { a = b; c = d; return a + c }"
	b.ReportAllocs()
	for b.Loop() {
		pool := pdg.NewBuilder()
		for range 100 {
			parseIR(b, source, pool)
		}
	}
}

func TestIRParenthesizedOperands(t *testing.T) {
	g := parseIR(t, "package example\nfunc f(x int) int { x = (x + 1); return (x) }", pdg.NewBuilder())
	operandEdges := 0
	for _, edge := range g.Edges {
		if g.Tables.Text(edge.Subkind) == "mining-value" {
			operandEdges++
		}
	}
	if operandEdges != 5 {
		t.Fatalf("operand edges=%d, want 5", operandEdges)
	}
}

func TestIRVariadicTypeIsKnown(t *testing.T) {
	g := parseIR(t, "package example\nfunc f(rest ...string) {}", pdg.NewBuilder())
	if g.Function.Inputs[0].Type.Status != pdg.Known {
		t.Fatal("variadic type is unknown")
	}
}

func TestIRGlobalWritesRetainGlobalRole(t *testing.T) {
	g := parseIR(t, "package example\nvar shared int; func f() int { shared = 1; return shared }", pdg.NewBuilder())
	if len(g.Symbols) != 1 || g.Symbols[0].Role != pdg.GlobalRole {
		t.Fatal("global binding marked local")
	}
	for _, edge := range g.Edges {
		if edge.Definition != 0 {
			t.Fatal("global def-use claimed under local-only policy")
		}
	}
}
