package patterns

import (
	"reflect"
	"testing"
)

// lshTestPdg builds a small linear PDG: param -> op -> call -> return.
func lshTestPdg() *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			{Kind: Param, Line: 1, TyClass: "int"},
			{Kind: Op, Line: 2, Detail: "add:int"},
			{Kind: Call, Line: 3, CalleeID: "builtin.println", SigClass: "fn(int)"},
			{Kind: Return, Line: 4},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Data},
			{From: 1, To: 2, Kind: Data},
			{From: 2, To: 3, Kind: Ctrl},
		},
	}
}

// lshTestPdgPermuted is isomorphic to lshTestPdg with nodes in reverse
// order: return <- call <- op <- param.
func lshTestPdgPermuted() *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			{Kind: Return, Line: 4},
			{Kind: Call, Line: 3, CalleeID: "builtin.println", SigClass: "fn(int)"},
			{Kind: Op, Line: 2, Detail: "add:int"},
			{Kind: Param, Line: 1, TyClass: "int"},
		},
		Edges: []PdgEdge{
			{From: 3, To: 2, Kind: Data},
			{From: 2, To: 1, Kind: Data},
			{From: 1, To: 0, Kind: Ctrl},
		},
	}
}

func TestVectorizeDim(t *testing.T) {
	vec := Vectorize(NewWl(lshTestPdg()))
	if len(vec) != lshDim {
		t.Fatalf("expected vector length %d, got %d", lshDim, len(vec))
	}
	nonzero := false
	for _, v := range vec {
		if v != 0 {
			nonzero = true
			break
		}
	}
	if !nonzero {
		t.Error("expected non-zero vector for non-empty graph")
	}
}

func TestVectorizeDeterministic(t *testing.T) {
	w := NewWl(lshTestPdg())
	a, b := Vectorize(w), Vectorize(w)
	if !reflect.DeepEqual(a, b) {
		t.Error("Vectorize is not deterministic")
	}
}

func TestVectorizeIsomorphismInvariant(t *testing.T) {
	// The key property for clone discovery: node order must not matter.
	a := Vectorize(NewWl(lshTestPdg()))
	b := Vectorize(NewWl(lshTestPdgPermuted()))
	if !reflect.DeepEqual(a, b) {
		t.Error("isomorphic PDGs gave different vectors")
	}
}

// unitVec returns a dim-length vector with 1.0 at position pos.
func unitVec(dim, pos int) []float64 {
	vec := make([]float64, dim)
	vec[pos] = 1.0
	return vec
}

// negVec returns the negation of vec.
func negVec(vec []float64) []float64 {
	out := make([]float64, len(vec))
	for i, v := range vec {
		out[i] = -v
	}
	return out
}

func TestLSHIdenticalCollide(t *testing.T) {
	lsh := NewLSH(lshDim, 4, 8, 42)
	v := unitVec(lshDim, 0)
	lsh.Add("a", v)
	found := false
	for _, id := range lsh.Query(v) {
		if id == "a" {
			found = true
		}
	}
	if !found {
		t.Error("identical vector did not collide in any table")
	}
}

func TestLSHOppositeSeparate(t *testing.T) {
	// With one table and one hyperplane, v and -v fall on opposite
	// sides (the Gaussian draw is never exactly zero).
	lsh := NewLSH(lshDim, 1, 1, 7)
	pos, neg := unitVec(lshDim, 0), negVec(unitVec(lshDim, 0))
	lsh.Add("neg", neg)
	for _, id := range lsh.Query(pos) {
		if id == "neg" {
			t.Error("opposite vectors collided")
		}
	}
}

func TestClusterClones(t *testing.T) {
	lsh := NewLSH(lshDim, 1, 1, 7)
	pos := unitVec(lshDim, 0)
	vectors := map[string][]float64{
		"a": pos,
		"b": pos,
		"c": negVec(pos),
	}
	groups := ClusterClones(vectors, lsh)
	if len(groups) != 1 {
		t.Fatalf("expected 1 clone group, got %d: %v", len(groups), groups)
	}
	if !reflect.DeepEqual(groups[0], []string{"a", "b"}) {
		t.Errorf("expected group [a b], got %v", groups[0])
	}
}

func TestClusterClonesDeterministic(t *testing.T) {
	build := func() [][]string {
		lsh := NewLSH(lshDim, 2, 4, 99)
		pos := unitVec(lshDim, 3)
		vectors := map[string][]float64{
			"x": pos,
			"y": pos,
			"z": negVec(pos),
		}
		return ClusterClones(vectors, lsh)
	}
	a, b := build(), build()
	if !reflect.DeepEqual(a, b) {
		t.Errorf("ClusterClones not deterministic: %v vs %v", a, b)
	}
}
