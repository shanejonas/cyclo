package patterns

import "sort"

// Arm is one arm of a type switch participating in a dispatch: the arm's
// index in the switch and the id of the method the arm calls on the
// case-bound value.
type Arm struct {
	Index    int
	CalleeID string
}

// Dispatch is a type switch whose arms call analogous methods — same
// signature class, different callees — on the value bound by each case.
// It is the closest thing to an interface that already exists in the
// source: the shared signature class could be declared as an interface
// method and every case type would satisfy it. This mirrors rstyle's
// dispatch::Dispatch, adapted from Rust `match` on enums to Go type
// switches (`switch x.(type)`).
type Dispatch struct {
	// Line is the source line of the type switch.
	Line int
	// Class is the signature class shared by the arm calls.
	Class string
	// Arms holds one entry per arm, ordered by arm index.
	Arms []Arm
}

// armCall is one arm's contribution to a dispatch: the arm index, the
// shared signature class of the call, and the callee id.
type armCall struct {
	arm    int
	class  string
	callee string
}

// receives reports whether the call node receives the value bound by the
// match node m as its receiver: a data edge m -> call at arg position 0.
// In a Go type switch (`switch v := x.(type) { case Dog: v.Bark() }`) the
// Match node is the producer of the case-bound value, so a call that takes
// it as its receiver is a method call on the bound variable — exactly the
// "calls ... on the bound value" condition of the Rust original.
func receives(pdg *Pdg, m, call int) bool {
	for _, e := range pdg.Edges {
		if e.From == m && e.To == call && e.Kind == Data && e.ArgPos == 0 {
			return true
		}
	}
	return false
}

// armCallOf returns the arm call for one ctrl edge from match node m: the
// edge must target a Call node that receives the matched value as its
// receiver (see receives) and has a callee id, mirroring the Rust
// filter_map on callee_id.
func armCallOf(pdg *Pdg, m int, e PdgEdge) (armCall, bool) {
	if e.From != m || e.Kind != Ctrl {
		return armCall{}, false
	}
	call := pdg.Nodes[e.To]
	if call.Kind != Call || call.CalleeID == "" {
		return armCall{}, false
	}
	if !receives(pdg, m, e.To) {
		return armCall{}, false
	}
	return armCall{arm: e.ArgPos, class: CallClass(call), callee: call.CalleeID}, true
}

// armCalls returns one entry per Call node that is control-dependent on an
// arm of match node m and receives the matched value as its receiver. The
// arm index is the ctrl edge's arg position.
func armCalls(pdg *Pdg, m int) []armCall {
	var out []armCall
	for _, e := range pdg.Edges {
		if ac, ok := armCallOf(pdg, m, e); ok {
			out = append(out, ac)
		}
	}
	return out
}

// groupByClass buckets arm calls by signature class, keeping the first
// callee seen per arm (Rust's or_insert).
func groupByClass(calls []armCall) map[string]map[int]string {
	byClass := map[string]map[int]string{}
	for _, ac := range calls {
		arms := byClass[ac.class]
		if arms == nil {
			arms = map[int]string{}
			byClass[ac.class] = arms
		}
		if _, ok := arms[ac.arm]; !ok {
			arms[ac.arm] = ac.callee
		}
	}
	return byClass
}

// dispatchOfClass builds a Dispatch for one signature class when the arms
// call at least two distinct callees — "analogous methods", not the same
// method repeated in every arm.
func dispatchOfClass(line int, class string, arms map[int]string) (Dispatch, bool) {
	distinct := map[string]bool{}
	for _, callee := range arms {
		distinct[callee] = true
	}
	if len(distinct) < 2 {
		return Dispatch{}, false
	}
	indices := make([]int, 0, len(arms))
	for idx := range arms {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	d := Dispatch{Line: line, Class: class}
	for _, idx := range indices {
		d.Arms = append(d.Arms, Arm{Index: idx, CalleeID: arms[idx]})
	}
	return d, true
}

// dispatchesOf finds the dispatches of one match node: arm calls grouped
// by signature class, keeping classes with analogous (same-class,
// different-callee) methods.
func dispatchesOf(pdg *Pdg, m int) []Dispatch {
	byClass := groupByClass(armCalls(pdg, m))
	classes := make([]string, 0, len(byClass))
	for class := range byClass {
		classes = append(classes, class)
	}
	sort.Strings(classes) // deterministic order, like Rust's BTreeMap
	var out []Dispatch
	for _, class := range classes {
		if d, ok := dispatchOfClass(pdg.Nodes[m].Line, class, byClass[class]); ok {
			out = append(out, d)
		}
	}
	return out
}

// FindDispatch finds type switches in one function's PDG whose arms call
// analogous methods (same signature class, different callees) on the
// case-bound value, and reports them as interface candidates. It mirrors
// rstyle's dispatch::find.
//
// The signature takes one *Pdg rather than []*FuncFacts on purpose: a
// match and its arm calls always live inside a single function body, so
// dispatch is intra-function. Callers iterating []*FuncFacts call it per
// f.Pdg (a nil PDG yields no dispatches).
//
// Go adaptations, mirroring the Rust semantics:
//   - Rust `match` on enums becomes Go type switches (`switch x.(type)`),
//     which the extractor lowers to Match nodes.
//   - A `default` clause is just another arm (its ctrl edge carries the
//     default's arm index); it participates in grouping like any case.
//   - Type assertions outside a switch (`x.(Dog).Bark()`) lower to Cast
//     nodes, never Match nodes, so they are skipped by construction.
//   - Nested switches need no recursion: the flat scan visits every Match
//     node, and an inner switch's arm calls are control-dependent on the
//     inner Match node and receive the inner bound value from it.
func FindDispatch(pdg *Pdg) []Dispatch {
	if pdg == nil {
		return nil
	}
	var out []Dispatch
	for m := range pdg.Nodes {
		if pdg.Nodes[m].Kind == Match {
			out = append(out, dispatchesOf(pdg, m)...)
		}
	}
	return out
}
