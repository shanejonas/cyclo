package patterns

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// ---- fixtures: port of candidates.rs's tests_support ----

func candSample(name string) *FuncFacts {
	return &FuncFacts{
		ID:   "k." + name,
		Name: name,
		Path: "src/lib.go",
		Line: 1,
	}
}

func candGetter(ty, name string) *FuncFacts {
	f := candSample(name)
	f.ID = "k." + ty + "." + name
	f.Name = ty + "." + name
	f.Path = "src/" + strings.ToLower(ty) + ".go"
	f.SigKey = "fn(_) -> string"
	f.SelfTy = ty
	return f
}

// emitLoopTy mirrors rstyle's pdgtest::emit_loop (`for x in xs {
// out.push(x.name()) }`).
func emitLoopTy(calleeID, sigClass string) *Pdg {
	param := func(tyClass string) PdgNode {
		n := pn(Param)
		n.TyClass = tyClass
		return n
	}
	push := pCall("vec.Push")
	push.SigClass = "fn(_, _) -> ()"
	get := pCall(calleeID)
	get.SigClass = sigClass
	return &Pdg{
		Nodes: []PdgNode{
			param("&[_]"),
			param("&[_]"),
			pn(Iterate),
			push,
			get,
			pn(Return),
		},
		Edges: []PdgEdge{
			pData(0, 2, 0),
			pData(1, 3, 0),
			pData(2, 4, 0),
			pData(4, 3, 1),
			pCtrl(2, 3, 0),
			pCtrl(2, 4, 0),
			pData(2, 5, 0),
		},
	}
}

func byteSum(s string) int {
	sum := 0
	for i := 0; i < len(s); i++ {
		sum += int(s[i])
	}
	return sum
}

func candCaller(ty, name string) *FuncFacts {
	f := candSample("emit_" + strings.ToLower(ty))
	f.Pdg = emitLoopTy("k."+ty+"."+name, "fn(_) -> string")
	f.Path = "src/emit.go"
	// Distinct lines keep site order deterministic, as in the Rust fixture.
	f.Line = 1000 - byteSum(ty)
	return f
}

func candCorpus() []*FuncFacts {
	return []*FuncFacts{
		candGetter("Dog", "bark"),
		candGetter("Cat", "meow"),
		candCaller("Dog", "bark"),
		candCaller("Cat", "meow"),
	}
}

func runCandidates(facts []*FuncFacts) Mined {
	return Mine(facts, MineGroups(facts), DefaultParams())
}

func candidateKinds(mined Mined) []CandidateKind {
	kinds := make([]CandidateKind, len(mined.Candidates))
	for i, c := range mined.Candidates {
		kinds[i] = c.Kind
	}
	return kinds
}

func TestTraitMethodFromParallelMethods(t *testing.T) {
	mined := runCandidates(candCorpus())
	if len(mined.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(mined.Candidates))
	}
	c := mined.Candidates[0]
	if c.Kind != TraitMethod {
		t.Errorf("kind = %q, want trait_method", c.Kind)
	}
	if !strings.Contains(c.Observation, "M0 = Dog.bark | Cat.meow") {
		t.Errorf("observation = %q, want M0 = Dog.bark | Cat.meow", c.Observation)
	}
	if !strings.Contains(c.PossibleRefactor, "func method() string") {
		t.Errorf("possible_refactor = %q, want func method() string", c.PossibleRefactor)
	}
	if c.Breakdown.Support != 2 {
		t.Errorf("support = %d, want 2", c.Breakdown.Support)
	}
	if c.Breakdown.CoverageMilli != 1000 {
		t.Errorf("coverage = %d, want 1000", c.Breakdown.CoverageMilli)
	}
	if len(c.Definitions) != 2 {
		t.Errorf("definitions = %d, want 2", len(c.Definitions))
	}
}

func TestEffectMismatchHalvesScore(t *testing.T) {
	clean := runCandidates(candCorpus())
	facts := candCorpus()
	facts[0].Mutates = true // Dog.bark now mutates; Cat.meow stays pure
	dirty := runCandidates(facts)
	if len(dirty.Candidates) != 1 || len(clean.Candidates) != 1 {
		t.Fatalf("candidates = %d/%d, want 1/1", len(dirty.Candidates), len(clean.Candidates))
	}
	d, c := dirty.Candidates[0], clean.Candidates[0]
	if d.Breakdown != c.Breakdown {
		t.Errorf("breakdown changed: %+v vs %+v", d.Breakdown, c.Breakdown)
	}
	if want := ScoreMilli(c.Breakdown, true); d.ScoreMilli != want {
		t.Errorf("score = %d, want halved %d", d.ScoreMilli, want)
	}
	if d.ScoreMilli >= c.ScoreMilli {
		t.Errorf("score %d not below clean %d", d.ScoreMilli, c.ScoreMilli)
	}
	found := false
	for _, ce := range d.CounterEvidence {
		if strings.Contains(ce, "effect mismatch") {
			found = true
		}
	}
	if !found {
		t.Errorf("counter evidence %q lacks effect mismatch", d.CounterEvidence)
	}
}

func TestAlreadyAbstractedSuppressed(t *testing.T) {
	facts := candCorpus()
	facts[0].Implements = true
	facts[1].Implements = true
	mined := runCandidates(facts)
	if len(mined.Candidates) != 0 {
		t.Errorf("candidates = %d, want 0", len(mined.Candidates))
	}
	if len(mined.Suppressed) != 1 {
		t.Fatalf("suppressed = %d, want 1", len(mined.Suppressed))
	}
	// Go adaptation: Implements is a bool, so the suppression names no
	// interface method id (rstyle: "all 2 impls are of trait method k::Speak::speak").
	if !strings.Contains(mined.Suppressed[0].Reason, "interface method") {
		t.Errorf("reason = %q, want it to name the interface method", mined.Suppressed[0].Reason)
	}
}

func TestScoreMilliMath(t *testing.T) {
	cases := []struct {
		name     string
		b        Breakdown
		mismatch bool
		want     uint32
	}{
		// (1000·ln2 + 0) × 1 = 693.1 → 693
		{"support", Breakdown{Support: 2, CoverageMilli: 1000}, false, 693},
		{"halved", Breakdown{Support: 2, CoverageMilli: 1000}, true, 347},
		{"no damping at 3 holes", Breakdown{Support: 2, Holes: 3, CoverageMilli: 1000}, false, 693},
		// 693.1 × 3/6 = 346.6 → 347
		{"damped past 3 holes", Breakdown{Support: 2, Holes: 6, CoverageMilli: 1000}, false, 347},
		// ln1 = 0: lift alone
		{"lift only", Breakdown{Support: 1, LiftMilli: 500, CoverageMilli: 1000}, false, 500},
		// coverage scales
		{"coverage", Breakdown{Support: 2, CoverageMilli: 500}, false, 347},
	}
	for _, tc := range cases {
		if got := ScoreMilli(tc.b, tc.mismatch); got != tc.want {
			t.Errorf("%s: ScoreMilli = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestLastTwo(t *testing.T) {
	if got := lastTwo("a.net.TcpStream.connect"); got != "TcpStream.connect" {
		t.Errorf("lastTwo = %q", got)
	}
	if got := lastTwo("connect"); got != "connect" {
		t.Errorf("lastTwo = %q", got)
	}
}

func TestShortGoPaths(t *testing.T) {
	if got := shortGoPaths("a.b.W[c.X, d.Y]"); got != "W[X, Y]" {
		t.Errorf("shortGoPaths = %q", got)
	}
	if got := shortGoPaths("k.Dog"); got != "Dog" {
		t.Errorf("shortGoPaths = %q", got)
	}
}

func testEvidence(group *SigGroup, defs []*FuncFacts, existingTrait string) *candidateEvidence {
	return &candidateEvidence{
		kind:          TraitMethod,
		breakdown:     Breakdown{Support: 2, LiftMilli: 0, Holes: 1, CoverageMilli: 1000},
		verdicts:      []traitVerdict{{group: group, defs: defs}},
		existingTrait: existingTrait,
	}
}

func TestTraitTextProposesNewInterface(t *testing.T) {
	facts := []*FuncFacts{candGetter("Dog", "bark"), candGetter("Cat", "meow")}
	groups := MineGroups(facts)
	_, refactor := traitText(testEvidence(&groups[0], facts, ""))
	if !strings.HasPrefix(refactor, "interface Shared {") {
		t.Errorf("refactor = %q", refactor)
	}
}

func TestTraitTextExtendsExistingInterface(t *testing.T) {
	facts := []*FuncFacts{candGetter("Dog", "bark"), candGetter("Cat", "meow")}
	groups := MineGroups(facts)
	_, refactor := traitText(testEvidence(&groups[0], facts, "k.public.Signature.is_valid"))
	if !strings.HasPrefix(refactor, "add func method() string to existing interface `Signature`") {
		t.Errorf("refactor = %q", refactor)
	}
	if strings.Contains(refactor, "interface Shared") {
		t.Errorf("refactor proposes a new interface: %q", refactor)
	}
}

func TestExistingTraitForMatchesSameOwners(t *testing.T) {
	dog := candGetter("Dog", "bark")
	dog.SelfTy = "k.Dog"
	cat := candGetter("Cat", "meow")
	cat.SelfTy = "k.Cat"
	facts := []*FuncFacts{dog, cat}
	groups := MineGroups(facts)
	defs := facts
	verdicts := []traitVerdict{{group: &groups[0], defs: defs}}
	hidden := []existing{
		{traitMethod: "k.Speak.speak", owners: map[string]bool{"k.Dog": true, "k.Cat": true}},
		{traitMethod: "k.Key.public", owners: map[string]bool{"k.Ec": true, "k.Rsa": true}},
	}
	if got := existingTraitFor(verdicts, hidden); got != "k.Speak.speak" {
		t.Errorf("existingTraitFor = %q", got)
	}
	if got := existingTraitFor(verdicts, hidden[1:]); got != "" {
		t.Errorf("existingTraitFor = %q, want empty", got)
	}
}

// typeOnlyCaller mirrors the Rust test's intent: callers that differ only in
// the element type, both calling one shared helper. Go adaptation: rstyle's
// raw type shapes differ on the params; the Go schema erases element types
// to "_", so the element type rides on the Iterate node, whose WL label
// hides TyClass (Param labels include it, so differing params would never
// pair and no hole would form).
func typeOnlyCaller(elemTy string) *FuncFacts {
	f := candCaller("X", "x")
	f.ID = "k.emit_" + strings.ToLower(elemTy)
	f.Name = "emit_" + strings.ToLower(elemTy)
	f.Line = 1000 - byteSum(elemTy)
	f.Pdg.Nodes[2].TyClass = elemTy
	f.Pdg.Nodes[4] = pCall("k.Shared.get")
	f.Pdg.Nodes[4].SigClass = "fn(_) -> string"
	return f
}

func typeOnlyCorpus() []*FuncFacts {
	facts := []*FuncFacts{typeOnlyCaller("Dog"), typeOnlyCaller("Cat")}
	return append(facts, candGetter("Dog", "own"), candGetter("Cat", "own"))
}

func TestTypeOnlyClusterProposesGenericFn(t *testing.T) {
	mined := runCandidates(typeOnlyCorpus())
	if len(mined.Candidates) != 1 {
		t.Fatalf("candidates = %+v, want 1", candidateKinds(mined))
	}
	c := mined.Candidates[0]
	if c.Kind != GenericFn {
		t.Errorf("kind = %q, want generic_fn", c.Kind)
	}
	if !strings.Contains(c.PossibleRefactor, "T0 = Dog | Cat") {
		t.Errorf("possible_refactor = %q, want T0 = Dog | Cat", c.PossibleRefactor)
	}
	if c.Breakdown.LiftMilli != 0 {
		t.Errorf("lift = %d, want 0", c.Breakdown.LiftMilli)
	}
	if want := ScoreMilli(c.Breakdown, false) / 2; c.ScoreMilli != want {
		t.Errorf("score = %d, want halved %d", c.ScoreMilli, want)
	}
}

func TestExternalTypeHolesShownToo(t *testing.T) {
	facts := typeOnlyCorpus()
	for i, f := range facts[:2] {
		ext := []string{"ext.Tcp", "ext.Unix"}[i]
		field := pn(Field)
		field.TyClass = ext
		f.Pdg.Nodes = append(f.Pdg.Nodes, field)
		last := len(f.Pdg.Nodes) - 1
		f.Pdg.Edges = append(f.Pdg.Edges, pData(2, last, 0))
	}
	c := runCandidates(facts).Candidates[0]
	if !strings.Contains(c.PossibleRefactor, "T0 = Dog | Cat") {
		t.Errorf("possible_refactor = %q, want T0 = Dog | Cat", c.PossibleRefactor)
	}
	if !strings.Contains(c.PossibleRefactor, "Tcp | Unix") {
		t.Errorf("possible_refactor = %q, want external Tcp | Unix", c.PossibleRefactor)
	}
}

// externalOnly mirrors the Rust fixture: callers that differ only in an
// external callee: no workspace type, no trait.
func externalOnly(callee string) *FuncFacts {
	f := candCaller("Dog", "x")
	f.Pdg.Nodes[4] = pCall(callee)
	f.Pdg.Nodes[4].SigClass = "fn(_) -> string"
	return f
}

func TestExternalCalleeOnlyProposesParameterizedHelper(t *testing.T) {
	facts := []*FuncFacts{externalOnly("ext.A.go"), externalOnly("ext.B.go")}
	mined := runCandidates(facts)
	if len(mined.Candidates) != 1 {
		t.Fatalf("candidates = %+v, want 1", candidateKinds(mined))
	}
	c := mined.Candidates[0]
	if c.Kind != Parameterize {
		t.Errorf("kind = %q, want parameterize", c.Kind)
	}
	if !strings.Contains(c.PossibleRefactor, "A.go | B.go") {
		t.Errorf("possible_refactor = %q, want A.go | B.go", c.PossibleRefactor)
	}
	if want := ScoreMilli(c.Breakdown, false) / 3; c.ScoreMilli != want {
		t.Errorf("score = %d, want thirded %d", c.ScoreMilli, want)
	}
}

func TestExternalCalleeHolesKeepTheirOwner(t *testing.T) {
	facts := typeOnlyCorpus()
	for i, f := range facts[:2] {
		owner := []string{"Tcp", "Unix"}[i]
		get := pCall("ext." + owner + ".connect")
		get.SigClass = "fn(_) -> string"
		f.Pdg.Nodes[4] = get
	}
	c := runCandidates(facts).Candidates[0]
	if !strings.Contains(c.Observation, "M0 = Tcp.connect | Unix.connect") {
		t.Errorf("observation = %q, want M0 = Tcp.connect | Unix.connect", c.Observation)
	}
}

func TestParameterizeSkips(t *testing.T) {
	pair := func() []*FuncFacts {
		return []*FuncFacts{externalOnly("ext.A.go"), externalOnly("ext.B.go")}
	}
	tests := pair()
	for _, f := range tests {
		f.ID = "k.tests." + f.ID
	}
	if got := runCandidates(tests); len(got.Candidates) != 0 {
		t.Errorf("test code: candidates = %d, want 0", len(got.Candidates))
	}
	forced := pair()
	for _, f := range forced {
		f.Implements = true
	}
	if got := runCandidates(forced); len(got.Candidates) != 0 {
		t.Errorf("trait impls: candidates = %d, want 0", len(got.Candidates))
	}
	columns := make([]Column, 0, DefaultParams().MaxHoles+1)
	for i := 0; i <= DefaultParams().MaxHoles; i++ {
		columns = append(columns, Column{Kind: HoleLiteral, Var: fmt.Sprintf("L%d", i)})
	}
	cluster := &Cluster{
		Members:       []int{0, 1},
		CoverageMilli: []uint32{1000, 1000},
		Columns:       columns,
	}
	sites := pair()
	cap := DefaultParams().MaxHoles
	if parameterizable(sites, cluster, cap) {
		t.Errorf("wide cluster admitted at cap %d", cap)
	}
	if !parameterizable(sites, cluster, cap+1) {
		t.Errorf("wide cluster rejected at raised cap %d", cap+1)
	}
	for _, id := range []string{"k.main", "k.raw.wrap", "k.ffi.wrap", "k.bindgen_test_layout_x"} {
		f := externalOnly("ext.A.go")
		f.ID = id
		if !isBoilerplate(f) {
			t.Errorf("isBoilerplate(%q) = false", id)
		}
	}
}

func TestGenericFnSkipsTestCode(t *testing.T) {
	facts := typeOnlyCorpus()
	for _, f := range facts[:2] {
		f.ID = "k.tests." + f.ID
	}
	if got := runCandidates(facts); len(got.Candidates) != 0 {
		t.Errorf("candidates = %d, want 0", len(got.Candidates))
	}
}

func TestTypeOnlyClusterOfOneCallBodiesIsNotEmitted(t *testing.T) {
	facts := typeOnlyCorpus()
	for _, f := range facts[:2] {
		// Drop the push call: only the shared helper call is left.
		f.Pdg.Nodes[3] = pn(Param)
	}
	if got := runCandidates(facts); len(got.Candidates) != 0 {
		t.Errorf("candidates = %d, want 0", len(got.Candidates))
	}
}

func guardedParams() Params {
	p := DefaultParams()
	p.SingleCallGuard = true
	return p
}

// oneCallBodies mirrors the Rust fixture: each body keeps one call.
func oneCallBodies(input NodeKind) []*FuncFacts {
	facts := typeOnlyCorpus()
	for _, f := range facts[:2] {
		f.Pdg.Nodes[3] = pn(Param)
		f.Pdg.Nodes[2] = pn(input)
	}
	return facts
}

func TestGuardEmitsOneCallBodiesThatDoRealWork(t *testing.T) {
	facts := oneCallBodies(Iterate)
	if got := Mine(facts, MineGroups(facts), DefaultParams()); len(got.Candidates) != 0 {
		t.Errorf("unguarded: candidates = %d, want 0", len(got.Candidates))
	}
	if got := Mine(facts, MineGroups(facts), guardedParams()); len(got.Candidates) != 1 {
		t.Errorf("guarded: candidates = %d, want 1", len(got.Candidates))
	}
}

func TestGuardStillRejectsPureParameterForwards(t *testing.T) {
	facts := oneCallBodies(Param)
	if got := Mine(facts, MineGroups(facts), guardedParams()); len(got.Candidates) != 0 {
		t.Errorf("candidates = %d, want 0", len(got.Candidates))
	}
}

func TestTypeOnlyClusterWithLiteralHoleIsNotAGenericFn(t *testing.T) {
	facts := typeOnlyCorpus()
	for i, f := range facts[:2] {
		lit := pLit("str", []string{"alpha", "beta"}[i])
		f.Pdg.Nodes = append(f.Pdg.Nodes, lit)
		last := len(f.Pdg.Nodes) - 1
		f.Pdg.Edges = append(f.Pdg.Edges, pData(last, 3, 2))
	}
	if kinds := candidateKinds(runCandidates(facts)); slices.Contains(kinds, GenericFn) {
		t.Errorf("kinds = %v, want no generic_fn", kinds)
	}
}

func TestWorkspaceCalleeHoleIsNotAGenericFn(t *testing.T) {
	facts := candCorpus()
	facts = append(facts, candGetter("Cow", "moo"))
	for _, f := range facts[:2] {
		f.SelfTy = ""
	}
	if kinds := candidateKinds(runCandidates(facts)); slices.Contains(kinds, GenericFn) {
		t.Errorf("kinds = %v, want no generic_fn", kinds)
	}
}

func TestTypeHoleOnExternalTypesIsNotAGenericFn(t *testing.T) {
	facts := typeOnlyCorpus()[:2]
	if kinds := candidateKinds(runCandidates(facts)); slices.Contains(kinds, GenericFn) {
		t.Errorf("kinds = %v, want no generic_fn", kinds)
	}
}

func TestConstructorColumnsNeverBecomeTraitCandidates(t *testing.T) {
	facts := candCorpus()
	for _, f := range facts[:2] {
		f.SelfTy = ""
	}
	kinds := candidateKinds(runCandidates(facts))
	if slices.Contains(kinds, TraitMethod) {
		t.Errorf("kinds = %v, want no trait_method", kinds)
	}
	if slices.Contains(kinds, CapabilitySet) {
		t.Errorf("kinds = %v, want no capability_set", kinds)
	}
}

func TestProvenanceListsEverySite(t *testing.T) {
	facts := candCorpus()
	facts = append(facts, candGetter("Cow", "moo"), candCaller("Cow", "moo"))
	mined := runCandidates(facts)
	if len(mined.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	c := mined.Candidates[0]
	if len(c.Sites) != 3 {
		t.Errorf("sites = %d, want 3", len(c.Sites))
	}
	for _, s := range c.Sites {
		if s.Line <= 0 || s.Path != "src/emit.go" {
			t.Errorf("site = %+v, want src/emit.go with a line", s)
		}
	}
	if len(c.Definitions) != 3 {
		t.Errorf("definitions = %d, want 3", len(c.Definitions))
	}
	if c.Breakdown.Support != 3 {
		t.Errorf("support = %d, want 3", c.Breakdown.Support)
	}
}

func TestSizeRatioFilterAppliedBeforeWL(t *testing.T) {
	facts := candCorpus()
	big := candCaller("Cow", "moo")
	for i := 0; i < 12; i++ {
		push := pCall("vec.Push")
		push.SigClass = "fn(_, _) -> ()"
		big.Pdg.Nodes = append(big.Pdg.Nodes, push)
	}
	facts = append(facts, candGetter("Cow", "moo"), big)
	mined := runCandidates(facts)
	if len(mined.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	if got := len(mined.Candidates[0].Sites); got != 2 {
		t.Errorf("sites = %d, want 2: the oversized look-alike must not join", got)
	}
}

func TestCommonSignatureScoresBelowRareOne(t *testing.T) {
	var others []*FuncFacts
	for i := 0; i < 12; i++ {
		f := candSample(fmt.Sprintf("solo%d", i))
		f.SigKey = fmt.Sprintf("fn(_) -> p%d", i)
		f.SelfTy = "Solo"
		others = append(others, f)
	}
	rareFacts := append(candCorpus(), others...)
	var crowd []*FuncFacts
	for i := 0; i < 40; i++ {
		crowd = append(crowd, candGetter(fmt.Sprintf("T%d", i), "label"))
	}
	crowd = append(crowd, rareFacts...)
	common := runCandidates(crowd).Candidates[0]
	rare := runCandidates(rareFacts).Candidates[0]
	if !(common.ScoreMilli < rare.ScoreMilli) {
		t.Errorf("common %d not below rare %d", common.ScoreMilli, rare.ScoreMilli)
	}
	if !(common.Breakdown.LiftMilli < 500) {
		t.Errorf("common lift %d not below 500", common.Breakdown.LiftMilli)
	}
}

func TestEnumDispatchProposesDispatchTable(t *testing.T) {
	dispatcher := candSample("process")
	dispatcher.Path = "src/process.go"
	dispatcher.Line = 10
	dispatcher.EndLine = 30
	dispatcher.EnumDispatches = []EnumDispatchHit{{Line: 15, NumCases: 3}}
	facts := []*FuncFacts{dispatcher}
	report := Run(facts, Options{})
	var found *Candidate
	for i, c := range report.Candidates {
		if c.Kind == EnumDispatch {
			found = &report.Candidates[i]
		}
	}
	if found == nil {
		t.Fatalf("candidates = %+v, want enum_dispatch", candidateKindsFromReport(report))
	}
	if !strings.Contains(found.Observation, "15") {
		t.Errorf("observation = %q, want switch line", found.Observation)
	}
	if found.FixSpec == nil || found.FixSpec.Line != 15 {
		t.Errorf("FixSpec should point at the switch line 15, got %+v", found.FixSpec)
	}
}

func candidateKindsFromReport(report PatternsReport) []CandidateKind {
	kinds := make([]CandidateKind, len(report.Candidates))
	for i, c := range report.Candidates {
		kinds[i] = c.Kind
	}
	return kinds
}

func TestFixKindOrder(t *testing.T) {
	// The dependency order: guards simplify control flow first,
	// type-creating passes next, parameterize last (PDG-sensitive).
	order := []CandidateKind{
		GuardClause, PrimitiveObsession, ValueObject, Factory,
		Parameterize,
	}
	for i := 1; i < len(order); i++ {
		if FixKindRank(order[i-1]) >= FixKindRank(order[i]) {
			t.Errorf("FixKindRank(%q)=%d not before FixKindRank(%q)=%d",
				order[i-1], FixKindRank(order[i-1]), order[i], FixKindRank(order[i]))
		}
	}
	// Detection-only kinds (never produce FixSpecs) sort after all
	// fixable kinds.
	lastFixable := FixKindRank(Parameterize)
	for _, k := range []CandidateKind{MutableIdentity, Aggregate, Repository} {
		if FixKindRank(k) <= lastFixable {
			t.Errorf("FixKindRank(%q)=%d, want after %d (fixable kinds first)",
				k, FixKindRank(k), lastFixable)
		}
	}
	// Every fixable kind is in the order.
	for _, k := range []CandidateKind{
		GuardClause, PrimitiveObsession, ValueObject, Factory,
		MissingIdentity, EntityIdentity, AnemicModel, TypeSwitch,
		EnumDispatch, TraitMethod, CapabilitySet, GenericFn, Parameterize,
	} {
		if FixKindRank(k) >= len(FixKindOrder) {
			t.Errorf("fixable kind %q not in FixKindOrder", k)
		}
	}
}
