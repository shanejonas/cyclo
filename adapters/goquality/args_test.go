package goquality

import (
	"context"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

func argsFacts(t *testing.T) []quality.Function {
	t.Helper()
	facts, err := (Analyzer{Root: "testdata/args"}).Extract(context.Background(), []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func findCall(t *testing.T, f quality.Function, calleeSuffix string, occurrence int) quality.Call {
	t.Helper()
	seen := 0
	for _, c := range f.Calls {
		if len(c.Callee) >= len(calleeSuffix) && c.Callee[len(c.Callee)-len(calleeSuffix):] == calleeSuffix {
			if seen == occurrence {
				return c
			}
			seen++
		}
	}
	t.Fatalf("call %s #%d missing in %+v", calleeSuffix, occurrence, f.Calls)
	return quality.Call{}
}

func TestArgProvenanceClassification(t *testing.T) {
	facts := argsFacts(t)
	caller := findFunction(t, facts, "caller")
	cases := []struct {
		callee string
		n      int
		want   []quality.ArgSource
	}{
		{"target", 0, []quality.ArgSource{{Kind: quality.ArgParam, Param: 0}, {Kind: quality.ArgConst}, {Kind: quality.ArgConst}}},
		{"target", 1, []quality.ArgSource{{Kind: quality.ArgParam, Param: 0}, {Kind: quality.ArgParam, Param: 1}, {Kind: quality.ArgField}}},
		{"target", 2, []quality.ArgSource{{Kind: quality.ArgConst}, {Kind: quality.ArgParam, Param: 1}, {Kind: quality.ArgConst}}},
		{"targetPtr", 0, []quality.ArgSource{{Kind: quality.ArgParam, Param: 0}, {Kind: quality.ArgParam, Param: 1}}},
		{"target", 3, []quality.ArgSource{{Kind: quality.ArgOther}, {Kind: quality.ArgParam, Param: 1}, {Kind: quality.ArgConst}}},
		{"target", 4, []quality.ArgSource{{Kind: quality.ArgOther}, {Kind: quality.ArgParam, Param: 1}, {Kind: quality.ArgConst}}},
	}
	for _, c := range cases {
		call := findCall(t, caller, c.callee, c.n)
		if len(call.Args) != len(c.want) {
			t.Fatalf("%s #%d args: %+v, want %+v", c.callee, c.n, call.Args, c.want)
		}
		for i, want := range c.want {
			if call.Args[i] != want {
				t.Fatalf("%s #%d arg %d: %+v, want %+v", c.callee, c.n, i, call.Args[i], want)
			}
		}
	}
}

func TestArgFieldThroughReceiver(t *testing.T) {
	facts := argsFacts(t)
	method := findFunction(t, facts, "method")
	call := findCall(t, method, "target", 0)
	want := []quality.ArgSource{{Kind: quality.ArgConst}, {Kind: quality.ArgParam, Param: 0}, {Kind: quality.ArgField}}
	if len(call.Args) != len(want) {
		t.Fatalf("args: %+v", call.Args)
	}
	for i, w := range want {
		if call.Args[i] != w {
			t.Fatalf("arg %d: %+v, want %+v", i, call.Args[i], w)
		}
	}
	// The receiver is excluded from the parameter list, matching Params.
	if len(method.ParamList) != 1 || method.ParamList[0].Name != "p" {
		t.Fatalf("method params: %+v", method.ParamList)
	}
}

func TestParamListNamesTypesAndUses(t *testing.T) {
	facts := argsFacts(t)
	caller := findFunction(t, facts, "caller")
	want := []quality.ParamFacts{
		{Name: "p1", Type: "int", Uses: 4},
		{Name: "p2", Type: "string", Uses: 6},
		{Name: "cfg", Type: "args.Config", Uses: 1},
	}
	if len(caller.ParamList) != len(want) {
		t.Fatalf("param list: %+v", caller.ParamList)
	}
	for i, w := range want {
		if caller.ParamList[i] != w {
			t.Fatalf("param %d: %+v, want %+v", i, caller.ParamList[i], w)
		}
	}
	if caller.Params != len(want) {
		t.Fatalf("param count %d vs list %d", caller.Params, len(want))
	}
}
