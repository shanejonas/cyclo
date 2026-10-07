// Package quality evaluates plain function facts without IO or compiler types.
package quality

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Name   string `json:"name"`
}

// Helper retains policy-free body evidence for one statically called local
// function. Helpers form a flat graph; cycles remain unknown during evaluation.
type Helper struct {
	Name      string     `json:"name"`
	Mutations []Mutation `json:"mutations"`
	Calls     []Call     `json:"calls"`
	Effects   []Effect   `json:"effects"`
}

type Function struct {
	Helpers []Helper `json:"helpers,omitempty"`
	Location
	Params        int        `json:"params"`
	HasSelf       bool       `json:"has_self"`
	Statements    int        `json:"statements"`
	CodeLines     int        `json:"code_lines"`
	Source        string     `json:"source"`
	PrecedingLine string     `json:"preceding_line"`
	Mutations     []Mutation `json:"mutations"`
	Calls         []Call     `json:"calls"`
	Effects       []Effect   `json:"effects"`
}

// Provenance concerns observable state, not allocation on the Go heap.
type Provenance string

const (
	Local    Provenance = "local"
	External Provenance = "external"
	Unknown  Provenance = "unknown"
)

type Mutation struct {
	Root       string     `json:"root"`
	RootID     string     `json:"root_id"`
	FieldPath  string     `json:"field_path"`
	Line       int        `json:"line"`
	Provenance Provenance `json:"provenance"`
}

type Call struct {
	Callee  string `json:"callee"`
	Line    int    `json:"line"`
	Dynamic bool   `json:"dynamic"`
	Local   bool   `json:"local"`
}

type Kind string

const (
	MutationEffect Kind = "mutation"
	IO             Kind = "io"
	Network        Kind = "network"
	Global         Kind = "global"
	Unsafe         Kind = "unsafe"
	Time           Kind = "time"
	Random         Kind = "random"
	Panic          Kind = "panic"
	UnknownEffect  Kind = "unknown"
	None           Kind = "none"
)

var kinds = []Kind{MutationEffect, IO, Network, Global, Unsafe, Time, Random, Panic, UnknownEffect}

type Effect struct {
	Kind   Kind   `json:"kind"`
	Detail string `json:"detail"`
	Line   int    `json:"line"`
}

type Diagnostic struct {
	Location
	RuleID    string     `json:"rule_id"`
	Actual    int64      `json:"actual"`
	Limit     int64      `json:"limit"`
	Message   string     `json:"message"`
	Effects   []Effect   `json:"effects,omitempty"`
	Mutations []Mutation `json:"mutations,omitempty"`
	// Weight, Statements and KindWeights explain side_effect_density
	// findings: weight over statements is the density arithmetic, and
	// kind weights show where the weight comes from. Other rules leave
	// them empty.
	Weight      int64            `json:"weight,omitempty"`
	Statements  int64            `json:"statements,omitempty"`
	KindWeights map[string]int64 `json:"kind_weights,omitempty"`
}

type FunctionResult struct {
	Location
	DensityMilli      int64    `json:"density_milli"`
	Mutations         int      `json:"mutations"`
	MutatedTargets    int      `json:"mutated_targets"`
	UnclassifiedCalls int      `json:"unclassified_calls"`
	Complete          bool     `json:"complete"`
	Effects           []Effect `json:"effects"`
}

type EffectCount struct {
	Kind  Kind `json:"kind"`
	Count int  `json:"count"`
}

type Summary struct {
	Functions           int           `json:"functions"`
	MeanDensityMilli    int64         `json:"mean_density_milli"`
	MaxDensityMilli     int64         `json:"max_density_milli"`
	IncompleteFunctions int           `json:"incomplete_functions"`
	Effects             []EffectCount `json:"effects"`
}

type Report struct {
	SchemaVersion int              `json:"schema_version"`
	Summary       Summary          `json:"summary"`
	Functions     []FunctionResult `json:"functions"`
	Diagnostics   []Diagnostic     `json:"diagnostics"`
	// FixGroups is the work plan over Diagnostics: violations whose fixes
	// overlap, ordered callees-first. Additive presentation only; rule
	// metrics are unaffected.
	FixGroups []FixGroup `json:"fix_groups,omitempty"`
}

// FixGroup is a set of violations whose fixes overlap: land them in one
// change or stack them in Functions order (callees first). Groups are
// independent of each other and can be fixed in parallel.
type FixGroup struct {
	ID        int             `json:"id"`
	Functions []GroupFunction `json:"functions"`
	// Stacked reports whether the group lists several functions that must
	// be fixed together. Single-function groups are independent.
	Stacked bool `json:"stacked"`
}

// GroupFunction is one violating function in a fix group and the rules it violates.
type GroupFunction struct {
	Location
	Rules []string `json:"rules"`
}
