package patterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseFunc parses a single function declaration from source.
func parseFunc(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", "package p\n"+src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			return fn
		}
	}
	t.Fatal("no function found")
	return nil
}

func TestAstNodeMultisetNil(t *testing.T) {
	if got := AstNodeMultiset(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %v", got)
	}
}

func TestAstNodeMultisetIgnoresNames(t *testing.T) {
	// Same structure, different identifiers and literals: multisets
	// should match.
	a := parseFunc(t, `func foo() int { x := 1; return x + 2 }`)
	b := parseFunc(t, `func bar() int { y := 99; return y + 42 }`)
	ma, mb := AstNodeMultiset(a), AstNodeMultiset(b)
	if len(ma) == 0 || len(mb) == 0 {
		t.Fatal("multisets should be non-empty")
	}
	if AstJaccard(ma, mb) != 1.0 {
		t.Fatalf("structurally identical functions should score 1.0, got %f", AstJaccard(ma, mb))
	}
}

func TestAstNodeMultisetSeesStructure(t *testing.T) {
	// Different structure: loop vs straight-line.
	a := parseFunc(t, `func foo() { for i := 0; i < 10; i++ { println(i) } }`)
	b := parseFunc(t, `func bar() { println(1); println(2) }`)
	if got := AstJaccard(AstNodeMultiset(a), AstNodeMultiset(b)); got >= 0.8 {
		t.Fatalf("structurally different functions should score low, got %f", got)
	}
}

func TestAstJaccardEmpty(t *testing.T) {
	if got := AstJaccard(nil, nil); got != 1.0 {
		t.Fatalf("two empty multisets should score 1.0, got %f", got)
	}
	if got := AstJaccard(map[string]int{"A": 1}, nil); got != 0.0 {
		t.Fatalf("empty vs non-empty should score 0.0, got %f", got)
	}
}

func TestAstJaccardPartial(t *testing.T) {
	a := map[string]int{"A": 2, "B": 1}
	b := map[string]int{"A": 1, "C": 1}
	// inter = min(2,1) = 1; union = max(2,1)+1+1 = 4 → 0.25
	if got := AstJaccard(a, b); got != 0.25 {
		t.Fatalf("expected 0.25, got %f", got)
	}
}

func TestCCASTBypassEmpty(t *testing.T) {
	if got := ccASTShapes([]string{"a", "b"}, nil); got[0].bypass(got[1]) {
		t.Fatalf("nil types should yield no bypass, got %v", got)
	}
}

func TestCCASTBypassHighSimilarity(t *testing.T) {
	a := parseFunc(t, `func foo() int { x := 1; return x + 2 }`)
	b := parseFunc(t, `func bar() int { y := 99; return y + 42 }`)
	c := parseFunc(t, `func baz() { for i := 0; i < 10; i++ { println(i) } }`)
	types := map[string]map[string]int{
		"f1": AstNodeMultiset(a),
		"f2": AstNodeMultiset(b),
		"f3": AstNodeMultiset(c),
	}
	got := ccASTShapes([]string{"f1", "f2", "f3"}, types)
	if !got[0].bypass(got[1]) {
		t.Fatal("structurally identical pair should be bypassed")
	}
	if got[0].bypass(got[2]) || got[1].bypass(got[2]) {
		t.Fatal("structurally different pairs should not be bypassed")
	}
}

func TestCCGraphClonesWithASTMatchesPlain(t *testing.T) {
	// With nil AST types, the AST path must match the plain path exactly.
	pdgs := map[string]*Pdg{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(20, 100),
		"f3": ccTestPdg(5, 200),
	}
	names := map[string]string{
		"f1": "processData",
		"f2": "processDatum",
		"f3": "renderWidget",
	}
	plain := CCGraphClones(pdgs, names)
	withAST := CCGraphClonesWithAST(pdgs, names, nil)
	if len(plain) != len(withAST) {
		t.Fatalf("nil AST types should match plain: %v vs %v", plain, withAST)
	}
}

func TestCCGraphClonesWithASTFindsSyntacticClone(t *testing.T) {
	// Two functions with identical AST structure but PDGs different
	// enough to fail the 0.9 characteristic-vector bar: the AST
	// bypass should still surface them as a clone group (via WL).
	//
	// We build this with real parsed functions so the AST multisets
	// are genuinely similar while the PDGs differ in node counts.
	a := parseFunc(t, `func alpha() int { x := 1; y := 2; return x + y }`)
	b := parseFunc(t, `func alphb() int { p := 3; q := 4; return p + q }`)
	types := map[string]map[string]int{
		"f1": AstNodeMultiset(a),
		"f2": AstNodeMultiset(b),
	}
	// Sanity: the AST similarity must clear the bypass threshold.
	if got := AstJaccard(types["f1"], types["f2"]); got < astBypassThreshold {
		t.Fatalf("test setup broken: AST similarity %f < %f", got, astBypassThreshold)
	}
	// PDGs with different node counts so Stage 1 likely rejects.
	pdgs := map[string]*Pdg{
		"f1": ccTestPdg(20, 1),
		"f2": ccTestPdg(8, 100),
	}
	names := map[string]string{"f1": "alpha", "f2": "alphb"}
	plain := CCGraphClones(pdgs, names)
	withAST := CCGraphClonesWithAST(pdgs, names, types)
	if len(withAST) < len(plain) {
		t.Fatalf("AST path found fewer groups than plain: %v vs %v", withAST, plain)
	}
}
