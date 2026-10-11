package pdg

import (
	"strings"
	"testing"
	"unsafe"
)

func fixture() (*Graph, *Builder) {
	b := NewBuilder()
	policy := b.Policy("fixture", "1")
	evidence := b.Evidence(Supported, "Fixture operation.", policy)
	origin := b.Origin(SourceOrigin, "Fixture source.", policy)
	g := &Graph{
		Tables: b.Tables, Profile: policy,
		Producer: Producer{Name: b.Text("fixture"), Version: b.Text("1"), Language: b.Text("go"), LanguageVersion: b.Known("1.26"), Configuration: b.Text("{}")},
		Sources:  []Source{{ID: b.Text("source"), Path: b.Text("source.go"), ContentIdentity: b.Missing(Unknown, "Not hashed.")}},
		Function: Function{ID: b.Text("f"), Name: b.Known("f"), QualifiedName: b.Known("example.f"), Span: Span{Source: 1, End: 20}, Interface: evidence,
			ReferenceCount: CountFact{Status: Unknown, Reason: b.Text("Policy unresolved.")}},
		Nodes: []Node{{ID: b.Text("n1"), Category: Call, OriginalKind: b.Known("ast.CallExpr"), Span: 1, Evidence: evidence, Provenance: origin}},
		Spans: []Span{{Source: 1, Start: 10, End: 13}},
	}
	for _, name := range RequiredCapabilities {
		g.Capabilities = append(g.Capabilities, NamedCapability{Name: b.Text(name), Evidence: b.Evidence(Unknown, "Fixture does not certify extraction.", 0)})
	}
	return g, b
}

func TestValidGraphAndUnknownEvidence(t *testing.T) {
	g, _ := fixture()
	if err := Validate(g); err != nil {
		t.Fatal(err)
	}
	if g.Function.ReferenceCount.Status == Known {
		t.Fatal("unknown reference count became a known zero")
	}
}

func TestRejectInvalidIR(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Graph, *Builder)
	}{
		{"dangling evidence", func(g *Graph, _ *Builder) { g.Nodes[0].Evidence = 100 }},
		{"source span required", func(g *Graph, _ *Builder) { g.Nodes[0].Span = 0 }},
		{"invalid span", func(g *Graph, _ *Builder) { g.Spans[0].Start = 99 }},
		{"duplicate nodes", func(g *Graph, _ *Builder) { g.Nodes = append(g.Nodes, g.Nodes[0]) }},
		{"missing capability", func(g *Graph, _ *Builder) { g.Capabilities = g.Capabilities[1:] }},
		{"invalid fact", func(g *Graph, b *Builder) { g.Function.Name = b.Missing(Known, ""); g.Function.Name.Status = Supported }},
		{"unknown without reason", func(g *Graph, _ *Builder) { g.Function.Name = Fact{} }},
		{"bad config", func(g *Graph, b *Builder) { g.Producer.Configuration = b.Text("[]") }},
		{"broken edge", func(g *Graph, b *Builder) {
			g.Edges = []Edge{{ID: b.Text("e"), Subkind: b.Text("operand"), Source: 1, Target: 2, Policy: g.Profile, Evidence: 1, Provenance: 1}}
		}},
		{"reason required", func(g *Graph, _ *Builder) { g.Conformance = []Conformance{{Profile: g.Profile, Status: NonConformant}} }},
		{"unknown extension preserved", func(g *Graph, b *Builder) {
			g.Extensions = []Extension{{Name: b.Text("not-namespaced"), Value: b.Text("{}")}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, b := fixture()
			tc.mutate(g, b)
			if Validate(g) == nil {
				t.Fatal("invalid IR accepted")
			}
		})
	}
}

func TestMemoryLocationAndParallelEdges(t *testing.T) {
	g, b := fixture()
	g.Locations = []Location{{ID: b.Text("heap"), Abstraction: b.Text("may-alias"), Description: b.Text("Possible target."), Evidence: b.Evidence(Approximated, "May alias.", g.Profile)}}
	g.Definitions = []Definition{{ID: b.Text("d1"), Node: 1, Location: 1}}
	attrs := []Attributes{{Defines: []Ref{1}, Writes: []Access{{Location: 1}}, Reads: []Access{{UnresolvedReason: b.Text("Unknown target.")}}}}
	g.Nodes[0].Attributes = 1
	if err := PackAttributes(g, attrs); err != nil {
		t.Fatal(err)
	}
	g.Edges = []Edge{
		{ID: b.Text("e1"), Source: 1, Target: 1, Kind: Data, Subkind: b.Text("may-def-use"), Location: 1, Definition: 1, Policy: g.Profile, Evidence: 1, Provenance: 1, Position: 1, LoopCarried: 2},
		{ID: b.Text("e2"), Source: 1, Target: 1, Kind: Data, Subkind: b.Text("may-def-use"), Location: 1, Definition: 1, Policy: g.Profile, Evidence: 1, Provenance: 1, Position: 2, LoopCarried: 2},
	}
	if err := Validate(g); err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 2 || g.Edges[0].Position == g.Edges[1].Position {
		t.Fatal("parallel operand edges collapsed")
	}
}

func TestSharedTablesAndRecordSize(t *testing.T) {
	b := NewBuilder()
	for range 10000 {
		b.Known("map[string]*example.Record")
		policy := b.Policy("extraction", "1")
		b.Evidence(Supported, "Typed source.", policy)
		b.Origin(SourceOrigin, "Source operation.", policy)
	}
	if len(b.Tables.Strings) != 5 || len(b.Tables.Policies) != 1 || len(b.Tables.Evidence) != 1 || len(b.Tables.Provenance) != 1 {
		t.Fatalf("metadata not shared: %+v", b.Tables)
	}
	if unsafe.Sizeof(Node{}) > 48 || unsafe.Sizeof(Edge{}) > 64 {
		t.Fatalf("hot records grew: node=%d edge=%d", unsafe.Sizeof(Node{}), unsafe.Sizeof(Edge{}))
	}
	t.Logf("node=%d edge=%d attributes=%d bytes", unsafe.Sizeof(Node{}), unsafe.Sizeof(Edge{}), unsafe.Sizeof(Attributes{}))
}

func TestExtensionsAndDetailedProvenance(t *testing.T) {
	g, b := fixture()
	g.Extensions = []Extension{{Name: b.Text("rstyle:ownership"), Value: b.Text(`{"future_field":[1,"unknown"]}`)}}
	g.Nodes[0].Provenance = b.AddProvenance(Provenance{Origin: Projection, Description: b.Text("Projected input."), Policy: g.Profile, Nodes: []Text{b.Text("external-node")}, Spans: []Span{{Source: 1, End: 3}}})
	if err := Validate(g); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.Tables.Text(g.Extensions[0].Value), "future_field") {
		t.Fatal("extension payload lost")
	}
}

func BenchmarkSharedMetadata(b *testing.B) {
	for b.Loop() {
		pool := NewBuilder()
		for range 10000 {
			pool.Known("map[string]*example.Record")
			policy := pool.Policy("extraction", "1")
			pool.Evidence(Supported, "Typed source.", policy)
			pool.Origin(SourceOrigin, "Source operation.", policy)
		}
	}
	b.ReportAllocs()
}

func TestPackedAttributesShareListsAndPreserveFacts(t *testing.T) {
	g, b := fixture()
	attrs := Attributes{
		Reads:    []Access{{Symbol: 1, Position: 1}},
		Operator: b.Known("+"), Outcomes: []Text{b.Text("true"), b.Text("false")},
		Extensions: []Extension{{Name: b.Text("future:fact"), Value: b.Text(`{"x":1}`)}},
	}
	g.Nodes = make([]Node, 10000)
	input := make([]Attributes, len(g.Nodes))
	for i := range input {
		input[i] = attrs
		g.Nodes[i].Attributes = Ref(i + 1)
	}
	if err := PackAttributes(g, input); err != nil {
		t.Fatal(err)
	}
	if len(g.Attributes) != 1 || len(g.AttributeTables.Accesses) != 1 || len(g.AttributeTables.Extensions) != 1 {
		t.Fatal("repeated attributes retain duplicate records")
	}
	got := g.NodeAttributes(g.Nodes[0])
	if got.Operator != attrs.Operator || len(got.Outcomes) != 2 || got.Outcomes[1] != attrs.Outcomes[1] || got.Extensions[0] != attrs.Extensions[0] || got.Reads[0] != attrs.Reads[0] {
		t.Fatal("packing loses facts")
	}
	retained := unsafe.Sizeof(Node{})*uintptr(len(g.Nodes)) + unsafe.Sizeof(AttributeSet{})*uintptr(len(g.Attributes)) + unsafe.Sizeof(Access{})
	if retained > 480000 {
		t.Fatalf("repeated-node storage grew: %d bytes", retained)
	}
	t.Logf("10,000 repeated nodes plus shared attributes: %d bytes (excludes source spans and string tables)", retained)
}

func BenchmarkPackedRepeatedNodes(b *testing.B) {
	for b.Loop() {
		g := &Graph{Nodes: make([]Node, 10000)}
		input := make([]Attributes, len(g.Nodes))
		for i := range input {
			input[i] = Attributes{Operator: Fact{Status: Known, Value: 1}}
			g.Nodes[i].Attributes = Ref(i + 1)
		}
		if err := PackAttributes(g, input); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(unsafe.Sizeof(Node{})*uintptr(len(g.Nodes))+unsafe.Sizeof(AttributeSet{})), "retained-bytes")
	}
	b.ReportAllocs()
}

func TestPackingRejectsBrokenConstructionReferences(t *testing.T) {
	g := &Graph{Nodes: []Node{{Attributes: 2}}}
	if PackAttributes(g, []Attributes{{}}) == nil {
		t.Fatal("broken construction reference accepted")
	}
	if len(g.Attributes) != 0 {
		t.Fatal("failed packing mutates graph")
	}
	if PackAttributes(nil, nil) == nil {
		t.Fatal("nil graph accepted")
	}
}

func TestCompactPreservesReferencesAndDropsConstructionCapacity(t *testing.T) {
	g := &Graph{Nodes: make([]Node, 1, 100), Edges: make([]Edge, 1, 100), Spans: make([]Span, 1, 100)}
	g.Nodes[0] = Node{ID: 7, Span: 1}
	g.Edges[0] = Edge{Source: 1, Target: 1, Position: 2}
	Compact(g)
	if cap(g.Nodes) != 1 || cap(g.Edges) != 1 || cap(g.Spans) != 1 {
		t.Fatal("construction capacity retained")
	}
	if g.Nodes[0].ID != 7 || g.Edges[0].Position != 2 || g.Edges[0].Target != 1 {
		t.Fatal("compaction loses values")
	}
}
