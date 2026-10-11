// Package pdgjson exports compact native graphs as compat draft 0.1.0 documents.
package pdgjson

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/shanejonas/cyclo/domain/pdg"
)

type object = map[string]any

type exporter struct{ g *pdg.Graph }

// Write validates and writes one schema document. It does not change the graph.
// Only this function's expanded document is allocated, not an extraction-wide copy.
func Write(w io.Writer, g *pdg.Graph) error {
	if err := pdg.Validate(g); err != nil {
		return fmt.Errorf("export PDG: %w", err)
	}
	return json.NewEncoder(w).Encode(exporter{g}.document())
}

func (x exporter) text(t pdg.Text) string { return x.g.Tables.Text(t) }
func (x exporter) profile(r pdg.Ref) object {
	p := x.g.Tables.Policies[r-1]
	return object{"id": x.text(p.ID), "version": x.text(p.Version)}
}
func (x exporter) fact(f pdg.Fact) object {
	key := "reason"
	if f.Status == pdg.Known {
		key = "value"
	}
	return object{"status": statusNames[f.Status], key: x.text(f.Value)}
}

var statusNames = [...]string{"unknown", "known", "unsupported", "not_applicable", "supported", "approximated"}
var roleNames = [...]string{"parameter", "local", "global", "capture", "receiver", "output", "other", "return"}
var categoryNames = [...]string{"declaration", "read", "assignment", "computation", "control", "call", "entry", "formal_input", "return", "formal_output", "exit", "merge", "memory_summary", "other"}

func (x exporter) capability(r pdg.Ref) object {
	c := x.g.Tables.Evidence[r-1]
	out := object{"status": statusNames[c.Status], "description": x.text(c.Description)}
	x.optionalProfile(out, "policy", c.Policy)
	return out
}
func (x exporter) optionalProfile(out object, key string, r pdg.Ref) {
	if r != 0 {
		out[key] = x.profile(r)
	}
}
func (x exporter) span(s pdg.Span) object {
	return object{"source_id": x.text(x.g.Sources[s.Source-1].ID), "start": s.Start, "end": s.End}
}
func (x exporter) extensions(out object, key string, values []pdg.Extension) {
	if len(values) == 0 {
		return
	}
	data := object{}
	for _, e := range values {
		data[x.text(e.Name)] = json.RawMessage(x.text(e.Value))
	}
	out[key] = data
}
func (x exporter) optionalText(out object, key string, t pdg.Text) {
	if t != 0 {
		out[key] = x.text(t)
	}
}
func optionalPosition(out object, key string, p pdg.Position) {
	if p != 0 {
		out[key] = uint32(p) - 1
	}
}
func (x exporter) optionalFact(out object, key string, f pdg.Fact) {
	if f.Value != 0 {
		out[key] = x.fact(f)
	}
}
func mapItems[T any](items []T, convert func(T) object) []object {
	out := make([]object, len(items))
	for i, item := range items {
		out[i] = convert(item)
	}
	return out
}
func (x exporter) provenance(r pdg.Ref) object {
	p := x.g.Tables.Provenance[r-1]
	origins := [...]string{"source", "desugaring", "macro_expansion", "analysis", "projection"}
	out := object{"origin": origins[p.Origin], "description": x.text(p.Description)}
	x.optionalProfile(out, "policy", p.Policy)
	if len(p.Nodes) > 0 {
		out["origin_node_ids"] = x.texts(p.Nodes)
	}
	if len(p.Spans) > 0 {
		out["origin_spans"] = mapItems(p.Spans, x.span)
	}
	return out
}
func (x exporter) texts(items []pdg.Text) []string {
	out := make([]string, len(items))
	for i, t := range items {
		out[i] = x.text(t)
	}
	return out
}
func (x exporter) document() object {
	out := object{
		"schema_version": pdg.SchemaVersion, "profile": x.profile(x.g.Profile),
		"producer": x.producer(), "sources": mapItems(x.g.Sources, x.source),
		"function": x.function(), "symbols": mapItems(x.g.Symbols, x.symbol),
		"definitions":      mapItems(x.g.Definitions, x.definition),
		"memory_locations": mapItems(x.g.Locations, x.location),
		"nodes":            mapItems(x.g.Nodes, x.node), "edges": mapItems(x.g.Edges, x.edge),
		"extraction_evidence": x.extractionEvidence(),
	}
	x.extensions(out, "extensions", x.g.Extensions)
	return out
}
