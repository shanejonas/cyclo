package quality

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func groupFunction(name string, line int, calls ...string) Function {
	typed := make([]Call, 0, len(calls))
	for _, callee := range calls {
		typed = append(typed, Call{Callee: "example.com/mod." + callee, Line: line + 1, Local: true})
	}
	return groupFunctionCalls(name, line, typed...)
}

func groupFunctionCalls(name string, line int, calls ...Call) Function {
	return Function{Location: Location{Path: "a.go", Line: line, Column: 1, Name: "example.com/mod." + name}, Calls: calls}
}

func groupDiagnostic(function Function, rule string) Diagnostic {
	return Diagnostic{Location: function.Location, RuleID: rule, Actual: 2, Limit: 1, Message: rule}
}

func withGrouping(rules []string, hops int) Config {
	config := DefaultConfig()
	config.Grouping = Grouping{CallerRules: rules, CallerHops: hops}
	return config
}

func buildTestGroups(t *testing.T, facts []Function, flags map[int][]string, config Config) []FixGroup {
	t.Helper()
	var diagnostics []Diagnostic
	for _, index := range slices.Sorted(maps.Keys(flags)) {
		for _, rule := range flags[index] {
			diagnostics = append(diagnostics, groupDiagnostic(facts[index], rule))
		}
	}
	groups, err := BuildFixGroups(facts, diagnostics, config)
	if err != nil {
		t.Fatal(err)
	}
	return groups
}

func shortName(qualified string) string {
	_, name, _ := strings.Cut(qualified, "example.com/mod.")
	return name
}

func groupMemberNames(groups []FixGroup) [][]string {
	names := make([][]string, len(groups))
	for i, group := range groups {
		for _, function := range group.Functions {
			names[i] = append(names[i], shortName(function.Name))
		}
	}
	return names
}

func groupStacked(groups []FixGroup) []bool {
	stacked := make([]bool, len(groups))
	for i, group := range groups {
		stacked[i] = group.Stacked
	}
	return stacked
}

func TestBuildFixGroups(t *testing.T) {
	defaults := DefaultConfig()
	tests := []struct {
		name        string
		facts       []Function
		flags       map[int][]string
		config      Config
		wantMembers [][]string
		wantStacked []bool
	}{
		{
			name:        "unrelated violations are independent",
			facts:       []Function{groupFunction("a", 1), groupFunction("b", 10)},
			flags:       map[int][]string{0: {"fn_length"}, 1: {"fn_length"}},
			config:      defaults,
			wantMembers: [][]string{{"a"}, {"b"}},
			wantStacked: []bool{false, false},
		},
		{
			name:        "signature fix groups the function with its callers callee first",
			facts:       []Function{groupFunction("caller", 1, "callee"), groupFunction("callee", 10)},
			flags:       map[int][]string{0: {"fn_length"}, 1: {"fn_params"}},
			config:      defaults,
			wantMembers: [][]string{{"callee", "caller"}},
			wantStacked: []bool{true},
		},
		{
			name:        "non-signature rules do not reach callers",
			facts:       []Function{groupFunction("caller", 1, "callee"), groupFunction("callee", 10)},
			flags:       map[int][]string{0: {"fn_length"}, 1: {"side_effect_density"}},
			config:      defaults,
			wantMembers: [][]string{{"caller"}, {"callee"}},
			wantStacked: []bool{false, false},
		},
		{
			name:        "caller rules are configurable",
			facts:       []Function{groupFunction("caller", 1, "callee"), groupFunction("callee", 10)},
			flags:       map[int][]string{0: {"fn_length"}, 1: {"side_effect_density"}},
			config:      withGrouping([]string{"side_effect_density"}, 1),
			wantMembers: [][]string{{"callee", "caller"}},
			wantStacked: []bool{true},
		},
		{
			name:        "shared non-violating caller links two signature fixes without joining",
			facts:       []Function{groupFunction("hub", 1, "x", "y"), groupFunction("x", 10), groupFunction("y", 20)},
			flags:       map[int][]string{1: {"fn_params"}, 2: {"fn_params"}},
			config:      defaults,
			wantMembers: [][]string{{"x", "y"}},
			wantStacked: []bool{true},
		},
		{
			name:        "one hop does not reach the grandcaller",
			facts:       []Function{groupFunction("top", 1, "mid"), groupFunction("mid", 10, "leaf"), groupFunction("leaf", 20)},
			flags:       map[int][]string{0: {"fn_length"}, 2: {"fn_params"}},
			config:      defaults,
			wantMembers: [][]string{{"top"}, {"leaf"}},
			wantStacked: []bool{false, false},
		},
		{
			name:        "caller hops widen the footprint",
			facts:       []Function{groupFunction("top", 1, "mid"), groupFunction("mid", 10, "leaf"), groupFunction("leaf", 20)},
			flags:       map[int][]string{0: {"fn_length"}, 2: {"fn_params"}},
			config:      withGrouping([]string{"fn_params"}, 2),
			wantMembers: [][]string{{"top", "leaf"}},
			wantStacked: []bool{true},
		},
		{
			name:        "cycles order deterministically",
			facts:       []Function{groupFunction("a", 1, "b"), groupFunction("b", 10, "a")},
			flags:       map[int][]string{0: {"fn_params"}, 1: {"fn_params"}},
			config:      defaults,
			wantMembers: [][]string{{"a", "b"}},
			wantStacked: []bool{true},
		},
		{
			name:        "no violations no groups",
			facts:       []Function{groupFunction("a", 1)},
			flags:       map[int][]string{},
			config:      defaults,
			wantMembers: nil,
			wantStacked: nil,
		},
		{
			name: "dynamic and external calls do not link",
			facts: []Function{
				groupFunctionCalls("a", 1,
					Call{Callee: "example.com/mod.b", Line: 2, Local: true, Dynamic: true},
					Call{Callee: "other/mod.b", Line: 3},
				),
				groupFunction("b", 10),
			},
			flags:       map[int][]string{0: {"fn_params"}, 1: {"fn_params"}},
			config:      defaults,
			wantMembers: [][]string{{"a"}, {"b"}},
			wantStacked: []bool{false, false},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			groups := buildTestGroups(t, test.facts, test.flags, test.config)
			if len(groups) != len(test.wantMembers) {
				t.Fatalf("got %d groups %v, want %d groups %v", len(groups), groupMemberNames(groups), len(test.wantMembers), test.wantMembers)
			}
			for i, group := range groups {
				if group.ID != i+1 {
					t.Errorf("group %d has ID %d", i, group.ID)
				}
				members := groupMemberNames([]FixGroup{group})[0]
				if !slices.Equal(members, test.wantMembers[i]) {
					t.Errorf("group %d members %v, want %v", group.ID, members, test.wantMembers[i])
				}
				if group.Stacked != test.wantStacked[i] {
					t.Errorf("group %d stacked %v, want %v", group.ID, group.Stacked, test.wantStacked[i])
				}
				for _, function := range group.Functions {
					if !slices.IsSorted(function.Rules) {
						t.Errorf("group %d rules %v are not sorted", group.ID, function.Rules)
					}
				}
			}
			if !slices.Equal(groupStacked(groups), test.wantStacked) {
				t.Errorf("stacked %v, want %v", groupStacked(groups), test.wantStacked)
			}
		})
	}
}

func TestBuildFixGroupsRejectsConflictingFacts(t *testing.T) {
	first := groupFunction("a", 1)
	second := groupFunction("a", 1)
	second.Params = 99
	if _, err := BuildFixGroups([]Function{first, second}, nil, DefaultConfig()); err == nil {
		t.Fatal("conflicting facts accepted")
	}
}

func TestGroupingConfigValidation(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default grouping rejected: %v", err)
	}
	empty := withGrouping(nil, 0)
	if err := empty.Validate(); err != nil {
		t.Fatalf("empty caller rules rejected: %v", err)
	}
	typo := withGrouping([]string{"fn_param"}, 1)
	if err := typo.Validate(); err == nil {
		t.Fatal("unknown caller rule accepted")
	}
	negative := withGrouping([]string{"fn_params"}, -1)
	if err := negative.Validate(); err == nil {
		t.Fatal("negative caller hops accepted")
	}
}
