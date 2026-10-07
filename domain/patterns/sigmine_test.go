package patterns

import (
	"fmt"
	"reflect"
	"testing"
)

// stringMethod mirrors sigmine.rs's test fixture: a getter-like method with
// a normalized signature shared across types, one owner per concrete SelfTy.
func stringMethod(selfTy, name string) *FuncFacts {
	return &FuncFacts{
		ID:     "k::" + selfTy + "." + name,
		Name:   selfTy + "." + name,
		Path:   "src/example.go",
		Line:   1,
		SigKey: "fn(&Self) -> String",
		SelfTy: selfTy,
	}
}

func selfGetter(selfTy, name, retClass string) *FuncFacts {
	return &FuncFacts{
		ID:     "k::" + selfTy + "." + name,
		Name:   selfTy + "." + name,
		Path:   "src/example.go",
		Line:   1,
		SigKey: "fn(&Self) -> " + retClass,
		SelfTy: selfTy,
	}
}

func traited(f *FuncFacts) *FuncFacts {
	f.Implements = true
	return f
}

func TestMineGroupNeedsTwoDistinctSelfTypes(t *testing.T) {
	sameType := []*FuncFacts{stringMethod("Dog", "bark"), stringMethod("Dog", "howl")}
	if got := Mine(sameType); len(got) != 0 {
		t.Fatalf("same self type must not form a group, got %v", got)
	}
	two := []*FuncFacts{stringMethod("Dog", "bark"), stringMethod("Cat", "meow")}
	groups := Mine(two)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].SelfTypes != 2 {
		t.Fatalf("expected 2 self types, got %d", groups[0].SelfTypes)
	}
	if groups[0].Key != "fn(&Self) -> String" {
		t.Fatalf("unexpected key %q", groups[0].Key)
	}
}

func TestMineTraitImplMembersAreExcludedButCounted(t *testing.T) {
	facts := []*FuncFacts{
		stringMethod("Dog", "bark"),
		stringMethod("Cat", "meow"),
		traited(stringMethod("Cow", "speak")),
		traited(stringMethod("Hen", "speak")),
	}
	groups := Mine(facts)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	group := groups[0]
	if len(group.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(group.Members))
	}
	if len(group.Traited) != 2 {
		t.Fatalf("expected 2 traited, got %d", len(group.Traited))
	}
	// Counted in the base rate: 4 of 4 functions share the signature.
	if group.SpecificityMilli != 0 {
		t.Fatalf("expected specificity 0, got %d", group.SpecificityMilli)
	}
	onlyTraited := []*FuncFacts{
		traited(stringMethod("Cow", "speak")),
		traited(stringMethod("Hen", "speak")),
	}
	if got := Mine(onlyTraited); len(got) != 0 {
		t.Fatalf("traited-only bucket must not form a group, got %v", got)
	}
}

func TestMineRareSignatureOutranksGetterSignature(t *testing.T) {
	var facts []*FuncFacts
	for i := 0; i < 10; i++ {
		facts = append(facts, stringMethod(fmt.Sprintf("T%d", i), fmt.Sprintf("label%d", i)))
	}
	facts = append(facts, selfGetter("Dog", "chase", "Self"))
	facts = append(facts, selfGetter("Cat", "chase", "Self"))
	for i := 0; i < 20; i++ {
		facts = append(facts, selfGetter("Solo", fmt.Sprintf("m%d", i), fmt.Sprintf("P%d", i)))
	}
	groups := Mine(facts)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Key != "fn(&Self) -> Self" {
		t.Fatalf("expected rare signature first, got %q", groups[0].Key)
	}
	if !(groups[0].ScoreMilli > groups[1].ScoreMilli) {
		t.Fatalf("expected group 0 to outscore group 1, got %d vs %d",
			groups[0].ScoreMilli, groups[1].ScoreMilli)
	}
	if !groups[0].SameName {
		t.Fatal("expected SameName on the rare signature group")
	}
	if groups[1].SameName {
		t.Fatal("expected no SameName on the getter-like group")
	}
}

func TestMineSameNameBoostApplies(t *testing.T) {
	facts := []*FuncFacts{
		stringMethod("Dog", "name"),
		stringMethod("Cat", "name"),
		selfGetter("Rock", "x", "Other"),
	}
	groups := Mine(facts)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if !g.SameName {
		t.Fatal("expected SameName")
	}
	if want := g.SpecificityMilli + g.SpecificityMilli/2; g.ScoreMilli != want {
		t.Fatalf("expected score %d, got %d", want, g.ScoreMilli)
	}
}

func TestMineBoostScalesWithShareOfTypesThatShareTheName(t *testing.T) {
	facts := []*FuncFacts{
		stringMethod("Dog", "name"),
		stringMethod("Cat", "name"),
		stringMethod("Cow", "moo"),
		selfGetter("Rock", "x", "Other"),
		selfGetter("Rock", "y", "Other"),
		selfGetter("Rock", "z", "Other"),
	}
	groups := Mine(facts)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if !g.SameName {
		t.Fatal("expected SameName")
	}
	if g.SpecificityMilli == 0 {
		t.Fatal("expected positive specificity")
	}
	// 2 of 3 types share the name: integer math mirrors the Rust formula.
	want := g.SpecificityMilli + g.SpecificityMilli/2*2/3
	if g.ScoreMilli != want {
		t.Fatalf("expected score %d, got %d", want, g.ScoreMilli)
	}
}

func TestMineSameSelfTyWithDifferentConcreteArgsFormsAGroup(t *testing.T) {
	facts := []*FuncFacts{stringMethod("Wrapper[u8]", "name"), stringMethod("Wrapper[u16]", "name")}
	groups := Mine(facts)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].SelfTypes != 2 {
		t.Fatalf("expected 2 self types, got %d", groups[0].SelfTypes)
	}
	if !groups[0].SameName {
		t.Fatal("expected SameName")
	}
}

func TestMineSameSelfTyWithParamArgsOnlyIsOneOwner(t *testing.T) {
	facts := []*FuncFacts{stringMethod("Wrapper[T0]", "name"), stringMethod("Wrapper[T0]", "name2")}
	if got := Mine(facts); len(got) != 0 {
		t.Fatalf("one generic owner must not form a group, got %v", got)
	}
}

func TestMineOutputIndependentOfInputOrder(t *testing.T) {
	facts := []*FuncFacts{
		stringMethod("Dog", "bark"),
		stringMethod("Cat", "meow"),
		selfGetter("Dog", "chase", "Self"),
		selfGetter("Cat", "chase", "Self"),
		traited(stringMethod("Cow", "speak")),
	}
	forward := Mine(facts)
	reversed := append([]*FuncFacts{}, facts...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if !reflect.DeepEqual(forward, Mine(reversed)) {
		t.Fatalf("output depends on input order:\n%v\n%v", forward, Mine(reversed))
	}
}

func TestShortName(t *testing.T) {
	cases := map[string]string{
		"Dog.bark":  "bark",
		"pkg.Dog.m": "m",
		"bark":      "bark",
	}
	for in, want := range cases {
		if got := shortName(in); got != want {
			t.Fatalf("shortName(%q) = %q, want %q", in, got, want)
		}
	}
}
