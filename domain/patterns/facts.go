package patterns

// ParamInfo is a function parameter's name and primitive type name
// (e.g. {Name: "amount", Type: "int"}). Only basic types are recorded;
// named types are already domain types and need no proposal.
type ParamInfo struct {
	Name string
	Type string
}

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
	// extension, not in rstyle's FnFacts).
	GuardClauses []GuardClauseHit
	// EnumDispatches are enum-value switches in this function that want to
	// be dispatch tables. AST-level finding from the extractor.
	EnumDispatches []EnumDispatchHit
	// TypeSwitches are type switches in this function whose arms all call
	// the same method on the case-bound value. AST-level finding from the
	// extractor.
	TypeSwitches []TypeSwitchHit
	// EntityIdentities are attribute-based equalities that should use ID.
	EntityIdentities []EntityIdentityHit
	// MutableIdentities are ID assignments outside constructors.
	MutableIdentities []MutableIdentityHit
	// AggregateMods are named struct types whose fields this function
	// mutates. Co-modified types suggest an aggregate boundary.
	AggregateMods []AggregateModHit
	// DbCalls are direct database calls in this function.
	DbCalls []DbCallHit
	// FactoryLits are complex struct literals in this function.
	FactoryLits []FactoryHit
	// SpecRules are boolean business-rule expressions (2+ conditions with
	// &&/||) in this function. Rules repeated across functions want a
	// Specification type (Evans).
	SpecRules []SpecificationHit
	// Params are the function's primitive-typed parameters (name and type).
	// Used for data-clump detection (value object proposals). Empty when the
	// function has no primitive params or params were not extracted.
	Params []ParamInfo
}

// GuardClauseHit is one inverted conditional: the if-statement line and
// the size of the trapped happy path.
type GuardClauseHit struct {
	// Line is the if-statement line.
	Line int
	// BodyStmts counts statements in the if body (the trapped happy path).
	BodyStmts int
}

// EnumDispatchHit is one enum-value switch that wants to be a dispatch
// table: the switch line and the number of value cases.
type EnumDispatchHit struct {
	// Line is the switch-statement line.
	Line int
	// NumCases counts the value case clauses.
	NumCases int
}

// TypeSwitchHit is one type switch whose arms all call the same method on
// the case-bound value: the switch line, the bound variable name, the
// switched expression text, the shared method name, and the case type names.
type TypeSwitchHit struct {
	// Line is the line of the switch statement.
	Line int
	// Bound is the variable bound by `switch v := x.(type)`.
	Bound string
	// Expr is the source text of the switched expression (x).
	Expr string
	// Method is the method name called in every arm.
	Method string
	// Types are the case type names (Dog, Cat, ...).
	Types []string
	// Args is the source text of the call arguments, identical in every arm.
	Args string
}

// EntityIdentityHit is one attribute-based equality that should use identity.
type EntityIdentityHit struct {
	Line     int
	TypeName string
	IDField  string
	Fields   []string
	Left     string
	Right    string
	// StructPath and StructLine locate the struct definition, for the
	// no-ID case where the fixer must add the ID field first.
	// Empty when the ID already exists.
	StructPath string
	StructLine int
}

// MutableIdentityHit is one ID assignment outside a constructor.
type MutableIdentityHit struct {
	Line     int
	Field    string
	FuncName string
}

// AggregateModHit records one named struct type whose fields are mutated
// inside a function. Pairs of types mutated together across functions
// suggest an aggregate boundary.
type AggregateModHit struct {
	TypeName string
}

// DbCallHit is one direct database call in business logic: the call line
// and the call text (e.g. "db.Query").
type DbCallHit struct {
	Line int
	Call string
}

// FactoryHit is one complex struct literal: the literal line, the struct
// type name, the field count, and the file declaring the struct type.
// HasLogic is true when the enclosing function has construction logic
// (validation, defaults, or error handling) that a factory could encapsulate.
type FactoryHit struct {
	Line      int
	TypeName  string
	NumFields int
	DeclFile  string
	HasLogic  bool
}

// SpecificationHit is one boolean business-rule expression: the if-statement
// line, the normalized rule key (for grouping across functions), the source
// text of the condition, the variable being tested, and its type name.
type SpecificationHit struct {
	Line     int
	RuleKey  string
	CondText string
	VarName  string
	TypeName string
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
