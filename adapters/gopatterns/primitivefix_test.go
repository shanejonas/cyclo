package gopatterns

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestFixPrimitiveObsessionBasic(t *testing.T) {
	src := `package p

func SendEmail(email string) {}
func ValidateEmail(email string) bool { return email != "" }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, fixes, err := FixPrimitiveObsession(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	if fixes[0].TypeName != "Email" {
		t.Errorf("TypeName = %q, want Email", fixes[0].TypeName)
	}
	s := string(out)
	if !strings.Contains(s, "type Email string") {
		t.Errorf("output should contain 'type Email string', got:\n%s", s)
	}
	if !strings.Contains(s, "email Email") {
		t.Errorf("output should update param type, got:\n%s", s)
	}
}

func TestFixPrimitiveObsessionSkipsSingleFunc(t *testing.T) {
	src := `package p

func SendEmail(email string) {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, fixes, err := FixPrimitiveObsession(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 0 {
		t.Errorf("expected 0 fixes for single function, got %d", len(fixes))
	}
}

func TestFixPrimitiveObsessionSkipsConcat(t *testing.T) {
	src := `package p

func A(email string) string { return "mailto:" + email }
func B(email string) string { return email }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, fixes, err := FixPrimitiveObsession(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 0 {
		t.Errorf("expected 0 fixes when param is concatenated, got %d", len(fixes))
	}
}

func TestFixPrimitiveObsessionWrapsCallSites(t *testing.T) {
	src := `package p

func SendEmail(email string) {}
func ValidateEmail(email string) bool { return true }

func main() {
	addr := "a@b.com"
	SendEmail(addr)
	ValidateEmail("x@y.com")
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, fixes, err := FixPrimitiveObsession(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	s := string(out)
	// Variable arg should be wrapped; literal should not need wrapping
	// (untyped constant assignable to named type).
	if !strings.Contains(s, "SendEmail(Email(addr))") {
		t.Errorf("variable call site should be wrapped, got:\n%s", s)
	}
}

func TestPrimitiveFixImplementsFix(t *testing.T) {
	var _ Fix = PrimitiveFix{}
	fx := PrimitiveFix{Line: 5, TypeName: "Email", Kind: "primitive_obsession"}
	if fx.FixLine() != 5 {
		t.Error("FixLine wrong")
	}
	if fx.FixKind() != "primitive_obsession" {
		t.Error("FixKind wrong")
	}
}

func TestFixPrimitiveObsessionKeepsTypeDeclTogether(t *testing.T) {
	// Regression: the inserted `type Name string` was split apart when a
	// doc comment followed the import block, emitting `type` ... docs ...
	// `Name string`. The declaration must stay together.
	src := `package p

// ParseCalendar turns a feed into event drafts.
// Second line of docs.
func findProp(name string) bool {
	return name != ""
}
func findOther(name string) bool {
	return name == "x"
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, _, err := FixPrimitiveObsession(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "type Name string") {
		t.Errorf("type declaration split apart, got:\n%s", s)
	}
	// The doc comment must stay with its function, not the type.
	docIdx := strings.Index(s, "// ParseCalendar")
	typeIdx := strings.Index(s, "type Name string")
	funcIdx := strings.Index(s, "func findProp")
	if !(typeIdx < docIdx && docIdx < funcIdx) {
		t.Errorf("doc comment misplaced, got:\n%s", s)
	}
}

func TestFixPrimitiveObsessionSkipsUnsafeBodies(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		// string field == named param would not compile.
		{"typed comparison", "type P struct{ name string }\nfunc f(p P, name string) bool { return p.name == name }\nfunc g(p P, name string) bool { return p.name == name }"},
		// Result type stays string.
		{"return", "func f(name string) string { return name }\nfunc g(name string) string { return name }"},
		// h takes a plain string; the fixer only rewrites group calls.
		{"plain call", "func h(s string) {}\nfunc f(name string) { h(name) }\nfunc g(name string) { h(name) }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\n" + tc.body + "\n"
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			_, fixes, err := FixPrimitiveObsession(fset, f, []byte(src))
			if err != nil {
				t.Fatalf("fix: %v", err)
			}
			if len(fixes) != 0 {
				t.Errorf("expected 0 fixes for unsafe body (%s), got %d", tc.name, len(fixes))
			}
		})
	}
}

func TestFixPrimitiveObsessionAllowsConstComparison(t *testing.T) {
	// Untyped constants are assignable to the named type: safe.
	src := `package p

func f(name string) bool { return name == "x" }
func g(name string) bool { return name != "" }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, fixes, err := FixPrimitiveObsession(fset, f, []byte(src))
	if err != nil {
		t.Fatalf("fix: %v", err)
	}
	if len(fixes) != 1 {
		t.Errorf("expected 1 fix for constant comparisons, got %d", len(fixes))
	}
}
