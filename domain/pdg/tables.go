package pdg

import "strings"

// Tables retains only unique values. The construction maps belong to Builder
// and are released when that builder is discarded. No compiler objects remain.
type Tables struct {
	MatchLabels []MatchLabel
	Strings     []string
	Policies    []Profile
	Evidence    []Capability
	Provenance  []Provenance
}

func (t *Tables) Text(id Text) string {
	if id == 0 || uint64(id) > uint64(len(t.Strings)) {
		return ""
	}
	return t.Strings[id-1]
}

// Builder owns interning maps for one extraction. Do not use it concurrently.
type Builder struct {
	labels   map[MatchLabel]Ref
	Tables   *Tables
	strings  map[string]Text
	policies map[Profile]Ref
	evidence map[Capability]Ref
	origins  map[provenanceKey]Ref
}

type provenanceKey struct {
	Description Text
	Policy      Ref
	Origin      Origin
}

func NewBuilder() *Builder {
	return &Builder{
		Tables: &Tables{}, labels: map[MatchLabel]Ref{}, strings: map[string]Text{},
		policies: map[Profile]Ref{}, evidence: map[Capability]Ref{},
		origins: map[provenanceKey]Ref{},
	}
}

func (b *Builder) Text(value string) Text {
	if id, ok := b.strings[value]; ok {
		return id
	}
	value = strings.Clone(value)
	b.Tables.Strings = append(b.Tables.Strings, value)
	id := Text(len(b.Tables.Strings))
	b.strings[value] = id
	return id
}

func (b *Builder) Known(value string) Fact {
	return Fact{Status: Known, Value: b.Text(value)}
}

func (b *Builder) Missing(status Status, reason string) Fact {
	return Fact{Status: status, Value: b.Text(reason)}
}

func (b *Builder) Policy(id, version string) Ref {
	p := Profile{ID: b.Text(id), Version: b.Text(version)}
	if ref, ok := b.policies[p]; ok {
		return ref
	}
	b.Tables.Policies = append(b.Tables.Policies, p)
	ref := Ref(len(b.Tables.Policies))
	b.policies[p] = ref
	return ref
}

func (b *Builder) Evidence(status Status, description string, policy Ref) Ref {
	c := Capability{Status: status, Description: b.Text(description), Policy: policy}
	if ref, ok := b.evidence[c]; ok {
		return ref
	}
	b.Tables.Evidence = append(b.Tables.Evidence, c)
	ref := Ref(len(b.Tables.Evidence))
	b.evidence[c] = ref
	return ref
}

// Origin interns provenance without per-item lists. Detailed transformation
// provenance can be appended separately with AddProvenance.
func (b *Builder) Origin(origin Origin, description string, policy Ref) Ref {
	key := provenanceKey{Origin: origin, Description: b.Text(description), Policy: policy}
	if ref, ok := b.origins[key]; ok {
		return ref
	}
	ref := b.AddProvenance(Provenance{Origin: origin, Description: key.Description, Policy: policy})
	b.origins[key] = ref
	return ref
}

func (b *Builder) AddProvenance(p Provenance) Ref {
	b.Tables.Provenance = append(b.Tables.Provenance, p)
	return Ref(len(b.Tables.Provenance))
}

var RequiredCapabilities = [...]string{
	"node_extraction", "scalar_reaching_definitions", "branch_merges",
	"loop_carried_dependencies", "control_dependencies", "execution_dependencies",
	"returns", "calls", "parameters", "closures", "macros", "memory_aliases",
	"exceptional_flow", "concurrency", "reference_counting",
}

func (b *Builder) MatchLabel(label MatchLabel) Ref {
	if ref, ok := b.labels[label]; ok {
		return ref
	}
	b.Tables.MatchLabels = append(b.Tables.MatchLabels, label)
	ref := Ref(len(b.Tables.MatchLabels))
	b.labels[label] = ref
	return ref
}
