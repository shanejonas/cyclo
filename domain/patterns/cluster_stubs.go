package patterns

// TEMPORARY STUBS for the parallel normalize/align ports.
//
// DELETE THIS FILE when domain/patterns/normalize.go and
// domain/patterns/align.go land. The declarations below mirror the assumed
// contract exactly (Rules fields, Canonicalize signature, HoleKind constants,
// Hole/Alignment shapes, Align signature); Canonicalize is the identity and
// Align returns zero coverage until the real ports replace them.
//
// Do not extend these stubs: real behavior belongs in the ported files.

// Rules mirrors the assumed normalize API: all fields off by default.
type Rules struct {
	Ctor      bool
	Pred      bool
	Case      bool
	Exit      bool
	Counter   bool
	Cmp       bool
	Propagate bool
}

// Canonicalize is the identity until normalize.go lands.
func Canonicalize(pdg *Pdg, rules Rules) *Pdg {
	return pdg
}

// HoleKind mirrors the assumed align API.
type HoleKind string

const (
	HoleType    HoleKind = "type"
	HoleMethod  HoleKind = "method"
	HoleFreeFn  HoleKind = "freefn"
	HoleLiteral HoleKind = "literal"
	HoleField   HoleKind = "field"
	HoleOp      HoleKind = "op"
)

// Hole mirrors the assumed align API: one substitution a -> b.
type Hole struct {
	Kind HoleKind
	Var  string
	A    string
	B    string
	// Sites are aligned node pairs (node in a, node in b) where the
	// substitution applies.
	Sites [][2]int
}

// Alignment mirrors the assumed align API.
type Alignment struct {
	Pairs         [][2]int
	Holes         []Hole
	CoverageMilli uint32
}

// Align returns a zero alignment until align.go lands.
func Align(pa *Pdg, wa *Wl, pb *Pdg, wb *Wl) Alignment {
	return Alignment{}
}
