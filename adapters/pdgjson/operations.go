package pdgjson

import "github.com/shanejonas/cyclo/domain/pdg"

func (x exporter) node(n pdg.Node) object {
	out := object{"id": x.text(n.ID), "category": categoryNames[n.Category], "original_kind": x.fact(n.OriginalKind), "synthetic": n.Synthetic, "attributes": x.attributes(x.g.NodeAttributes(n)), "provenance": x.provenance(n.Provenance), "evidence": x.capability(n.Evidence)}
	x.optionalSpan(out, n.Span)
	x.annotations(out, n.Annotations)
	return out
}
func (x exporter) optionalSpan(out object, r pdg.Ref) {
	if r != 0 {
		out["source_span"] = x.span(x.g.Spans[r-1])
	}
}
func (x exporter) annotations(out object, r pdg.Ref) {
	if r != 0 {
		x.extensions(out, "annotations", x.g.Annotations[r-1])
	}
}
func (x exporter) attributes(a pdg.Attributes) object {
	out := object{}
	x.attributeLists(out, a)
	x.attributeFacts(out, a)
	x.optionalText(out, "construct", a.Construct)
	x.optionalText(out, "write_form", a.WriteForm)
	x.optionalText(out, "description", a.Description)
	optionalPosition(out, "position", a.Position)
	if a.CallForm != 0 {
		out["call_form"] = [...]string{"", "direct", "indirect", "unknown"}[a.CallForm]
	}
	if a.ExitKind != 0 {
		out["exit_kind"] = [...]string{"", "normal", "exceptional", "other"}[a.ExitKind]
	}
	x.extensions(out, "extensions", a.Extensions)
	return out
}
func (x exporter) attributeLists(out object, a pdg.Attributes) {
	if len(a.Symbols) > 0 {
		out["symbol_ids"] = x.symbolIDs(a.Symbols)
	}
	if len(a.Defines) > 0 {
		out["defines"] = x.definitionIDs(a.Defines)
	}
	if len(a.Reads) > 0 {
		out["reads"] = mapItems(a.Reads, x.access)
	}
	if len(a.Writes) > 0 {
		out["writes"] = mapItems(a.Writes, x.access)
	}
	if len(a.Outcomes) > 0 {
		out["outcomes"] = x.texts(a.Outcomes)
	}
}
func (x exporter) attributeFacts(out object, a pdg.Attributes) {
	x.optionalFact(out, "operator", a.Operator)
	x.optionalFact(out, "literal", a.Literal)
	x.optionalFact(out, "callee", a.Callee)
}
func (x exporter) symbolIDs(refs []pdg.Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = x.text(x.g.Symbols[r-1].ID)
	}
	return out
}
func (x exporter) definitionIDs(refs []pdg.Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = x.text(x.g.Definitions[r-1].ID)
	}
	return out
}
func (x exporter) access(a pdg.Access) object {
	out := object{}
	x.optionalSymbol(out, a.Symbol)
	x.optionalLocation(out, a.Location)
	x.optionalText(out, "unresolved_reason", a.UnresolvedReason)
	optionalPosition(out, "operand_position", a.Position)
	return out
}
func (x exporter) edge(e pdg.Edge) object {
	kinds := [...]string{"data", "control", "execution"}
	out := object{"id": x.text(e.ID), "source": x.text(x.g.Nodes[e.Source-1].ID), "target": x.text(x.g.Nodes[e.Target-1].ID), "kind": kinds[e.Kind], "subkind": x.text(e.Subkind), "policy": x.profile(e.Policy), "evidence": x.capability(e.Evidence), "provenance": x.provenance(e.Provenance)}
	x.edgeFacts(out, e)
	x.annotations(out, e.Annotations)
	return out
}
func (x exporter) edgeFacts(out object, e pdg.Edge) {
	x.optionalSymbol(out, e.Symbol)
	x.optionalLocation(out, e.Location)
	if e.Definition != 0 {
		out["definition_id"] = x.text(x.g.Definitions[e.Definition-1].ID)
	}
	x.optionalText(out, "outcome", e.Outcome)
	optionalPosition(out, "operand_position", e.Position)
	if e.LoopCarried != 0 {
		out["loop_carried"] = e.LoopCarried == 2
	}
}
