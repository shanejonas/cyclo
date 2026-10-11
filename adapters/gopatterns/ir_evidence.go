package gopatterns

import "github.com/shanejonas/cyclo/domain/pdg"

// Extraction completeness describes the actual producer, not what the IR can
// express. In particular CFG succession is not an execution dependency policy.
var irCapabilities = map[string]struct {
	status      pdg.Status
	description string
}{
	"node_extraction":             {pdg.Approximated, "Mixed statement/expression operations; closures are opaque; not a complete language lowering."},
	"scalar_reaching_definitions": {pdg.Unsupported, "Distinct syntactic definitions are recorded; mining edges use first bindings, not reaching definitions."},
	"branch_merges":               {pdg.Unsupported, "Merge values are not computed."},
	"loop_carried_dependencies":   {pdg.Unsupported, "Loop-carried definitions are not computed."},
	"control_dependencies":        {pdg.Approximated, "Lexical control nesting is approximated; post-dominator dependencies are not computed."},
	"execution_dependencies":      {pdg.Approximated, "Sparse must-precede edges between reachable statement/condition anchors, from sequential order and dominance; intrastatement and exceptional/concurrent ordering are incomplete."},
	"returns":                     {pdg.Approximated, "Explicit returns connect to formal output slots; bare returns read named results. Reaching definitions, deferred result writes and implicit or exceptional exits are not computed."},
	"calls":                       {pdg.Approximated, "Static declarations resolve where available; dynamic targets and call effects are unknown."},
	"parameters":                  {pdg.Supported, "Receiver, inputs, named/unnamed outputs, types and variadic facts are recorded."},
	"closures":                    {pdg.Unsupported, "Closure bodies and capture dependencies are not extracted."},
	"macros":                      {pdg.NotApplicable, "Go has no language macros."},
	"memory_aliases":              {pdg.Unknown, "Memory accesses are explicit; targets and aliases are unresolved."},
	"exceptional_flow":            {pdg.Unsupported, "Panic/recover dependencies are not computed."},
	"concurrency":                 {pdg.Unsupported, "Go/select/send operations are recorded; scheduling dependencies are not computed."},
	"reference_counting":          {pdg.Supported, "Distinct recorded Go reference-bearing variable bindings; type parameters remain unknown. Not alias or storage-location counting."},
}

func (b *irBuilder) capabilities() {
	for _, name := range pdg.RequiredCapabilities {
		c := irCapabilities[name]

		b.graph.Capabilities = append(b.graph.Capabilities, pdg.NamedCapability{
			Name: b.pool.Text(name), Evidence: b.pool.Evidence(c.status, c.description, b.policy),
		})
	}
	b.graph.Conformance = []pdg.Conformance{{
		Profile: b.policy, Status: pdg.NonConformant,
		Reasons: []pdg.Text{b.pool.Text("Dependency analysis and the shared extraction profile are incomplete.")},
	}}
	b.graph.Diagnostics = []pdg.Diagnostic{{
		Code: b.pool.Text("pdg.incomplete-dependencies"), Severity: pdg.Warning,
		Message: b.pool.Text("Operation facts are available; dependency absence must not be treated as independence."),
	}}
}
