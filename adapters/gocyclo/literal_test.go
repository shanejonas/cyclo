package gocyclo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shanejonas/cyclo/domain"
)

func TestFunctionVariableNames(t *testing.T) {
	for _, test := range []struct {
		source string
		names  []string
	}{
		{"package p\nvar first, second = func() {}, func() {}\n", []string{"first", "second"}},
		{"package p\nvar number, second = 1, func() {}\n", []string{"second"}},
		{"package p\nvar (first = func() {}; second = func() {})\n", []string{"first", "second"}},
	} {
		file := analyzeLiteralSource(t, test.source)
		if len(file.Functions) != len(test.names) {
			t.Fatalf("functions = %+v", file.Functions)
		}
		for index, name := range test.names {
			if file.Functions[index].Name != name {
				t.Errorf("%s: function %d name = %q, want %q", test.source, index, file.Functions[index].Name, name)
			}
		}
	}
}

func TestNestedLiteralCognitiveNesting(t *testing.T) {
	file := analyzeLiteralSource(t, "package p\nvar outer = func() {\n if true {\n  inner := func() {\n   if true {}\n  }\n  inner()\n }\n}\n")
	if len(file.Functions) != 1 {
		t.Fatalf("functions = %+v", file.Functions)
	}
	function := file.Functions[0]
	if function.CognitiveComplexity != 4 {
		t.Fatalf("score = %d, want 4", function.CognitiveComplexity)
	}
	diagnostics := function.CognitiveDiagnostics
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	if diagnostics[0].Line != 3 || diagnostics[0].Nesting != 0 || diagnostics[1].Line != 5 || diagnostics[1].Nesting != 2 || diagnostics[1].Increment != 3 {
		t.Fatalf("incorrect nesting or position: %+v", diagnostics)
	}
}

func analyzeLiteralSource(t *testing.T, source string) domain.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repro.go")
	err := os.WriteFile(path, []byte(source), 0600)
	if err != nil {
		t.Fatal(err)
	}
	file, err := analyzeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
