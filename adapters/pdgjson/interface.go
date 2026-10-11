package pdgjson

import (
	"encoding/json"
	"github.com/shanejonas/cyclo/domain/pdg"
)

func (x exporter) producer() object {
	p := x.g.Producer
	return object{"name": x.text(p.Name), "version": x.text(p.Version), "language": x.text(p.Language), "language_version": x.fact(p.LanguageVersion), "configuration": json.RawMessage(x.text(p.Configuration))}
}
func (x exporter) source(s pdg.Source) object {
	return object{"id": x.text(s.ID), "path": x.text(s.Path), "content_identity": x.fact(s.ContentIdentity)}
}
func (x exporter) function() object {
	f := x.g.Function
	return object{"id": x.text(f.ID), "name": x.fact(f.Name), "qualified_name": x.fact(f.QualifiedName), "source_span": x.span(f.Span), "inputs": mapItems(f.Inputs, x.parameter), "outputs": mapItems(f.Outputs, x.parameter), "interface_completeness": x.capability(f.Interface), "reference_count": x.count(f.ReferenceCount)}
}
func (x exporter) count(c pdg.CountFact) object {
	if c.Status != pdg.Known {
		return object{"status": statusNames[c.Status], "reason": x.text(c.Reason)}
	}
	return object{"status": "known", "value": c.Value, "policy": x.profile(c.Policy)}
}
func (x exporter) parameter(p pdg.Parameter) object {
	out := object{"position": p.Position, "role": roleNames[p.Role], "name": x.fact(p.Name), "type": x.fact(p.Type), "variadic": x.fact(p.Variadic)}
	x.optionalSymbol(out, p.Symbol)
	x.extensions(out, "extensions", p.Extensions)
	return out
}
func (x exporter) symbol(s pdg.Symbol) object {
	out := object{"id": x.text(s.ID), "name": x.fact(s.Name), "scope_id": x.text(s.Scope), "role": roleNames[s.Role], "type": x.fact(s.Type)}
	x.extensions(out, "extensions", s.Extensions)
	return out
}
func (x exporter) optionalSymbol(out object, r pdg.Ref) {
	if r != 0 {
		out["symbol_id"] = x.text(x.g.Symbols[r-1].ID)
	}
}
func (x exporter) optionalLocation(out object, r pdg.Ref) {
	if r != 0 {
		out["location_id"] = x.text(x.g.Locations[r-1].ID)
	}
}
func (x exporter) definition(d pdg.Definition) object {
	out := object{"id": x.text(d.ID), "node_id": x.text(x.g.Nodes[d.Node-1].ID)}
	x.optionalSymbol(out, d.Symbol)
	x.optionalLocation(out, d.Location)
	return out
}
func (x exporter) location(l pdg.Location) object {
	out := object{"id": x.text(l.ID), "abstraction": x.text(l.Abstraction), "description": x.text(l.Description), "evidence": x.capability(l.Evidence)}
	x.extensions(out, "extensions", l.Extensions)
	return out
}
