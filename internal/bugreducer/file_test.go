package bugreducer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckerHardLinkCannotOverwriteOriginal(t *testing.T) {
	if originalChanges(t, []byte("x\n")) {
		t.Fatal("a later candidate overwrote the original through the checker's hard link")
	}
}

func originalChanges(t *testing.T, source []byte) bool {
	t.Helper()
	command := checkerCommand(t, "link")
	input := filepath.Join(t.TempDir(), "original.txt")
	t.Setenv("BUG_REDUCER_LINK_TARGET", input)
	err := os.WriteFile(input, source, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), append([]string{input, "--"}, command...), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	return !bytes.Equal(after, source)
}

func TestRelativeTempDirectoryStillPassesAbsoluteCandidate(t *testing.T) {
	command := checkerCommand(t, "empty")
	t.Chdir(t.TempDir())
	err := os.Mkdir("tmp", 0700)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", "tmp")
	c, cleanup, err := newChecker(options{input: "input.txt", command: command, timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !filepath.IsAbs(c.path) {
		t.Fatalf("candidate path is relative: %q", c.path)
	}
}

func TestCandidateReplacementDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original")
	path := filepath.Join(root, "candidate")
	err := os.WriteFile(original, []byte("keep"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(original, path)
	if err != nil {
		t.Fatal(err)
	}
	err = writeCandidate(path, []byte("smaller"))
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := os.ReadFile(original)
	if err != nil || string(unchanged) != "keep" {
		t.Fatalf("original = %q, %v", unchanged, err)
	}
	candidate, err := os.ReadFile(path)
	if err != nil || string(candidate) != "smaller" {
		t.Fatalf("candidate = %q, %v", candidate, err)
	}
}

func TestReadInputRejectsDirectories(t *testing.T) {
	_, err := readInput(t.TempDir())
	if err == nil {
		t.Fatal("accepted a directory")
	}
}

func TestReadInputFollowsRegularFileSymlink(t *testing.T) {
	directory := t.TempDir()
	original := filepath.Join(directory, "original")
	link := filepath.Join(directory, "link")
	err := os.WriteFile(original, []byte("input"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(original, link)
	if err != nil {
		t.Fatal(err)
	}
	source, err := readInput(link)
	if err != nil || string(source) != "input" {
		t.Fatalf("input = %q, %v", source, err)
	}
}
