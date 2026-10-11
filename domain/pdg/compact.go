package pdg

// Compact drops spare construction capacity before publishing a graph.
// It preserves order, references and shared values.
func Compact(g *Graph) {
	g.Nodes = compactSlice(g.Nodes)
	g.Edges = compactSlice(g.Edges)
	g.Spans = compactSlice(g.Spans)
	g.Symbols = compactSlice(g.Symbols)
	g.Definitions = compactSlice(g.Definitions)
	g.Attributes = compactSlice(g.Attributes)
	g.AttributeTables.References = compactSlice(g.AttributeTables.References)
	g.AttributeTables.Accesses = compactSlice(g.AttributeTables.Accesses)
	g.AttributeTables.Extensions = compactSlice(g.AttributeTables.Extensions)
}

func compactSlice[T any](values []T) []T {
	if cap(values) <= len(values)+len(values)/8 {
		return values
	}
	out := make([]T, len(values))
	copy(out, values)
	return out
}
