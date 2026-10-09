package patterns

import (
	"fmt"
	"sort"
)

// Dependence Clusters (Binkley & Harman):
// A dependence cluster is a set of statements that are mutually dependent
// (strongly connected in the dependence graph). Empirically, large programs
// contain clusters spanning 10%+ of the codebase, and big clusters map to
// "everything affects everything" spaghetti that resists testing and
// comprehension.
//
// For cyclo: a dependence_cluster pattern flags functions whose PDG contains
// a large strongly-connected region — the code is a tangle. Linchpin
// functions (whose removal would collapse the cluster) are the
// highest-leverage refactoring targets.

// minClusterSize is the minimum SCC size to flag as a cluster.
// Smaller SCCs are normal (e.g., a loop); large ones are spaghetti.
const minClusterSize = 10

// clusterSizeFraction flags a cluster if it spans this fraction of the
// function's PDG nodes, even if below minClusterSize.
const clusterSizeFraction = 0.25

// DepCluster is a strongly-connected region of a PDG.
type DepCluster struct {
	// FuncID identifies the source function.
	FuncID string
	// Nodes are the PDG node indices in the cluster.
	Nodes []int
	// Lines are the source lines covered.
	Lines []int
	// Fraction is the cluster size as a fraction of the PDG.
	Fraction float64
}

// FindDepClusters finds dependence clusters (large SCCs) in function PDGs.
func FindDepClusters(facts []*FuncFacts) []DepCluster {
	var out []DepCluster
	for _, f := range facts {
		if f.Pdg == nil || len(f.Pdg.Nodes) == 0 {
			continue
		}
		sccs := stronglyConnected(f.Pdg)
		for _, scc := range sccs {
			if !isCluster(scc, len(f.Pdg.Nodes)) {
				continue
			}
			out = append(out, DepCluster{
				FuncID:   f.ID,
				Nodes:    scc,
				Lines:    clusterLines(f.Pdg, scc),
				Fraction: float64(len(scc)) / float64(len(f.Pdg.Nodes)),
			})
		}
	}
	return out
}

// isCluster reports whether an SCC is large enough to be a dependence cluster.
func isCluster(scc []int, totalNodes int) bool {
	if len(scc) < 2 {
		return false // Single node or empty; not a cluster.
	}
	if len(scc) >= minClusterSize {
		return true
	}
	return float64(len(scc))/float64(totalNodes) >= clusterSizeFraction
}

// clusterLines returns the sorted source lines for a cluster.
func clusterLines(pdg *Pdg, nodes []int) []int {
	lines := make([]int, len(nodes))
	for i, n := range nodes {
		lines[i] = pdg.Nodes[n].Line
	}
	sort.Ints(lines)
	return lines
}

// stronglyConnected finds all strongly connected components using Tarjan's
// algorithm. Returns components with 2+ nodes (singletons aren't clusters).
func stronglyConnected(pdg *Pdg) [][]int {
	t := newTarjan(pdg)
	t.run()
	return filterSCCs(t.sccs)
}

// newTarjan creates a Tarjan state for the PDG.
func newTarjan(pdg *Pdg) *tarjan {
	n := len(pdg.Nodes)
	adj := make([][]int, n)
	for _, e := range pdg.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	t := &tarjan{
		adj:     adj,
		index:   make([]int, n),
		lowlink: make([]int, n),
		onStack: make([]bool, n),
	}
	for i := range t.index {
		t.index[i] = -1
	}
	return t
}

// run executes Tarjan's algorithm.
func (t *tarjan) run() {
	for v := 0; v < len(t.index); v++ {
		if t.index[v] == -1 {
			t.visit(v)
		}
	}
}

// filterSCCs keeps components with 2+ nodes.
func filterSCCs(sccs [][]int) [][]int {
	var out [][]int
	for _, scc := range sccs {
		if len(scc) >= 2 {
			out = append(out, scc)
		}
	}
	return out
}

type tarjan struct {
	adj     [][]int
	index   []int
	lowlink []int
	onStack []bool
	stack   []int
	counter int
	sccs    [][]int
}

func (t *tarjan) visit(v int) {
	t.index[v] = t.counter
	t.lowlink[v] = t.counter
	t.counter++
	t.stack = append(t.stack, v)
	t.onStack[v] = true
	for _, w := range t.adj[v] {
		t.visitEdge(v, w)
	}
	if t.lowlink[v] == t.index[v] {
		t.popSCC(v)
	}
}

// visitEdge processes one edge v->w in Tarjan's algorithm.
func (t *tarjan) visitEdge(v, w int) {
	if t.index[w] == -1 {
		t.visit(w)
		if t.lowlink[w] < t.lowlink[v] {
			t.lowlink[v] = t.lowlink[w]
		}
	} else if t.onStack[w] {
		if t.index[w] < t.lowlink[v] {
			t.lowlink[v] = t.index[w]
		}
	}
}

// popSCC pops a strongly connected component off the stack.
func (t *tarjan) popSCC(v int) {
	var scc []int
	for {
		w := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		t.onStack[w] = false
		scc = append(scc, w)
		if w == v {
			break
		}
	}
	t.sccs = append(t.sccs, scc)
}

// DepClusterCandidates converts clusters to pattern candidates.
func DepClusterCandidates(clusters []DepCluster, facts []*FuncFacts) []Candidate {
	factByID := map[string]*FuncFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, c := range clusters {
		f := factByID[c.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:             DependenceCluster,
			ScoreMilli:       650, // High: tangles resist testing.
			Observation:      fmt.Sprintf("%d mutually-dependent statements (%.0f%% of function)", len(c.Nodes), c.Fraction*100),
			Inference:        "everything affects everything here; the code resists testing and comprehension",
			PossibleRefactor: "break the cycle: extract the cluster into smaller functions with clear inputs/outputs",
			Sites: []Site{
				{Path: f.Path, Line: c.Lines[0], Name: f.Name},
			},
		})
	}
	return out
}
