package gocyclo

import (
	"path/filepath"
	"testing"
)

func TestGitDiffTreatsSourceFilenameLiterally(t *testing.T) {
	for _, name := range []string{"[ab].go", "*.go", "?.go", ":(glob)*.go"} {
		t.Run(name, func(t *testing.T) {
			root := setupFilenameCase(t, name)
			path := filepath.Join(root, name)
			writeDiffFixture(t, path, "package p\nfunc picked() { println(\"after\") }\n")
			report, err := NewAnalyzer().Analyze([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Files) != 1 || len(report.Files[0].Functions) != 1 {
				t.Fatalf("unexpected report: %+v", report)
			}
			lines := report.Files[0].Functions[0].DiffLines
			if len(lines) != 2 {
				t.Fatalf("diff includes other files: %+v", lines)
			}
			if lines[0].Kind != "deleted" || lines[0].Text != "func picked() { println(\"before\") }" || lines[0].OldLine != 2 {
				t.Fatalf("deleted line = %+v", lines[0])
			}
			if lines[1].Kind != "added" || lines[1].Text != "func picked() { println(\"after\") }" || lines[1].NewLine != 2 {
				t.Fatalf("added line = %+v", lines[1])
			}
		})
	}
}

func setupFilenameCase(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "cyclo@example.com")
	runGit(t, root, "config", "user.name", "Cyclo")
	writeDiffFixture(t, filepath.Join(root, name), "package p\nfunc picked() { println(\"before\") }\n")
	writeDiffFixture(t, filepath.Join(root, "a.go"), "package p\nfunc other() { println(\"before\") }\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeDiffFixture(t, filepath.Join(root, "a.go"), "package p\nfunc other() { println(\"after\") }\n")
	return root
}
