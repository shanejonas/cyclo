package quality

import (
	"slices"
	"testing"
)

func stackParam(name, typ string, uses int) ParamFacts {
	return ParamFacts{Name: name, Type: typ, Uses: uses}
}

func stackFunc(name string, params []ParamFacts) Function {
	return Function{
		Location:  Location{Path: "sample.go", Line: 1, Column: 1, Name: name},
		Params:    len(params),
		ParamList: params,
		Calls:     []Call{},
	}
}

func stackCall(caller *Function, callee string, args []ArgSource) {
	caller.Calls = append(caller.Calls, Call{Callee: callee, Line: 2, Local: true, Args: args})
}

func paramArg(index int) ArgSource { return ArgSource{Kind: ArgParam, Param: index} }

var (
	argConst = ArgSource{Kind: ArgConst}
	argField = ArgSource{Kind: ArgField}
	argOther = ArgSource{Kind: ArgOther}
)

func abcUses(uses int) []ParamFacts {
	return []ParamFacts{
		stackParam("a", "int", uses),
		stackParam("b", "bool", uses),
		stackParam("c", "string", uses),
	}
}

func stackKeys(names ...string) map[bundleKey]bool {
	set := map[bundleKey]bool{}
	for _, n := range names {
		set[bundleKey{name: n, typ: "t"}] = true
	}
	return set
}

func TestClosedSetsDropSubsetsWithEqualSupport(t *testing.T) {
	sets := []map[bundleKey]bool{stackKeys("a", "b", "c"), stackKeys("a", "b", "c")}
	found := closedSets(sets)
	if len(found) != 1 || found[0].count != 2 || len(found[0].set) != 3 {
		t.Fatalf("closed: %+v", found)
	}
}

func TestClosedSetsKeepSmallerSetWithMoreSupport(t *testing.T) {
	sets := []map[bundleKey]bool{
		stackKeys("a", "b", "c"),
		stackKeys("a", "b", "d"),
		stackKeys("a", "b"),
	}
	found := closedSets(sets)
	counts := map[int]int{}
	for _, s := range found {
		counts[len(s.set)] = s.count
	}
	if counts[2] != 3 || counts[3] != 1 || len(found) != 3 {
		t.Fatalf("closed: %+v", found)
	}
}

func TestClosedSetsIgnoreSingletonsAndEmpty(t *testing.T) {
	if len(closedSets([]map[bundleKey]bool{stackKeys("a"), stackKeys("a")})) != 0 {
		t.Fatal("singletons kept")
	}
	if len(closedSets(nil)) != 0 {
		t.Fatal("empty input kept")
	}
}

func TestForwardsPairCallerAndCalleePositions(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	caller := stackFunc("caller", abcUses(1))
	stackCall(&caller, "callee", []ArgSource{paramArg(2), argConst, paramArg(0)})
	functions := []Function{callee, caller}
	sites := stackSites(functions)
	if len(sites) != 1 {
		t.Fatalf("sites: %+v", sites)
	}
	want := []forward{{from: 2, to: 0}, {from: 0, to: 2}}
	if !slices.Equal(forwards(sites[0], functions), want) {
		t.Fatalf("forwards: %+v", forwards(sites[0], functions))
	}
}

func TestForwardsIgnoreOutOfRange(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	caller := stackFunc("caller", []ParamFacts{stackParam("a", "int", 1)})
	stackCall(&caller, "callee", []ArgSource{paramArg(5), paramArg(0), argOther, argField, argConst})
	functions := []Function{callee, caller}
	sites := stackSites(functions)
	want := []forward{{from: 0, to: 1}}
	if !slices.Equal(forwards(sites[0], functions), want) {
		t.Fatalf("forwards: %+v", forwards(sites[0], functions))
	}
}

func TestStackSitesSkipDynamicAndUnresolved(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	caller := stackFunc("caller", abcUses(1))
	caller.Calls = []Call{
		{Callee: "callee", Line: 2, Local: true},
		{Callee: "callee", Line: 3, Local: true, Dynamic: true},
		{Callee: "missing", Line: 4, Local: true},
		{Callee: "fmt.Println", Line: 5},
		{Callee: "caller", Line: 6, Local: true},
	}
	sites := stackSites([]Function{callee, caller})
	if len(sites) != 1 || sites[0].caller != 1 || sites[0].callee != 0 {
		t.Fatalf("sites: %+v", sites)
	}
}

func onePlan(t *testing.T, members []int, violators map[int]bool, functions []Function) *StackPlan {
	t.Helper()
	return buildPlan(functions, stackSites(functions), members, violators)
}

func TestNoPlanWithoutFnParamsViolation(t *testing.T) {
	functions := []Function{stackFunc("lonely", abcUses(1))}
	if onePlan(t, []int{0}, map[int]bool{}, functions) != nil {
		t.Fatal("plan without violator")
	}
}

func TestPlanListsCalleeSignatureThenCallSiteEdits(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	mid := stackFunc("mid", abcUses(1))
	stackCall(&mid, "callee", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	top := stackFunc("top", nil)
	stackCall(&top, "mid", []ArgSource{argConst, argConst, argConst})
	stackCall(&top, "mid", []ArgSource{argConst, argConst, argConst})
	functions := []Function{callee, mid, top}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil || len(plan.Steps) != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	// top calls mid, not the re-signed callee: outside this plan.
	first, second := plan.Steps[0], plan.Steps[1]
	if first.Name != "callee" || !first.Signature || first.IncomingSites != 1 || first.OutgoingSites != 0 {
		t.Fatalf("first step: %+v", first)
	}
	if second.Name != "mid" || second.Signature || second.IncomingSites != 2 || second.OutgoingSites != 1 {
		t.Fatalf("second step: %+v", second)
	}
}

func TestPlanOrdersViolatingCallersAfterCallees(t *testing.T) {
	leaf := stackFunc("leaf", abcUses(1))
	mid := stackFunc("mid", abcUses(1))
	stackCall(&mid, "leaf", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	top := stackFunc("top", abcUses(1))
	stackCall(&top, "mid", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	functions := []Function{leaf, mid, top}
	violators := map[int]bool{0: true, 1: true, 2: true}
	plan := onePlan(t, []int{0, 1, 2}, violators, functions)
	if plan == nil {
		t.Fatal("no plan")
	}
	var order []string
	for _, s := range plan.Steps {
		order = append(order, s.Name)
		if !s.Signature {
			t.Fatalf("violator without signature step: %+v", s)
		}
	}
	if !slices.Equal(order, []string{"leaf", "mid", "top"}) {
		t.Fatalf("order: %v", order)
	}
}

func TestBundleIsParamsForwardedTogether(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	mid := stackFunc("mid", abcUses(1))
	stackCall(&mid, "callee", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	functions := []Function{callee, mid}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil || len(plan.Bundles) != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	bundle := plan.Bundles[0]
	var names []string
	for _, p := range bundle.Params {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"a", "b", "c"}) || bundle.Sites != 1 {
		t.Fatalf("bundle: %+v", bundle)
	}
	if !slices.Equal(bundle.Functions, []string{"callee", "mid"}) {
		t.Fatalf("bundle functions: %+v", bundle.Functions)
	}
}

func TestConstantsAndFieldsDoNotBundle(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	caller := stackFunc("caller", []ParamFacts{stackParam("x", "int", 1)})
	stackCall(&caller, "callee", []ArgSource{argConst, argField, paramArg(0)})
	functions := []Function{callee, caller}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil || len(plan.Bundles) != 0 {
		t.Fatalf("plan: %+v", plan)
	}
}

func TestBundleSupportCountsSitesAcrossCallers(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	first := stackFunc("one", abcUses(1))
	stackCall(&first, "callee", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	second := stackFunc("two", abcUses(1))
	stackCall(&second, "callee", []ArgSource{paramArg(0), paramArg(1), argConst})
	functions := []Function{callee, first, second}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil || len(plan.Bundles) != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	sizes := [][2]int{}
	for _, b := range plan.Bundles {
		sizes = append(sizes, [2]int{len(b.Params), b.Sites})
	}
	if !slices.Equal(sizes, [][2]int{{3, 1}, {2, 2}}) {
		t.Fatalf("bundle sizes: %v", sizes)
	}
}

func TestOnlyForwardedParamIsPassThrough(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	params := []ParamFacts{
		stackParam("a", "int", 1),
		stackParam("b", "bool", 2),
		stackParam("c", "string", 1),
	}
	mid := stackFunc("mid", params)
	stackCall(&mid, "callee", []ArgSource{paramArg(0), paramArg(1), argOther})
	functions := []Function{callee, mid}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil || len(plan.Steps) != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	// a is only forwarded; b is used twice but forwarded once; c never forwarded.
	if !slices.Equal(plan.Steps[1].PassThrough, []string{"a"}) {
		t.Fatalf("pass-through: %+v", plan.Steps[1].PassThrough)
	}
}

func TestUnusedParamIsNotPassThrough(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	mid := stackFunc("mid", abcUses(0))
	stackCall(&mid, "callee", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	functions := []Function{callee, mid}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil || len(plan.Steps[1].PassThrough) != 0 {
		t.Fatalf("plan: %+v", plan)
	}
}

func TestPlanIsDeterministicUnderCallerOrder(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	b := stackFunc("b", abcUses(1))
	stackCall(&b, "callee", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	a := stackFunc("a", abcUses(1))
	stackCall(&a, "callee", []ArgSource{paramArg(0), paramArg(1), paramArg(2)})
	// callers out of order in the function list; steps still sort by index.
	functions := []Function{callee, b, a}
	plan := onePlan(t, []int{0}, map[int]bool{0: true}, functions)
	if plan == nil {
		t.Fatal("no plan")
	}
	var order []string
	for _, s := range plan.Steps {
		order = append(order, s.Name)
	}
	if !slices.Equal(order, []string{"callee", "b", "a"}) {
		t.Fatalf("order: %v", order)
	}
	if plan.Bundles[0].Sites != 2 {
		t.Fatalf("bundle sites: %+v", plan.Bundles[0])
	}
}

func TestPlanFindingsOnlyForFnParams(t *testing.T) {
	callee := stackFunc("callee", abcUses(1))
	caller := stackFunc("caller", abcUses(1))
	stackCall(&caller, "callee", []ArgSource{paramArg(0)})
	functions := []Function{callee, caller}
	sites := stackSites(functions)
	diagnostics := []Diagnostic{
		{Location: caller.Location, RuleID: "fn_params"},
		{Location: caller.Location, RuleID: "fn_length"},
		{Location: callee.Location, RuleID: "fn_params"},
	}
	planFindings(functions, sites, 1, diagnostics[:2])
	planFindings(functions, sites, 0, diagnostics[2:])
	planned := 0
	for _, d := range diagnostics {
		if d.RuleID == "fn_params" && d.Plan == nil {
			t.Fatalf("missing plan: %+v", d)
		}
		if d.RuleID != "fn_params" && d.Plan != nil {
			t.Fatalf("unexpected plan: %+v", d)
		}
		if d.Plan != nil {
			planned++
		}
	}
	if planned != 2 {
		t.Fatalf("planned: %d", planned)
	}
}
