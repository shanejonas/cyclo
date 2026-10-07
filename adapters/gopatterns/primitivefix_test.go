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

import "strings"

// ParseCalendar turns a feed into event drafts.
// Second line of docs.
func findProp(name string) string {
	_ = strings.TrimSpace("x")
	return name
}
func findOther(name string) string {
	return name
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
