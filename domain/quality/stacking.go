package quality

// Stacking plans: ordered parameter refactors for fn_params violations.
// Pure over facts and the call graph.
//
// Changing a signature edits the function and every call site, so the plan
// is ordered callee signature first, then the callers whose call sites must
// follow. Two facts make the plan actionable:
//   - bundles: parameters that a caller hands to a callee unchanged, several
//     at once. They travel together, so they are a params-struct candidate.
//   - pass-through parameters: a caller's parameter that is only ever
//     forwarded to a re-signed function. The caller does not need it; the
//     fix belongs in the callee or an upper layer.
//
// Plans are built per fn_params diagnostic over the violating function and
// its callers. buildPlan takes callee-first member indices and a violator
// set so it can attach to fix groups later; today each diagnostic gets its
// own single-member plan.

import (
	"cmp"
	"maps"
	"slices"
	"strings"
)

const stackRule = "fn_params"

// callSite is one static local call from a caller into a callee, with the
// argument provenance recorded by the extractor.
type callSite struct {
	caller int
	callee int
	args   []ArgSource
}

// stackSites resolves static local calls to function indices. Unresolved,
// dynamic, and self calls are not sites.
func stackSites(functions []Function) []callSite {
	index := nameIndex(functions)
	var sites []callSite
	for caller, f := range functions {
		for _, c := range f.Calls {
			if s, ok := resolveSite(caller, c, index); ok {
				sites = append(sites, s)
			}
		}
	}
	return sites
}

func resolveSite(caller int, c Call, index map[string]int) (callSite, bool) {
	if !c.Local || c.Dynamic {
		return callSite{}, false
	}
	callee, ok := index[NormalizeCallee(c.Callee)]
	if !ok || callee == caller {
		return callSite{}, false
	}
	return callSite{caller: caller, callee: callee, args: c.Args}, true
}

// paramCount is the number of forwardable parameter positions.
func paramCount(f Function) int {
	if len(f.ParamList) > 0 {
		return len(f.ParamList)
	}
	return f.Params
}

// forward is a (callerParam, calleeParam) pair for one bare-parameter argument.
type forward struct {
	from int
	to   int
}

// forwards pairs caller and callee parameter positions for every argument
// that is a bare caller parameter.
func forwards(s callSite, functions []Function) []forward {
	callerParams := paramCount(functions[s.caller])
	calleeParams := paramCount(functions[s.callee])
	var out []forward
	for to, arg := range s.args {
		if to >= calleeParams {
			continue
		}
		if arg.Kind == ArgParam && arg.Param < callerParams {
			out = append(out, forward{from: arg.Param, to: to})
		}
	}
	return out
}

type bundleKey struct {
	name string
	typ  string
}

func calleeKey(functions []Function, callee, param int) (bundleKey, bool) {
	list := functions[callee].ParamList
	if param < 0 || param >= len(list) {
		return bundleKey{}, false
	}
	return bundleKey{name: list[param].Name, typ: list[param].Type}, true
}

type scoredSet struct {
	set   map[bundleKey]bool
	count int
}

func compareBundleKey(a, b bundleKey) int {
	return cmp.Or(
		strings.Compare(a.name, b.name),
		strings.Compare(a.typ, b.typ),
	)
}

func setKey(s map[bundleKey]bool) string {
	keys := make([]bundleKey, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareBundleKey)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k.name)
		sb.WriteByte(0)
		sb.WriteString(k.typ)
		sb.WriteByte(0)
	}
	return sb.String()
}

func intersectSets(a, b map[bundleKey]bool) map[bundleKey]bool {
	out := map[bundleKey]bool{}
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

func subsetOf(a, b map[bundleKey]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// closedSets finds itemsets with at least two items and their support,
// keeping only closed ones: a set is dropped when a strict superset is
// supported by exactly the same inputs. Candidates are the inputs and their
// pairwise intersections (one round; enough for plan-sized inputs).
func closedSets(inputs []map[bundleKey]bool) []scoredSet {
	var scored []scoredSet
	for _, c := range candidateSets(inputs) {
		if len(c) >= 2 {
			scored = append(scored, scoredSet{set: c, count: setSupport(c, inputs)})
		}
	}
	return keepClosed(scored)
}

func candidateSets(inputs []map[bundleKey]bool) []map[bundleKey]bool {
	pairs := make([]map[bundleKey]bool, 0, len(inputs)*len(inputs)/2)
	for i, a := range inputs {
		for _, b := range inputs[i+1:] {
			pairs = append(pairs, intersectSets(a, b))
		}
	}
	return dedupSets(slices.Concat(inputs, pairs))
}

// dedupSets drops duplicate sets, keeping first occurrence order.
func dedupSets(in []map[bundleKey]bool) []map[bundleKey]bool {
	var out []map[bundleKey]bool
	for _, s := range in {
		if !containsSet(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// containsSet is pure: it only reads.
func containsSet(out []map[bundleKey]bool, s map[bundleKey]bool) bool {
	for _, o := range out {
		if maps.Equal(o, s) {
			return true
		}
	}
	return false
}

func setSupport(s map[bundleKey]bool, inputs []map[bundleKey]bool) int {
	n := 0
	for _, in := range inputs {
		if subsetOf(s, in) {
			n++
		}
	}
	return n
}

func keepClosed(scored []scoredSet) []scoredSet {
	var out []scoredSet
	for _, s := range scored {
		if isClosed(s, scored) {
			out = append(out, s)
		}
	}
	return out
}

func isClosed(s scoredSet, scored []scoredSet) bool {
	for _, o := range scored {
		if o.count == s.count && len(o.set) > len(s.set) && subsetOf(s.set, o.set) {
			return false
		}
	}
	return true
}

// bundles finds parameter bundles traveling together through sites: the
// callee's (name, type) keys forwarded as bare params at each site.
// Biggest first, then most sites, then carrier functions.
func bundles(sites []callSite, functions []Function) []Bundle {
	perSite := siteKeys(sites, functions)
	var out []Bundle
	for _, sc := range closedSets(perSite) {
		out = append(out, makeBundle(sc, sites, perSite, functions))
	}
	sortBundles(out)
	return out
}

func siteKeys(sites []callSite, functions []Function) []map[bundleKey]bool {
	perSite := make([]map[bundleKey]bool, len(sites))
	for i, s := range sites {
		set := map[bundleKey]bool{}
		for _, fw := range forwards(s, functions) {
			if key, ok := calleeKey(functions, s.callee, fw.to); ok {
				set[key] = true
			}
		}
		perSite[i] = set
	}
	return perSite
}

func makeBundle(sc scoredSet, sites []callSite, perSite []map[bundleKey]bool, functions []Function) Bundle {
	carriers := map[string]bool{}
	for i, s := range sites {
		if subsetOf(sc.set, perSite[i]) {
			carriers[functions[s.caller].Name] = true
			carriers[functions[s.callee].Name] = true
		}
	}
	params := make([]ParamRef, 0, len(sc.set))
	for k := range sc.set {
		params = append(params, ParamRef{Name: k.name, Type: k.typ})
	}
	slices.SortFunc(params, compareParamRef)
	return Bundle{Params: params, Sites: sc.count, Functions: slices.Sorted(maps.Keys(carriers))}
}

func compareParamRef(a, b ParamRef) int {
	return cmp.Or(
		strings.Compare(a.Name, b.Name),
		strings.Compare(a.Type, b.Type),
	)
}

func sortBundles(out []Bundle) {
	slices.SortFunc(out, compareBundles)
}

func compareBundles(a, b Bundle) int {
	if len(a.Params) != len(b.Params) {
		return cmp.Compare(len(b.Params), len(a.Params))
	}
	if a.Sites != b.Sites {
		return cmp.Compare(b.Sites, a.Sites)
	}
	return slices.Compare(a.Functions, b.Functions)
}

// passThrough names parameters of f whose every mention is a forward into
// the given sites: the caller does not need them.
func passThrough(f int, functions []Function, sites []callSite) []string {
	forwarded := forwardCounts(f, functions, sites)
	var out []string
	for i, p := range functions[f].ParamList {
		if p.Uses > 0 && forwarded[i] >= p.Uses {
			out = append(out, p.Name)
		}
	}
	return out
}

func forwardCounts(f int, functions []Function, sites []callSite) map[int]int {
	forwarded := map[int]int{}
	for _, s := range sites {
		if s.caller != f {
			continue
		}
		for _, fw := range forwards(s, functions) {
			forwarded[fw.from]++
		}
	}
	return forwarded
}

// planCtx holds the resolved state for building one stacking plan.
type planCtx struct {
	functions []Function
	feeding   []callSite
	incoming  map[int]int
	outgoing  map[int]int
}

// buildPlan computes the stacking plan for members (callee-first indices)
// where violators marks the fn_params violators. Nil when no member violates.
func buildPlan(functions []Function, sites []callSite, members []int, violators map[int]bool) *StackPlan {
	changed, changedSet := changedMembers(members, violators)
	if len(changed) == 0 {
		return nil
	}
	ctx := newPlanCtx(functions, sites, changedSet)
	touched := touchedFunctions(members, changedSet, ctx.feeding)
	return &StackPlan{Steps: ctx.steps(changed, touched), Bundles: bundles(ctx.feeding, functions)}
}

// newPlanCtx resolves the feeding sites and site counts once.
func newPlanCtx(functions []Function, sites []callSite, changedSet map[int]bool) *planCtx {
	feeding := feedingSites(sites, changedSet)
	incoming, outgoing := siteCounts(sites, feeding)
	return &planCtx{functions: functions, feeding: feeding, incoming: incoming, outgoing: outgoing}
}

func changedMembers(members []int, violators map[int]bool) ([]int, map[int]bool) {
	var changed []int
	set := map[int]bool{}
	for _, m := range members {
		if violators[m] {
			changed = append(changed, m)
			set[m] = true
		}
	}
	return changed, set
}

// feedingSites are the sites into a re-signed function.
func feedingSites(sites []callSite, changedSet map[int]bool) []callSite {
	var out []callSite
	for _, s := range sites {
		if changedSet[s.callee] {
			out = append(out, s)
		}
	}
	return out
}

// touchedFunctions are the callers of re-signed functions plus the other
// members, sorted by index for determinism.
func touchedFunctions(members []int, changedSet map[int]bool, feeding []callSite) []int {
	touched := map[int]bool{}
	for _, s := range feeding {
		if !changedSet[s.caller] {
			touched[s.caller] = true
		}
	}
	for _, m := range members {
		if !changedSet[m] {
			touched[m] = true
		}
	}
	return slices.Sorted(maps.Keys(touched))
}

func siteCounts(sites, feeding []callSite) (incoming, outgoing map[int]int) {
	incoming = map[int]int{}
	for _, s := range sites {
		incoming[s.callee]++
	}
	outgoing = map[int]int{}
	for _, s := range feeding {
		outgoing[s.caller]++
	}
	return incoming, outgoing
}

func (c *planCtx) step(index int, signature bool) PlanStep {
	return PlanStep{
		Location:      c.functions[index].Location,
		Signature:     signature,
		IncomingSites: c.incoming[index],
		OutgoingSites: c.outgoing[index],
		PassThrough:   passThrough(index, c.functions, c.feeding),
	}
}

// steps builds the ordered step list: re-signed signatures callee-first,
// then the callers whose call sites must follow.
func (c *planCtx) steps(changed, touched []int) []PlanStep {
	order := slices.Concat(changed, touched)
	isChanged := map[int]bool{}
	for _, m := range changed {
		isChanged[m] = true
	}
	out := make([]PlanStep, len(order))
	for i, index := range order {
		out[i] = c.step(index, isChanged[index])
	}
	return out
}

// planFindings attaches a stacking plan to each fn_params finding: the
// violating function's signature change plus its callers' call-site edits.
// buildPlan takes callee-first member indices so fix groups can pass their
// members here later; today each diagnostic gets its own single-member plan.
func planFindings(functions []Function, sites []callSite, idx int, findings []Diagnostic) {
	for i := range findings {
		if findings[i].RuleID == stackRule {
			findings[i].Plan = buildPlan(functions, sites, []int{idx}, map[int]bool{idx: true})
		}
	}
}
