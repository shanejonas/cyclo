package gocyclo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzerCapturesFunctionSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	source := "package sample\n\nfunc Branchy(ready bool) string {\n\tif ready {\n\t\treturn \"yes\"\n\t}\n\treturn \"no\"\n}\n"
	err := os.WriteFile(path, []byte(source), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	report, err := NewAnalyzer().Analyze([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Files) != 1 || len(report.Files[0].Functions) != 1 {
		t.Fatalf("report = %+v", report)
	}

	function := report.Files[0].Functions[0]
	if function.EndLine != 8 {
		t.Fatalf("end line = %d, want 8", function.EndLine)
	}
	if !strings.Contains(function.Source, "if ready") || !strings.Contains(function.Source, "return \"no\"") {
		t.Fatalf("source = %q", function.Source)
	}
	if function.CognitiveComplexity != 1 {
		t.Fatalf("cognitive complexity = %d, want 1", function.CognitiveComplexity)
	}
	if len(function.CognitiveDiagnostics) != 1 || function.CognitiveDiagnostics[0].Line != 4 {
		t.Fatalf("cognitive diagnostics = %+v", function.CognitiveDiagnostics)
	}
	if report.CognitiveTotal != 1 || report.CognitiveAverage != 1 {
		t.Fatalf("cognitive report = %d total, %.1f average", report.CognitiveTotal, report.CognitiveAverage)
	}
	if len(function.CyclomaticDiagnostics) != function.Complexity {
		t.Fatalf("cyclomatic diagnostics = %+v, want %d entries", function.CyclomaticDiagnostics, function.Complexity)
	}
	if function.CyclomaticDiagnostics[0].Line != 3 || function.CyclomaticDiagnostics[1].Line != 4 {
		t.Fatalf("cyclomatic diagnostic lines = %+v", function.CyclomaticDiagnostics)
	}
}

func TestAnalyzerCapturesTheWorkingTreeDiffFromMainOrMaster(t *testing.T) {
	for _, branch := range []string{"main", "master"} {
		t.Run(branch, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init", "-b", branch)
			runGit(t, root, "config", "user.email", "cyclo@example.com")
			runGit(t, root, "config", "user.name", "Cyclo")
			path := filepath.Join(root, "sample.go")
			before := "package sample\n\nfunc value() string {\n\treturn \"before\"\n}\n"
			after := "package sample\n\nfunc value() string {\n\treturn \"after\"\n}\n"
			err := os.WriteFile(path, []byte(before), 0o600)
			if err != nil {
				t.Fatal(err)
			}
			runGit(t, root, "add", "sample.go")
			runGit(t, root, "commit", "-m", "base")
			err = os.WriteFile(path, []byte(after), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			report, err := NewAnalyzer().Analyze([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			if report.DiffBase != branch {
				t.Fatalf("diff base = %q, want %s", report.DiffBase, branch)
			}
			lines := report.Files[0].Functions[0].DiffLines
			if len(lines) != 2 {
				t.Fatalf("diff lines = %#v, want one deletion and one addition", lines)
			}
			if lines[0].Kind != "deleted" || lines[0].OldLine != 4 || lines[0].Text != "\treturn \"before\"" {
				t.Fatalf("deleted line = %#v", lines[0])
			}
			if lines[1].Kind != "added" || lines[1].NewLine != 4 || lines[1].Text != "\treturn \"after\"" {
				t.Fatalf("added line = %#v", lines[1])
			}
		})
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestDeletionOnlyHunksStayBetweenTheirCurrentLines(t *testing.T) {
	lines := parseDiff("@@ -4 +3,0 @@\n-removed\n")
	if len(lines) != 1 || lines[0].NewLine != 4 {
		t.Fatalf("deletion anchor = %#v, want current line 4", lines)
	}
}
