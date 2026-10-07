package patterns

// FuncFacts is the Go equivalent of rstyle's facts::FnFacts for the patterns
// miner: a function's identity, its PDG, and the effect summary used for the
// effect-mismatch check. The extractor (adapters/gopatterns) builds these.
type FuncFacts struct {
	// ID is the stable function id (package path + name, like FuncID).
	ID string
	// Name is the short function name for display.
	Name string
	// Path is the source file path.
	Path string
	// Line is the function's declaration line.
	Line int
	// EndLine is the function's closing line.
	EndLine int
	// Pdg is the function's program dependence graph. Nil means the body
	// was not extracted (filtered out of mining, like rstyle's pdg: None).
	Pdg *Pdg
	// Mutates is true when the function has any escaping mutation
	// (rstyle: mutations.iter().any(|m| m.escapes)).
	Mutates bool
	// EffectKinds are the effect kind names (rstyle: effects[].kind.name()),
	// e.g. "io", "mutation". Empty means pure.
	EffectKinds []string
	// SigKey is the normalized signature key for this function
	// (rstyle: shape::normalize(sig).key; Go: SigClass). Used by sigmine.
	SigKey string
	// SelfTy is the receiver type class for methods (rstyle: sig.owner()).
	// Empty for free functions.
	SelfTy string
	// Implements is true when the method implements an interface method
	// (rstyle: f.implements.is_some()). Counted by sigmine, never proposed.
	Implements bool
	// GuardClauses are inverted conditionals in this function that want to
	// be guard clauses. AST-level finding from the extractor (Go-specific
	// extension, not in rstyle's FnFacts); each holds the if-statement line.
	GuardClauses []int
}

// EffectClass returns the observable-effect class set for the mismatch check:
// effect kind names plus "mutation" when Mutates, mirroring rstyle's
// candidates::effect_class. Empty set means pure.
func (f *FuncFacts) EffectClass() map[string]bool {
	classes := map[string]bool{}
	for _, kind := range f.EffectKinds {
		classes[kind] = true
	}
	if f.Mutates {
		classes["mutation"] = true
	}
	return classes
}
