package gopatterns

import (
	"github.com/shanejonas/cyclo/domain/pdg"
	"strings"
	"testing"
)

func TestExecutionMustPrecedeBranchesLoopsAndTransfers(t *testing.T) {
	cases := []struct {
		name, body, from, to string
		want                 bool
	}{
		{"sequence", "a(); b()", "a()", "b()", true},
		{"branch join", "if c { a() }; b()", "a()", "b()", false},
		{"two arms", "if c { a() } else { b() }; z()", "a()", "z()", false},
		{"loop exit", "for c { a() }; b()", "a()", "b()", false},
		{"continue", "for c { a(); continue; b() }", "a()", "b()", false},
		{"goto skip", "goto L; a(); L: b()", "a()", "b()", false},
		{"return skip", "a(); return; b()", "a()", "b()", false},
		{"panic skip", "panic(1); b()", "panic(1)", "b()", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "package example\nfunc f(c bool) { " + tc.body + " }; func a() {}; func b() {}; func z() {}"
			g := parseIR(t, source, pdg.NewBuilder())
			if err := pdg.Validate(&g); err != nil {
				t.Fatal(err)
			}
			got := false
			for _, edge := range g.Edges {
				if edge.Kind != pdg.Execution {
					continue
				}
				from := g.Spans[g.Nodes[edge.Source-1].Span-1]
				to := g.Spans[g.Nodes[edge.Target-1].Span-1]
				if source[from.Start:from.End] == tc.from && source[to.Start:to.End] == tc.to {
					got = true
				}
				if g.Tables.Text(edge.Subkind) != "must-precede" {
					t.Fatal("missing execution policy")
				}
			}
			if got != tc.want {
				t.Fatalf("edge %s -> %s=%t want %t", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestReferenceBindingsAreDistinctTypedVariables(t *testing.T) {
	source := `package example
 type Alias = *int
 type Pointer *int
 func f(p Alias, q Pointer, xs []int, m map[int]int, ch chan int, action func(), i interface{}, x int, a [2]int) {
  p = p; p = p; var local *int; local = p; _ = local
  xs = xs; m = m; ch = ch; action = action; i = i; x = x; a = a
 }`
	g := parseIR(t, source, pdg.NewBuilder())
	count := g.Function.ReferenceCount
	if count.Status != pdg.Known || count.Value != 8 || count.Policy == 0 {
		t.Fatalf("count=%+v", count)
	}
	if err := pdg.Validate(&g); err != nil {
		t.Fatal(err)
	}
	generic := parseIR(t, "package example; func f[T any](x T) { _ = x }", pdg.NewBuilder())
	if generic.Function.ReferenceCount.Status != pdg.Unknown || generic.Function.ReferenceCount.Reason == 0 {
		t.Fatal("generic reference kind must stay unknown")
	}
	if !strings.Contains(generic.Tables.Text(generic.Function.ReferenceCount.Reason), "type parameter") {
		t.Fatal("missing reason")
	}
}
