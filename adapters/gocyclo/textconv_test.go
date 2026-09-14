package gocyclo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitDiffUsesSourceInsteadOfTextConversion(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "cyclo@example.com")
	runGit(t, root, "config", "user.name", "Cyclo")
	runGit(t, root, "config", "diff.convert.textconv", "sed s/before/converted/g")
	writeDiffFixture(t, filepath.Join(root, ".gitattributes"), "*.go diff=convert\n")
	path := filepath.Join(root, "sample.go")
	writeDiffFixture(t, path, "package p\nfunc f() { println(\"before\") }\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeDiffFixture(t, path, "package p\nfunc f() { println(\"after\") }\n")

	report, err := NewAnalyzer().Analyze([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	lines := report.Files[0].Functions[0].DiffLines
	if len(lines) != 2 {
		t.Fatalf("diff = %+v", lines)
	}
	if lines[0].Text != "func f() { println(\"before\") }" {
		t.Fatalf("deleted source = %q, want original source", lines[0].Text)
	}
	if lines[1].Text != "func f() { println(\"after\") }" {
		t.Fatalf("added source = %q", lines[1].Text)
	}
}

func writeDiffFixture(t *testing.T, path, source string) {
	t.Helper()
	err := os.WriteFile(path, []byte(source), 0600)
	if err != nil {
		t.Fatal(err)
	}
}
