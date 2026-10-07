package quality

import (
	"cmp"
	"maps"
	"slices"
)

// BuildFixGroups turns flat diagnostics into an ordered work plan. A
// violation's footprint is its function plus, for signature-changing rules
// (config.Grouping.CallerRules), its callers up to CallerHops, because a
// signature fix edits every call site. Violations with overlapping
// footprints form one group; members are ordered callees-first so fixes
// stack in that order. Everything else is independent.
//
// Shared callers or mutated types that do not force a shared edit are
// deliberately not links: on real code they collapse half the violations
// into one hub group instead of a parallelizable plan.
func BuildFixGroups(facts []Function, diagnostics []Diagnostic, config Config) ([]FixGroup, error) {
	functions, err := canonicalFacts(facts)
	if err != nil {
		return nil, err
	}
	return buildFixGroups(functions, diagnostics, config), nil
}

// buildFixGroups groups already-canonical facts; Evaluate calls it directly
// after canonicalizing.
func buildFixGroups(functions []Function, diagnostics []Diagnostic, config Config) []FixGroup {
	violators := violatorRules(functions, diagnostics)
	if len(violators) == 0 {
		return nil
	}
	callees, callers := callGraph(functions)
	components := groupComponents(violators, callers, config.Grouping, len(functions))
	groups := make([]FixGroup, 0, len(components))
	for id, members := range components {
		ordered := stackOrder(members, callees)
		listed := make([]GroupFunction, 0, len(ordered))
		for _, index := range ordered {
			listed = append(listed, GroupFunction{Location: functions[index].Location, Rules: violators[index]})
		}
		groups = append(groups, FixGroup{ID: id + 1, Functions: listed, Stacked: len(ordered) > 1})
	}
	return groups
}

type functionKey struct {
	path         string
	line, column int
}

// violatorRules maps each violating function index to its sorted violated rule IDs.
func violatorRules(functions []Function, diagnostics []Diagnostic) map[int][]string {
	byLocation := make(map[functionKey]int, len(functions))
	for index, function := range functions {
		byLocation[functionKey{function.Path, function.Line, function.Column}] = index
	}
	ruleSets := make(map[int]map[string]bool)
	for _, diagnostic := range diagnostics {
		index, ok := byLocation[functionKey{diagnostic.Path, diagnostic.Line, diagnostic.Column}]
		if !ok {
			continue
		}
		if ruleSets[index] == nil {
			ruleSets[index] = make(map[string]bool)
		}
		ruleSets[index][diagnostic.RuleID] = true
	}
	violators := make(map[int][]string, len(ruleSets))
	for index, set := range ruleSets {
		violators[index] = slices.Sorted(maps.Keys(set))
	}
	return violators
}

// callGraph resolves static local calls to function indices: callees[i]
// lists the functions i calls, callers[j] the functions calling j. Names
// resolve the same flat way helper summaries do.
func callGraph(functions []Function) (callees, callers [][]int) {
	byName := nameIndex(functions)
	callees = make([][]int, len(functions))
	callers = make([][]int, len(functions))
	for index, function := range functions {
		for _, target := range resolvedCallees(function, byName) {
			callees[index] = append(callees[index], target)
			if target != index {
				callers[target] = append(callers[target], index)
			}
		}
	}
	for index := range functions {
		callees[index] = sortedUnique(callees[index])
		callers[index] = sortedUnique(callers[index])
	}
	return callees, callers
}

// nameIndex maps normalized function names to their index.
func nameIndex(functions []Function) map[string]int {
	byName := make(map[string]int, len(functions))
	for index, function := range functions {
		byName[NormalizeCallee(function.Name)] = index
	}
	return byName
}

// resolvedCallees resolves a function's static local calls to function indices.
func resolvedCallees(function Function, byName map[string]int) []int {
	var targets []int
	for _, call := range function.Calls {
		if !call.Local || call.Dynamic {
			continue
		}
		if target, ok := byName[NormalizeCallee(call.Callee)]; ok {
			targets = append(targets, target)
		}
	}
	return targets
}

func sortedUnique(list []int) []int {
	slices.Sort(list)
	return slices.Compact(list)
}

// footprint is the set of functions a fix edits: the function itself plus,
// for signature-changing rules, its callers up to hops calls away.
func footprint(index int, rules []string, callers [][]int, grouping Grouping) []int {
	if !reachesCallers(rules, grouping) {
		return []int{index}
	}
	return callersWithin(index, grouping.CallerHops, callers)
}

// reachesCallers reports whether any violated rule changes signatures, so
// its fix edits callers too.
func reachesCallers(rules []string, grouping Grouping) bool {
	return slices.ContainsFunc(rules, func(rule string) bool {
		return slices.Contains(grouping.CallerRules, rule)
	})
}

// callersWithin returns index plus its callers up to hops calls away.
func callersWithin(index, hops int, callers [][]int) []int {
	seen := map[int]bool{index: true}
	frontier := []int{index}
	for range hops {
		frontier = expandCallers(frontier, callers, seen)
	}
	return slices.Sorted(maps.Keys(seen))
}

// expandCallers records the unseen callers of frontier and returns them as
// the next frontier.
func expandCallers(frontier []int, callers [][]int, seen map[int]bool) []int {
	var next []int
	for _, function := range frontier {
		for _, caller := range callers[function] {
			if !seen[caller] {
				seen[caller] = true
				next = append(next, caller)
			}
		}
	}
	return next
}

// groupComponents unions overlapping footprints and returns each
// component's member indices in source order. Only violating functions are
// members; shared non-violating callers link groups without joining them.
func groupComponents(violators map[int][]string, callers [][]int, grouping Grouping, size int) [][]int {
	merged := newFixDSU(size)
	for _, index := range slices.Sorted(maps.Keys(violators)) {
		for _, item := range footprint(index, violators[index], callers, grouping) {
			merged.union(index, item)
		}
	}
	byRoot := make(map[int][]int)
	for _, index := range slices.Sorted(maps.Keys(violators)) {
		root := merged.find(index)
		byRoot[root] = append(byRoot[root], index)
	}
	components := slices.Collect(maps.Values(byRoot))
	slices.SortFunc(components, func(a, b []int) int { return cmp.Compare(a[0], b[0]) })
	return components
}

// stackOrder orders members callees-first: a member is emitted once none of
// the members it calls are still waiting. Cycles fall back to source order.
func stackOrder(members []int, callees [][]int) []int {
	waiting := make(map[int]bool, len(members))
	for _, member := range members {
		waiting[member] = true
	}
	ordered := make([]int, 0, len(members))
	for len(waiting) > 0 {
		next := nextStackable(members, waiting, callees)
		if next == -1 {
			break // unreachable: members always covers the waiting set
		}
		delete(waiting, next)
		ordered = append(ordered, next)
	}
	return ordered
}

// nextStackable returns the first member in source order whose callees have
// all been emitted. When calls form a cycle no member is ready; then it
// returns the first waiting member so ordering stays deterministic.
func nextStackable(members []int, waiting map[int]bool, callees [][]int) int {
	for _, member := range members {
		if waiting[member] && stackReady(member, waiting, callees) {
			return member
		}
	}
	for _, member := range members {
		if waiting[member] {
			return member
		}
	}
	return -1
}

// stackReady reports whether member calls no member that is still waiting.
func stackReady(member int, waiting map[int]bool, callees [][]int) bool {
	for _, callee := range callees[member] {
		if waiting[callee] {
			return false
		}
	}
	return true
}

// fixDSU is a union-find with path compression; the smaller index wins so
// results do not depend on union order.
type fixDSU struct {
	parent []int
}

func newFixDSU(size int) *fixDSU {
	parent := make([]int, size)
	for index := range parent {
		parent[index] = index
	}
	return &fixDSU{parent: parent}
}

func (d *fixDSU) find(index int) int {
	if d.parent[index] != index {
		d.parent[index] = d.find(d.parent[index])
	}
	return d.parent[index]
}

func (d *fixDSU) union(a, b int) {
	a, b = d.find(a), d.find(b)
	if a == b {
		return
	}
	if a > b {
		a, b = b, a
	}
	d.parent[b] = a
}
