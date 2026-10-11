package patterns

import (
	"fmt"
	"strings"
)

// Taint analysis: track untrusted data from sources → through propagators
// → to sinks. Flag when tainted data reaches a dangerous sink without
// sanitization.
//
// The three abstractions (from SAST engineering):
//   - Sources: entry points of untrusted input (HTTP params, CLI args, ...)
//   - Propagators: operations that carry taint (string concat, assignment, ...)
//   - Sinks: sensitive operations that must not receive tainted input
//     (SQL queries, shell commands, ...)
// Sanitizers break the chain: their output is clean even with tainted input.
//
// For cyclo: v1 is intraprocedural. We walk the PDG's data edges forward
// from sources, marking tainted nodes to a fixpoint. A sink whose dangerous
// argument receives tainted data is a finding. Detection-only: the fix
// depends on context (parameterize the query? escape the output? validate
// the path?), so the LLM judges.

// taintSourceSuffixes are CalleeID suffixes whose result is tainted.
var taintSourceSuffixes = []string{
	// HTTP request input.
	"(*url.Values).Get", // r.URL.Query().Get("x")
	"(*http.Request).FormValue",
	"(*http.Header).Get", // r.Header.Get("x")
	// OS input.
	"os.Getenv",
	// I/O: result tainted (conservative: assume the reader is untrusted).
	"io.ReadAll",
	"ioutil.ReadAll",
}

// taintSanitizerSuffixes are CalleeID suffixes whose result is clean even
// with tainted input.
var taintSanitizerSuffixes = []string{
	"template.HTMLEscapeString",
	"html.EscapeString",
	"url.QueryEscape",
	"url.PathEscape",
	// Numeric conversions: numbers can't inject.
	"strconv.Atoi",
	"strconv.ParseInt",
	"strconv.ParseFloat",
	"strconv.ParseUint",
}

// taintSink describes a dangerous operation and which argument positions
// must not receive tainted data. A nil dangerousArgs means any argument.
type taintSink struct {
	// callee is a CalleeID suffix, e.g. "(*sql.DB).Exec".
	callee string
	// dangerousArgs are the ArgPos values that must be clean. Nil = any.
	// Note: for method calls the receiver is ArgPos 0, so the first
	// real argument is ArgPos 1.
	dangerousArgs []int
	// desc is the human-readable risk, e.g. "SQL injection".
	desc string
}

// taintSinks is the hardcoded sink list.
var taintSinks = []taintSink{
	// SQL: only the query string is dangerous (ArgPos 1; ArgPos 0 is the
	// receiver). Tainted query parameters (ArgPos 2+) are the safe
	// parameterized pattern — do NOT flag those.
	{callee: "(*sql.DB).Exec", dangerousArgs: []int{1}, desc: "SQL injection"},
	{callee: "(*sql.DB).Query", dangerousArgs: []int{1}, desc: "SQL injection"},
	{callee: "(*sql.DB).QueryRow", dangerousArgs: []int{1}, desc: "SQL injection"},
	{callee: "(*sql.Tx).Exec", dangerousArgs: []int{1}, desc: "SQL injection"},
	{callee: "(*sql.Tx).Query", dangerousArgs: []int{1}, desc: "SQL injection"},
	{callee: "(*sql.Conn).Exec", dangerousArgs: []int{1}, desc: "SQL injection"},
	// Shell: any tainted argument is dangerous.
	{callee: "exec.Command", desc: "command injection"},
	{callee: "exec.CommandContext", desc: "command injection"},
	// Templates: the data argument (ArgPos 2; receiver 0, writer 1).
	{callee: "(*template.Template).Execute", dangerousArgs: []int{2}, desc: "XSS"},
	// File paths: the path argument.
	{callee: "os.Open", dangerousArgs: []int{0}, desc: "path traversal"},
	{callee: "os.Create", dangerousArgs: []int{0}, desc: "path traversal"},
	{callee: "os.WriteFile", dangerousArgs: []int{0}, desc: "path traversal"},
	{callee: "ioutil.WriteFile", dangerousArgs: []int{0}, desc: "path traversal"},
}

// TaintFlow is one tainted-data-to-sink flow.
type TaintFlow struct {
	FuncID string
	// SourceLine is where the tainted data enters.
	SourceLine int
	// SourceCall is the source callee, e.g. "(*url.Values).Get".
	SourceCall string
	// SinkLine is where it reaches the dangerous sink.
	SinkLine int
	// SinkCall is the sink callee, e.g. "(*sql.DB).Exec".
	SinkCall string
	// Risk is the human-readable risk, e.g. "SQL injection".
	Risk string
}

// isTaintSource reports whether n is a taint source call.
func isTaintSource(n PdgNode) bool {
	if n.Kind != Call {
		return false
	}
	for _, s := range taintSourceSuffixes {
		if strings.HasSuffix(n.CalleeID, s) {
			return true
		}
	}
	return false
}

// isTaintSanitizer reports whether n is a sanitizer call.
func isTaintSanitizer(n PdgNode) bool {
	if n.Kind != Call {
		return false
	}
	for _, s := range taintSanitizerSuffixes {
		if strings.HasSuffix(n.CalleeID, s) {
			return true
		}
	}
	return false
}

// matchTaintSink returns the sink rule for n, or nil.
func matchTaintSink(n PdgNode) *taintSink {
	if n.Kind != Call {
		return nil
	}
	for i := range taintSinks {
		if strings.HasSuffix(n.CalleeID, taintSinks[i].callee) {
			return &taintSinks[i]
		}
	}
	return nil
}

// propagateTaint marks tainted nodes via forward dataflow to a fixpoint.
// Source calls start tainted; taint flows along data edges; sanitizer calls
// stay clean and break the chain.
func propagateTaint(pdg *MiningGraph) map[int]bool {
	tainted := seedTaintSources(pdg)
	succ := dataSuccessors(pdg)
	for changed := true; changed; {
		changed = propagateTaintStep(pdg, succ, tainted)
	}
	return tainted
}

// seedTaintSources marks source call nodes as tainted.
func seedTaintSources(pdg *MiningGraph) map[int]bool {
	tainted := map[int]bool{}
	for i, n := range pdg.Nodes {
		if isTaintSource(n) {
			tainted[i] = true
		}
	}
	return tainted
}

// dataSuccessors builds adjacency lists for data edges only.
func dataSuccessors(pdg *MiningGraph) [][]int {
	succ := make([][]int, len(pdg.Nodes))
	for _, e := range pdg.Edges {
		if e.Kind == Data {
			succ[e.From] = append(succ[e.From], e.To)
		}
	}
	return succ
}

// propagateTaintStep performs one forward propagation pass.
// Returns true if any new node was marked tainted.
func propagateTaintStep(pdg *MiningGraph, succ [][]int, tainted map[int]bool) bool {
	changed := false
	for from, tos := range succ {
		if taintSpreads(pdg, tainted, from, tos) {
			changed = true
		}
	}
	return changed
}

// taintSpreads marks untainted non-sanitizer successors of a tainted node.
// Returns true if any new node was marked.
func taintSpreads(pdg *MiningGraph, tainted map[int]bool, from int, tos []int) bool {
	if !tainted[from] {
		return false
	}
	spread := false
	for _, to := range tos {
		if tainted[to] || isTaintSanitizer(pdg.Nodes[to]) {
			continue
		}
		tainted[to] = true
		spread = true
	}
	return spread
}

// sinkArgTainted reports whether a dangerous argument of sink node idx
// receives tainted data.
func sinkArgTainted(pdg *MiningGraph, idx int, sink *taintSink, tainted map[int]bool) bool {
	for _, e := range pdg.Edges {
		if taintedSinkEdge(pdg, e, idx, sink, tainted) {
			return true
		}
	}
	return false
}

// taintedSinkEdge reports whether e is a tainted data edge into a
// dangerous argument of the sink.
func taintedSinkEdge(pdg *MiningGraph, e PdgEdge, idx int, sink *taintSink, tainted map[int]bool) bool {
	if e.Kind != Data || e.To != idx || !tainted[e.From] {
		return false
	}
	return sink.dangerousArgs == nil || intInSlice(e.ArgPos, sink.dangerousArgs)
}

// intInSlice reports whether v is in s.
func intInSlice(v int, s []int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// findTaintSource traces back from a tainted node to a source call,
// returning its line and callee. Best-effort: first source found.
func findTaintSource(pdg *MiningGraph, tainted map[int]bool, from int) (line int, callee string) {
	visited := map[int]bool{from: true}
	queue := taintedPredecessors(pdg, tainted, from, visited)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		if isTaintSource(pdg.Nodes[cur]) {
			return pdg.Nodes[cur].Line, pdg.Nodes[cur].CalleeID
		}
		queue = append(queue, taintedPredecessors(pdg, tainted, cur, visited)...)
	}
	return 0, ""
}

// taintedPredecessors returns tainted data-predecessors of node, excluding
// already-visited ones.
func taintedPredecessors(pdg *MiningGraph, tainted map[int]bool, to int, visited map[int]bool) []int {
	var out []int
	for _, e := range pdg.Edges {
		if e.Kind == Data && e.To == to && tainted[e.From] && !visited[e.From] {
			out = append(out, e.From)
		}
	}
	return out
}

// FindTaintFlows finds tainted-data-to-sink flows in each function.
func FindTaintFlows(facts []*MiningFacts) []TaintFlow {
	var out []TaintFlow
	for _, f := range facts {
		if f.Pdg == nil {
			continue
		}
		tainted := propagateTaint(f.Pdg)
		for i, n := range f.Pdg.Nodes {
			sink := matchTaintSink(n)
			if sink == nil {
				continue
			}
			if !sinkArgTainted(f.Pdg, i, sink, tainted) {
				continue
			}
			srcLine, srcCall := findTaintSource(f.Pdg, tainted, i)
			out = append(out, TaintFlow{
				FuncID:     f.ID,
				SourceLine: srcLine,
				SourceCall: srcCall,
				SinkLine:   n.Line,
				SinkCall:   n.CalleeID,
				Risk:       sink.desc,
			})
		}
	}
	return out
}

// TaintFlowCandidates converts taint flows to pattern candidates.
// Detection-only: the fix depends on context, so FixSpec is nil.
func TaintFlowCandidates(flows []TaintFlow, facts []*MiningFacts) []Candidate {
	factByID := map[string]*MiningFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, fl := range flows {
		f := factByID[fl.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:       TaintFlowKind,
			ScoreMilli: 900, // High: taint to sink is a security bug.
			Observation: fmt.Sprintf(
				"tainted data from `%s` reaches `%s` (%s)",
				shortCallee(fl.SourceCall), shortCallee(fl.SinkCall), fl.Risk,
			),
			Inference: fmt.Sprintf(
				"untrusted input flows to a dangerous sink without sanitization",
			),
			PossibleRefactor: "sanitize the input or parameterize the operation",
			Sites: []Site{
				{Path: f.Path, Line: fl.SourceLine, Name: f.Name},
				{Path: f.Path, Line: fl.SinkLine, Name: f.Name},
			},
		})
	}
	return out
}

// shortCallee trims a CalleeID to its last two dot-separated parts.
func shortCallee(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) <= 2 {
		return id
	}
	return strings.Join(parts[len(parts)-2:], ".")
}
