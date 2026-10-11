package pdg

import (
	"encoding/binary"
	"fmt"
)

// AttributeSet stores optional lists as indexes, and is comparable so repeated
// attribute objects can share one record. Empty lists always use index zero.
type AttributeSet struct {
	Symbols, Defines, Reads, Writes, Outcomes, Extensions Ref
	Operator, Literal, Callee                             Fact
	Construct, WriteForm, Description                     Text
	Position                                              Position
	CallForm                                              CallForm
	ExitKind                                              ExitKind
}

// AttributeTables are graph-local because bindings and definitions are local.
type AttributeTables struct {
	References [][]Ref
	Accesses   [][]Access
	Extensions [][]Extension
}

type attributeBuilder struct {
	g          *Graph
	refs       map[string]Ref
	accesses   map[string]Ref
	extensions map[string]Ref
	sets       map[AttributeSet]Ref
}

// PackAttributes replaces construction attributes with deduplicated records and
// lists. Nodes initially reference the input slice; their indexes are remapped.
// Construction maps and the original records are not retained by the graph.
func PackAttributes(g *Graph, input []Attributes) error {
	if err := attributeInput(g, input); err != nil {
		return err
	}
	b := attributeBuilder{g: g, refs: map[string]Ref{}, accesses: map[string]Ref{}, extensions: map[string]Ref{}, sets: map[AttributeSet]Ref{}}
	mapping := make([]Ref, len(input)+1)
	for i, attrs := range input {
		mapping[i+1] = b.pack(attrs)
	}
	for i := range g.Nodes {
		g.Nodes[i].Attributes = mapping[g.Nodes[i].Attributes]
	}
	return nil
}

func (b *attributeBuilder) pack(a Attributes) Ref {
	set := AttributeSet{
		Symbols: b.references(a.Symbols), Defines: b.references(a.Defines),
		Reads: b.accessList(a.Reads), Writes: b.accessList(a.Writes),
		Outcomes: b.textList(a.Outcomes), Extensions: b.extensionList(a.Extensions),
		Operator: a.Operator, Literal: a.Literal, Callee: a.Callee,
		Construct: a.Construct, WriteForm: a.WriteForm, Description: a.Description,
		Position: a.Position, CallForm: a.CallForm, ExitKind: a.ExitKind,
	}
	if set == (AttributeSet{}) {
		return 0
	}
	if id, ok := b.sets[set]; ok {
		return id
	}
	b.g.Attributes = append(b.g.Attributes, set)
	id := Ref(len(b.g.Attributes))
	b.sets[set] = id
	return id
}

func (b *attributeBuilder) references(list []Ref) Ref {
	if len(list) == 0 {
		return 0
	}
	key := referenceKey(list)
	if id, ok := b.refs[key]; ok {
		return id
	}
	b.g.AttributeTables.References = append(b.g.AttributeTables.References, append([]Ref(nil), list...))
	id := Ref(len(b.g.AttributeTables.References))
	b.refs[key] = id
	return id
}

func referenceKey(list []Ref) string {
	data := make([]byte, 0, len(list)*4)
	for _, value := range list {
		data = binary.LittleEndian.AppendUint32(data, uint32(value))
	}
	return string(data)
}

func (b *attributeBuilder) textList(list []Text) Ref {
	refs := make([]Ref, len(list))
	for i, value := range list {
		refs[i] = Ref(value)
	}
	return b.references(refs)
}

func (b *attributeBuilder) accessList(list []Access) Ref {
	if len(list) == 0 {
		return 0
	}
	key := accessKey(list)
	if id, ok := b.accesses[key]; ok {
		return id
	}
	b.g.AttributeTables.Accesses = append(b.g.AttributeTables.Accesses, append([]Access(nil), list...))
	id := Ref(len(b.g.AttributeTables.Accesses))
	b.accesses[key] = id
	return id
}

func accessKey(list []Access) string {
	data := make([]byte, 0, len(list)*16)
	for _, value := range list {
		data = binary.LittleEndian.AppendUint32(data, uint32(value.Symbol))
		data = binary.LittleEndian.AppendUint32(data, uint32(value.Location))
		data = binary.LittleEndian.AppendUint32(data, uint32(value.UnresolvedReason))
		data = binary.LittleEndian.AppendUint32(data, uint32(value.Position))
	}
	return string(data)
}

func (b *attributeBuilder) extensionList(list []Extension) Ref {
	if len(list) == 0 {
		return 0
	}
	refs := make([]Ref, 0, len(list)*2)
	for _, value := range list {
		refs = append(refs, Ref(value.Name), Ref(value.Value))
	}
	key := referenceKey(refs)
	if id, ok := b.extensions[key]; ok {
		return id
	}
	b.g.AttributeTables.Extensions = append(b.g.AttributeTables.Extensions, append([]Extension(nil), list...))
	id := Ref(len(b.g.AttributeTables.Extensions))
	b.extensions[key] = id
	return id
}

// NodeAttributes returns a view over shared lists. Treat returned lists as
// immutable; changing one can affect other nodes using the same record.
func (g *Graph) NodeAttributes(node Node) Attributes {
	if node.Attributes == 0 || uint64(node.Attributes) > uint64(len(g.Attributes)) {
		return Attributes{}
	}
	return g.expand(g.Attributes[node.Attributes-1])
}

func (g *Graph) expand(a AttributeSet) Attributes {
	return Attributes{
		Symbols: listAt(g.AttributeTables.References, a.Symbols), Defines: listAt(g.AttributeTables.References, a.Defines),
		Reads: listAt(g.AttributeTables.Accesses, a.Reads), Writes: listAt(g.AttributeTables.Accesses, a.Writes),
		Operator: a.Operator, Literal: a.Literal, Callee: a.Callee,
		Construct: a.Construct, WriteForm: a.WriteForm, Description: a.Description,
		Position: a.Position, CallForm: a.CallForm, ExitKind: a.ExitKind,
		Extensions: listAt(g.AttributeTables.Extensions, a.Extensions),
		Outcomes:   g.outcomes(a.Outcomes),
	}
}

func listAt[T any](lists [][]T, ref Ref) []T {
	if ref == 0 || uint64(ref) > uint64(len(lists)) {
		return nil
	}
	return lists[ref-1]
}

func (g *Graph) outcomes(ref Ref) []Text {
	refs := listAt(g.AttributeTables.References, ref)
	if len(refs) == 0 {
		return nil
	}
	texts := make([]Text, len(refs))
	for i, value := range refs {
		texts[i] = Text(value)
	}
	return texts
}

func attributeInput(g *Graph, input []Attributes) error {
	if g == nil {
		return fmt.Errorf("PDG graph is required")
	}
	if len(g.Attributes) != 0 {
		return fmt.Errorf("PDG attributes are already packed")
	}
	for _, node := range g.Nodes {
		if uint64(node.Attributes) > uint64(len(input)) {
			return fmt.Errorf("PDG construction attribute reference out of bounds")
		}
	}
	return nil
}
