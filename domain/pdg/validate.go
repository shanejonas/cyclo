package pdg

import (
	"encoding/json"
	"fmt"
	"regexp"
)

var extensionName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*:[a-zA-Z0-9_.-]+$`)

type validator struct {
	g   *Graph
	err error
}

// Validate checks the IR's structure and local references. It does not certify
// extraction correctness or profile conformance; those require source fixtures.
func Validate(g *Graph) error {
	if g == nil || g.Tables == nil {
		return fmt.Errorf("PDG tables are required")
	}
	v := validator{g: g}
	v.tables()
	v.header()
	v.sources()
	v.symbols()
	v.definitions()
	v.locations()
	v.nodes()
	v.edges()
	v.evidence()
	return v.err
}

func (v *validator) require(ok bool, message string) {
	if !ok && v.err == nil {
		v.err = fmt.Errorf("invalid PDG: %s", message)
	}
}

func (v *validator) text(id Text, required bool) {
	v.require(uint64(id) <= uint64(len(v.g.Tables.Strings)), "text reference out of bounds")
	if required {
		v.require(v.g.Tables.Text(id) != "", "nonempty text is required")
	}
}

func (v *validator) ref(id Ref, length int, required bool) {
	v.require(uint64(id) <= uint64(length), "reference out of bounds")
	if required {
		v.require(id != 0, "reference is required")
	}
}

func (v *validator) fact(f Fact) {
	v.text(f.Value, f.Status != Known)
	v.require(f.Value != 0, "fact value or reason is required")
	v.require(f.Status <= NotApplicable, "invalid fact status")
}

func (v *validator) unique(ids []Text) {
	seen := make(map[Text]bool, len(ids))
	for _, id := range ids {
		v.text(id, true)
		v.require(!seen[id], "duplicate ID in namespace")
		seen[id] = true
	}
}

func (v *validator) tables() {
	v.matchLabels()
	for _, p := range v.g.Tables.Policies {
		v.text(p.ID, true)
		v.text(p.Version, true)
	}
	for _, c := range v.g.Tables.Evidence {
		v.text(c.Description, true)
		v.ref(c.Policy, len(v.g.Tables.Policies), false)
		v.require(c.Status >= Unsupported && c.Status <= Approximated || c.Status == Unknown, "invalid capability status")
	}
	for _, p := range v.g.Tables.Provenance {
		v.provenance(p)
	}
}

func (v *validator) provenance(p Provenance) {
	v.text(p.Description, false)
	v.require(p.Description != 0, "provenance description is required")
	v.require(p.Origin <= Projection, "invalid provenance origin")
	v.ref(p.Policy, len(v.g.Tables.Policies), false)
	for _, id := range p.Nodes {
		v.text(id, true)
	}
	// Shared provenance spans use the current document's source namespace.
	for _, span := range p.Spans {
		v.span(span)
	}
}

func (v *validator) header() {
	g := v.g
	v.ref(g.Profile, len(g.Tables.Policies), true)
	v.text(g.Producer.Name, true)
	v.text(g.Producer.Version, true)
	v.text(g.Producer.Language, true)
	v.fact(g.Producer.LanguageVersion)
	v.jsonObject(g.Producer.Configuration)
	v.function()
	v.extensions(g.Extensions)
}

func (v *validator) jsonObject(id Text) {
	v.text(id, true)
	var object map[string]json.RawMessage
	err := json.Unmarshal([]byte(v.g.Tables.Text(id)), &object)
	v.require(err == nil && object != nil, "configuration must be a JSON object")
}

func (v *validator) function() {
	f := v.g.Function
	v.text(f.ID, true)
	v.fact(f.Name)
	v.fact(f.QualifiedName)
	v.span(f.Span)
	v.ref(f.Interface, len(v.g.Tables.Evidence), true)
	v.parameters(f.Inputs)
	v.parameters(f.Outputs)
	v.count(f.ReferenceCount)
}

func (v *validator) count(c CountFact) {
	v.require(c.Status <= NotApplicable, "invalid count status")
	if c.Status == Known {
		v.ref(c.Policy, len(v.g.Tables.Policies), true)
		return
	}
	v.text(c.Reason, true)
}

func (v *validator) parameters(params []Parameter) {
	positions := make(map[uint32]bool, len(params))
	for _, p := range params {
		v.require(!positions[p.Position], "duplicate interface position")
		positions[p.Position] = true
		v.parameter(p)
	}
}

func (v *validator) parameter(p Parameter) {
	v.fact(p.Name)
	v.fact(p.Type)
	v.fact(p.Variadic)
	v.ref(p.Symbol, len(v.g.Symbols), false)
	v.require(p.Role == ParameterRole || p.Role == ReceiverRole || p.Role == CaptureRole || p.Role == ReturnRole || p.Role == OutputRole, "invalid parameter role")
	v.extensions(p.Extensions)
}

func (v *validator) sources() {
	ids := make([]Text, 0, len(v.g.Sources))
	for _, s := range v.g.Sources {
		ids = append(ids, s.ID)
		v.text(s.Path, true)
		v.fact(s.ContentIdentity)
	}
	v.unique(ids)
	for _, span := range v.g.Spans {
		v.span(span)
	}
}

func (v *validator) span(s Span) {
	v.ref(Ref(s.Source), len(v.g.Sources), true)
	v.require(s.Start <= s.End, "span start exceeds end")
}

func (v *validator) symbols() {
	ids := make([]Text, 0, len(v.g.Symbols))
	for _, s := range v.g.Symbols {
		ids = append(ids, s.ID)
		v.symbol(s)
	}
	v.unique(ids)
}

func (v *validator) symbol(s Symbol) {
	v.text(s.Scope, true)
	v.fact(s.Name)
	v.fact(s.Type)
	v.require(s.Role <= OtherRole, "invalid symbol role")
	v.extensions(s.Extensions)
}

func (v *validator) definitions() {
	ids := make([]Text, 0, len(v.g.Definitions))
	for i, d := range v.g.Definitions {
		ids = append(ids, d.ID)
		v.definition(d, Ref(i+1))
	}
	v.unique(ids)
}

func (v *validator) definition(d Definition, id Ref) {
	v.ref(d.Node, len(v.g.Nodes), true)
	v.ref(d.Symbol, len(v.g.Symbols), false)
	v.ref(d.Location, len(v.g.Locations), false)
	v.require(d.Symbol != 0 || d.Location != 0, "definition target is required")
	if d.Node == 0 || uint64(d.Node) > uint64(len(v.g.Nodes)) {
		return
	}
	v.definitionOwner(d, id)
}

func (v *validator) definitionOwner(d Definition, id Ref) {
	n := v.g.Nodes[d.Node-1]
	if n.Attributes == 0 || uint64(n.Attributes) > uint64(len(v.g.Attributes)) {
		v.require(false, "definition owner has no attributes")
		return
	}
	v.require(contains(v.g.NodeAttributes(n).Defines, id), "definition missing from owner")
}

func contains(ids []Ref, target Ref) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func (v *validator) locations() {
	ids := make([]Text, 0, len(v.g.Locations))
	for _, l := range v.g.Locations {
		ids = append(ids, l.ID)
		v.text(l.Abstraction, true)
		v.text(l.Description, false)
		v.require(l.Description != 0, "location description is required")
		v.ref(l.Evidence, len(v.g.Tables.Evidence), true)
		v.extensions(l.Extensions)
	}
	v.unique(ids)
}

func (v *validator) nodes() {
	ids := make([]Text, 0, len(v.g.Nodes))
	for _, n := range v.g.Nodes {
		ids = append(ids, n.ID)
		v.node(n)
	}
	v.unique(ids)
	for _, a := range v.g.Attributes {
		v.attributeSet(a)
	}
	for _, annotations := range v.g.Annotations {
		v.extensions(annotations)
	}
}

func (v *validator) node(n Node) {
	v.ref(n.MatchLabel, len(v.g.Tables.MatchLabels), false)
	v.require(n.Category <= Other, "invalid node category")
	v.fact(n.OriginalKind)
	v.ref(n.Span, len(v.g.Spans), !n.Synthetic)
	v.ref(n.Attributes, len(v.g.Attributes), false)
	v.ref(n.Evidence, len(v.g.Tables.Evidence), true)
	v.ref(n.Provenance, len(v.g.Tables.Provenance), true)
	v.ref(n.Annotations, len(v.g.Annotations), false)
	v.nodeDefinitions(n)
	if n.Category == Other {
		v.other(n)
	}
}

func (v *validator) other(n Node) {
	if n.Attributes == 0 || uint64(n.Attributes) > uint64(len(v.g.Attributes)) {
		v.require(false, "other node needs a description")
		return
	}
	v.text(v.g.Attributes[n.Attributes-1].Description, true)
}

func (v *validator) attributes(a Attributes) {
	v.refs(a.Symbols, len(v.g.Symbols))
	v.refs(a.Defines, len(v.g.Definitions))
	v.accesses(a.Reads)
	v.accesses(a.Writes)
	v.optionalFacts(a.Operator, a.Literal, a.Callee)
	v.attributeTexts(a)
	v.require(a.CallForm <= UnknownCall, "invalid call form")
	v.require(a.ExitKind <= OtherExit, "invalid exit kind")
	v.extensions(a.Extensions)
}

func (v *validator) attributeTexts(a Attributes) {
	v.text(a.Construct, a.Construct != 0)
	v.text(a.WriteForm, a.WriteForm != 0)
	v.text(a.Description, a.Description != 0)
	for _, outcome := range a.Outcomes {
		v.text(outcome, true)
	}
}

func (v *validator) optionalFacts(facts ...Fact) {
	for _, f := range facts {
		if f.Value != 0 {
			v.fact(f)
		}
	}
}

func (v *validator) refs(ids []Ref, length int) {
	for _, id := range ids {
		v.ref(id, length, true)
	}
}

func (v *validator) accesses(accesses []Access) {
	for _, a := range accesses {
		v.ref(a.Symbol, len(v.g.Symbols), false)
		v.ref(a.Location, len(v.g.Locations), false)
		v.text(a.UnresolvedReason, a.UnresolvedReason != 0)
		v.require(a.Symbol != 0 || a.Location != 0 || a.UnresolvedReason != 0, "access target or reason is required")
	}
}

func (v *validator) edges() {
	ids := make([]Text, 0, len(v.g.Edges))
	for _, e := range v.g.Edges {
		ids = append(ids, e.ID)
		v.edge(e)
	}
	v.unique(ids)
}

func (v *validator) edge(e Edge) {
	v.text(e.Subkind, true)
	v.ref(e.Source, len(v.g.Nodes), true)
	v.ref(e.Target, len(v.g.Nodes), true)
	v.ref(e.Policy, len(v.g.Tables.Policies), true)
	v.ref(e.Evidence, len(v.g.Tables.Evidence), true)
	v.ref(e.Provenance, len(v.g.Tables.Provenance), true)
	v.ref(e.Annotations, len(v.g.Annotations), false)
	v.edgeAttributes(e)
	v.require(e.Kind <= Execution, "invalid edge kind")
	v.require(e.LoopCarried <= 2, "invalid optional loop-carried value")
}

func (v *validator) edgeAttributes(e Edge) {
	v.ref(e.Symbol, len(v.g.Symbols), false)
	v.ref(e.Definition, len(v.g.Definitions), false)
	v.ref(e.Location, len(v.g.Locations), false)
	v.text(e.Outcome, e.Outcome != 0)
}

func (v *validator) evidence() {
	names := make(map[string]bool, len(v.g.Capabilities))
	for _, c := range v.g.Capabilities {
		v.text(c.Name, true)
		v.ref(c.Evidence, len(v.g.Tables.Evidence), true)
		name := v.g.Tables.Text(c.Name)
		v.require(!names[name], "duplicate capability")
		names[name] = true
	}
	for _, name := range RequiredCapabilities {
		v.require(names[name], "required capability is missing")
	}
	v.results()
}

func (v *validator) results() {
	for _, d := range v.g.Diagnostics {
		v.diagnostic(d)
	}
	for _, c := range v.g.Conformance {
		v.ref(c.Profile, len(v.g.Tables.Policies), true)
		v.require(c.Status <= NonConformant, "invalid conformance status")
		v.require(c.Status != NonConformant || len(c.Reasons) > 0, "non-conformance needs a reason")
		for _, reason := range c.Reasons {
			v.text(reason, true)
		}
	}
}

func (v *validator) diagnostic(d Diagnostic) {
	v.text(d.Code, true)
	v.text(d.Message, true)
	v.text(d.Capability, d.Capability != 0)
	v.ref(d.Span, len(v.g.Spans), false)
	v.ref(d.Node, len(v.g.Nodes), false)
	v.require(d.Severity <= Error, "invalid diagnostic severity")
}

func (v *validator) extensions(extensions []Extension) {
	for _, e := range extensions {
		v.require(extensionName.MatchString(v.g.Tables.Text(e.Name)), "extension name must be namespaced")
		v.text(e.Value, true)
		v.require(json.Valid([]byte(v.g.Tables.Text(e.Value))), "extension value must be valid JSON")
	}
}

func (v *validator) attributeSet(a AttributeSet) {
	v.ref(a.Symbols, len(v.g.AttributeTables.References), false)
	v.ref(a.Defines, len(v.g.AttributeTables.References), false)
	v.ref(a.Outcomes, len(v.g.AttributeTables.References), false)
	v.ref(a.Reads, len(v.g.AttributeTables.Accesses), false)
	v.ref(a.Writes, len(v.g.AttributeTables.Accesses), false)
	v.ref(a.Extensions, len(v.g.AttributeTables.Extensions), false)
	v.attributes(v.g.expand(a))
	for _, value := range listAt(v.g.AttributeTables.References, a.Outcomes) {
		v.text(Text(value), true)
	}
}

func (v *validator) nodeDefinitions(n Node) {
	for _, id := range v.g.NodeAttributes(n).Defines {
		if id == 0 || uint64(id) > uint64(len(v.g.Definitions)) {
			continue
		}
		owner := v.g.Definitions[id-1].Node
		if owner == 0 || uint64(owner) > uint64(len(v.g.Nodes)) {
			continue
		}
		v.require(v.g.Nodes[owner-1].ID == n.ID, "node claims another node's definition")
	}
}

func (v *validator) matchLabels() {
	for _, label := range v.g.Tables.MatchLabels {
		v.text(label.Kind, true)
		for _, id := range []Text{label.TypeClass, label.SignatureClass, label.Callee, label.LiteralKind, label.Detail} {
			v.text(id, false)
		}
	}
}
