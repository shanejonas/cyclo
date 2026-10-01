package goquality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/domain/quality"
)

func TestReportJoinsTypedEvidenceToComplexityFunctions(t *testing.T) {
	path, err := filepath.Abs("testdata/sample/sample.go")
	if err != nil {
		t.Fatal(err)
	}
	analyzer := ReportAnalyzer{Complexity: gocyclo.NewAnalyzer(), Config: quality.DefaultConfig()}
	report, err := analyzer.Analyze([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if report.Quality == nil || report.Quality.Status != "ready" {
		t.Fatalf("quality: %+v", report.Quality)
	}
	for _, function := range report.Files[0].Functions {
		if function.Name != "Writes" {
			continue
		}
		if function.Quality == nil {
			t.Fatal("typed evidence did not join by source position")
		}
		q := function.Quality
		if q.Mutations != 11 || len(q.MutationEvents) != 11 || len(q.Diagnostics) == 0 {
			t.Fatalf("quality: %+v", q)
		}
		return
	}
	t.Fatal("Writes function missing")
}

func TestTypeErrorsKeepComplexityReportAvailable(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{"go.mod": "module broken\n\ngo 1.25.0\n", "broken.go": "package broken\nfunc Broken() { missing() }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	analyzer := ReportAnalyzer{Complexity: gocyclo.NewAnalyzer(), Config: quality.DefaultConfig()}
	report, err := analyzer.Analyze([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if report.Functions != 1 || report.Quality.Status != "error" || report.Quality.Report != nil {
		t.Fatalf("report: %+v", report)
	}
	if !strings.Contains(report.Quality.Error, "missing") || report.Files[0].Functions[0].Quality != nil {
		t.Fatal("failure looks like a clean typed scan")
	}
}
