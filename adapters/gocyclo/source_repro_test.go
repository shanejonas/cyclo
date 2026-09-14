package gocyclo

import "testing"

func TestParenthesizedFunctionLiteral(t *testing.T) {
	for _, expression := range []string{"(func() { if true {} })", "((func() { if true {} }))"} {
		file := analyzeLiteralSource(t, "package p\nvar handle = "+expression+"\n")
		if len(file.Functions) != 1 {
			t.Fatalf("functions = %+v, want handle", file.Functions)
		}
		function := file.Functions[0]
		if function.Name != "handle" || function.Complexity != 2 || function.CognitiveComplexity != 1 {
			t.Fatalf("incorrect function: %+v", function)
		}
	}
}

func TestLineDirectiveUsesPhysicalSourceLocations(t *testing.T) {
	file := analyzeLiteralSource(t, "package p\n//line imaginary.go:100\nfunc f() {\n if true {}\n}\n")
	function := file.Functions[0]
	if function.Line != 3 || function.EndLine != 5 || function.Column != 1 {
		t.Fatalf("source location = %d:%d-%d, want 3:1-5", function.Line, function.Column, function.EndLine)
	}
	if function.CyclomaticDiagnostics[1].Line != 4 || function.CyclomaticDiagnostics[1].Column != 2 {
		t.Fatalf("cyclomatic diagnostics = %+v", function.CyclomaticDiagnostics)
	}
	if function.CognitiveDiagnostics[0].Line != 4 || function.CognitiveDiagnostics[0].Column != 2 {
		t.Fatalf("cognitive diagnostics = %+v", function.CognitiveDiagnostics)
	}
}

func TestMinimizedSourceReproducers(t *testing.T) {
	for _, source := range []string{
		"package p\nvar handle = (func() {})\n",
		"package p\n//line imaginary.go:100\nfunc f() {\n}\n",
	} {
		file := analyzeLiteralSource(t, source)
		if len(file.Functions) != 1 {
			t.Fatalf("%q: functions = %+v", source, file.Functions)
		}
		function := file.Functions[0]
		if function.Source == "" || function.Line > 3 || function.EndLine > 4 {
			t.Fatalf("invalid source location: %+v", function)
		}
	}
}

func TestParenthesizedLiteralNamesAndIgnore(t *testing.T) {
	file := analyzeLiteralSource(t, "package p\nvar number, second = 1, ((func() {}))\n//gocyclo:ignore\nvar ignored = (func() {})\n")
	if len(file.Functions) != 1 || file.Functions[0].Name != "second" {
		t.Fatalf("functions = %+v, want only second", file.Functions)
	}
}
