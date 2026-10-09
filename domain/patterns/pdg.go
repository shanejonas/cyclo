// Package patterns implements the program-dependence-graph schema and mining
// primitives for cyclo's abstraction proposer (the port of rstyle's patterns
// miner). The schema is front-end neutral by design: it mirrors rstyle's
// facts::{NodeKind, PdgNode, PdgEdge}, with two Go-specific additions (Defer,
// Go) documented below.
package patterns

// NodeKind is the kind of a program-dependence-graph node. It mirrors rstyle's
// facts::NodeKind, which is explicitly "front-end neutral: a MIR emitter could
// produce the same kinds."
type NodeKind string

const (
	// Param is a function parameter (including the receiver).
	Param NodeKind = "param"
	// Let is a destructuring binding (`a, b := f()`); simple `x := e` is
	// transparent and `x` resolves directly to `e`'s node.
	Let NodeKind = "let"
	// Call is a function or method call.
	Call NodeKind = "call"
	// Field is a field access (`x.F`).
	Field NodeKind = "field"
	// Lit is a literal.
	Lit NodeKind = "lit"
	// Branch is an `if` statement.
	Branch NodeKind = "branch"
	// Match is a type switch (`switch x.(type)`); value switches lower to it too.
	Match NodeKind = "match"
	// Iterate is a `for`/`range` loop over a collection: the same abstract
	// iteration regardless of spelling. The loop variable binds to this node.
	Iterate NodeKind = "iterate"
	// Loop is `for {}` / `for cond {}`.
	Loop NodeKind = "loop"
	// Return is a return statement.
	Return NodeKind = "return"
	// Try is the `if err != nil { return err }` idiom: Go's `?` equivalent.
	Try NodeKind = "try"
	// Closure is a func literal. Opaque: the body is not descended into.
	Closure NodeKind = "closure"
	// Cast is a type conversion or assertion.
	Cast NodeKind = "cast"
	// Op covers operators, composite literals, indexing, assignment, and
	// channel sends. The operator class is in Detail; never the raw operator.
	Op NodeKind = "op"
	// Defer is a `defer f()` statement. Go-specific: rstyle has no equivalent
	// (its schema carries Rust-specific Macro instead, which Go skips).
	// The deferred call lowers to a Call node; Defer wraps it with a data
	// edge so "runs at function exit" is not lost.
	Defer NodeKind = "defer"
	// Go is a `go f()` statement (goroutine spawn). Go-specific, same
	// wrapping as Defer: the spawned call lowers to a Call node first.
	Go NodeKind = "go"
	// Ctor is canonical value construction, synthesized by normalize.
	// Never emitted by the extractor.
	Ctor NodeKind = "ctor"
	// Case is a canonical decision, synthesized by normalize.
	// Never emitted by the extractor.
	Case NodeKind = "case"
)

// EdgeKind mirrors rstyle's facts::EdgeKind.
type EdgeKind string

const (
	// Data is a def→use edge. ArgPos is the operand position (receiver is 0).
	Data EdgeKind = "data"
	// Ctrl is a control-dependence edge. ArgPos is the arm index.
	Ctrl EdgeKind = "ctrl"
)

// PdgNode mirrors rstyle's facts::PdgNode.
type PdgNode struct {
	Kind NodeKind
	// TyClass is the normalized type class of the value this node produces
	// (Param labels use it): primitives kept ("string"), named user types are
	// "_", stdlib ADTs keep their short name, e.g. "*_", "[_]".
	TyClass string
	// SigClass is the normalized signature class (Call labels use it),
	// e.g. "fn(string) -> error". Named types are "_".
	SigClass string
	// CalleeID is the stable id of the called function (Call nodes). Kept for
	// hole detection in the align stage; never part of the WL label.
	CalleeID string
	// LitKind is the literal kind ("string", "int", "float", "char", "nil").
	LitKind string
	// Detail carries operator class ("cmp:=="), field name, etc. For Op nodes
	// only the class before ':' is labeled; never the raw operator or name.
	Detail string
	Line   int
}

// PdgEdge mirrors rstyle's facts::PdgEdge.
type PdgEdge struct {
	From   int
	To     int
	Kind   EdgeKind
	ArgPos int
}

// Pdg is the dependence graph of one function body. Like rstyle's facts::Pdg:
// no execution-order edges, so reordering independent statements does not
// change it.
type Pdg struct {
	Nodes []PdgNode
	Edges []PdgEdge
}
