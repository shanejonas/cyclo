package gopatterns

import (
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/pdg"
)

func TestReturnValuesReachEachFormalOutput(t *testing.T) {
	cases := []struct {
		name, source     string
		returns, outputs int
	}{
		{"identity", "func f(x int) int { return x }", 1, 1},
		{"branch", "func f(x int,c bool) int { if c { return x }; return 0 }", 2, 1},
		{"reassignment", "func f(x int) int { x=1; x=2; return x }", 1, 1},
		{"loop", "func f(x int) int { for x>0 { x--; if x==2 { return x } }; return x }", 2, 1},
		{"multiple results", "func f(x int) (int,bool) { return x,true }", 1, 2},
		{"tuple call", "func f() (int,bool) { return pair() }; func pair() (int,bool) { return 1,true }", 1, 2},
		{"named bare return", "func f(x int) (a,b int) { a=x; b=x+1; return }", 1, 2},
		{"deferred write", "func f() (x int) { defer func(){x++}(); return 1 }", 1, 1},
		{"closure", "func f(x int) int { g:=func() int { return x }; return g() }", 1, 1},
		{"void", "func f() { return }", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "package example; " + tc.source
			g := parseIR(t, source, pdg.NewBuilder())
			if err := pdg.Validate(&g); err != nil {
				t.Fatal(err)
			}
			found := map[pdg.Ref]map[pdg.Position]bool{}
			count := 0
			for _, edge := range g.Edges {
				if g.Tables.Text(edge.Subkind) != "return-value" {
					continue
				}
				count++
				from, to := g.Nodes[edge.Source-1], g.Nodes[edge.Target-1]
				if edge.Kind != pdg.Data || from.Category != pdg.Return || to.Category != pdg.FormalOutput {
					t.Fatalf("wrong return link: %+v", edge)
				}
				if edge.Position != g.NodeAttributes(to).Position {
					t.Fatal("wrong result slot")
				}
				if found[edge.Source] == nil {
					found[edge.Source] = map[pdg.Position]bool{}
				}
				if found[edge.Source][edge.Position] {
					t.Fatal("duplicate result transfer")
				}
				found[edge.Source][edge.Position] = true
				policy := g.Tables.Policies[edge.Policy-1]
				evidence := g.Tables.Evidence[edge.Evidence-1]
				if g.Tables.Text(policy.ID) != "cyclo.go-return-values" || evidence.Policy != edge.Policy || evidence.Status != pdg.Approximated || edge.Definition != 0 {
					t.Fatal("return edge overstates dependency precision")
				}
			}
			if count != tc.returns*tc.outputs {
				t.Fatalf("return links=%d want %d", count, tc.returns*tc.outputs)
			}
			for _, positions := range found {
				if len(positions) != tc.outputs {
					t.Fatal("return misses result slot")
				}
			}
			if tc.outputs > 0 && len(found) != tc.returns {
				t.Fatal("return site missing")
			}
		})
	}
}

func TestBareReturnReadsNamedResultsWithoutDefUseClaims(t *testing.T) {
	g := parseIR(t, "package example; func f(x int) (a,b int) { a=x; b=x+1; return }", pdg.NewBuilder())
	for _, node := range g.Nodes {
		if node.Category != pdg.Return {
			continue
		}
		reads := g.NodeAttributes(node).Reads
		if len(reads) != 2 {
			t.Fatalf("named result reads=%v", reads)
		}
		for i, read := range reads {
			if read.Symbol != g.Function.Outputs[i].Symbol || read.Position != pdg.Position(i+1) {
				t.Fatalf("result %d read=%+v", i, read)
			}
		}
		return
	}
	t.Fatal("no return")
}

func TestReturnCapabilityStatesRemainingLimits(t *testing.T) {
	g := parseIR(t, "package example; func f(x int) int { return x }", pdg.NewBuilder())
	for _, c := range g.Capabilities {
		if g.Tables.Text(c.Name) != "returns" {
			continue
		}
		e := g.Tables.Evidence[c.Evidence-1]
		if e.Status != pdg.Approximated || !strings.Contains(g.Tables.Text(e.Description), "deferred") {
			t.Fatal("return capability loses precision limits")
		}
		return
	}
	t.Fatal("return capability missing")
}
