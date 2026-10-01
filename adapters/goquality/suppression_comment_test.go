package goquality

import (
	"context"
	"os"
	"testing"

	"github.com/shanejonas/cyclo/domain/quality"
)

func TestReducedSuppressionStringIsNotAComment(t *testing.T) {
	source, err := os.ReadFile("testdata/suppression-strings/reduced.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	root := qualitySourceModule(t, source)
	facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	f := findFunction(t, facts, "Plain")
	if f.PrecedingLine != "" {
		t.Fatalf("Go string data becomes suppression metadata: %q", f.PrecedingLine)
	}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Diagnostics) != 0 {
		t.Fatalf("string data causes findings: %+v", report.Diagnostics)
	}
}

func TestSuppressionReadsParsedDeclarationComments(t *testing.T) {
	for _, test := range []struct {
		name, prefix, want string
	}{
		{"line comment and docs", "// cyclo-allow(fn_params): stable API\n// Plain has five parameters.\n", ""},
		{"missing reason", "// cyclo-allow(fn_params):\n", "invalid_suppression"},
		{"block directive", "/* cyclo-allow(fn_params): stable API */\n", "invalid_suppression"},
		{"raw string", "const example = `example\n// cyclo-allow(fn_params): sample`\n", "fn_params"},
		{"string and docs", "const example = \"cyclo-allow\"\n// Plain has five parameters.\n", "fn_params"},
		{"blank separates comments", "// cyclo-allow(fn_params): unrelated\n\n", "fn_params"},
		{"trailing comment belongs to constant", "const example = 1 // cyclo-allow(fn_params): unrelated\n", "fn_params"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := "package repro\n" + test.prefix + "func Plain(a, b, c, d, e int) int { return 1 }\n"
			root := qualitySourceModule(t, []byte(source))
			facts, err := (Analyzer{Root: root}).Extract(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			report, err := quality.Evaluate(facts, quality.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			f := findFunction(t, facts, "Plain")
			if test.want == "" {
				if len(report.Diagnostics) != 0 {
					t.Fatalf("valid suppression fails: %+v", report.Diagnostics)
				}
				return
			}
			for _, diagnostic := range report.Diagnostics {
				if diagnostic.RuleID == test.want {
					return
				}
			}
			t.Fatalf("findings = %+v, want %s; comment = %q", report.Diagnostics, test.want, f.PrecedingLine)
		})
	}
}
