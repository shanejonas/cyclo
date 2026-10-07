package patterncheck

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const pairTemplate = `package main

import "errors"

func %s(name string, email string) error {
	if name == "" {
		return errors.New("name required")
	}
	if email == "" {
		return errors.New("email required")
	}
	if len(name) > 100 {
		return errors.New("name too long")
	}
	return nil
}

func %s(item string, qty int) error {
	if item == "" {
		return errors.New("item required")
	}
	if qty <= 0 {
		return errors.New("qty must be positive")
	}
	if len(item) > 100 {
		return errors.New("item too long")
	}
	return nil
}
`

const pairTemplateB = `package main

import (
	"errors"
	"time"
)

func %s(url string, retries int) error {
	var err error
	for i := 0; i < retries; i++ {
		if url == "" {
			err = errors.New("url required")
			break
		}
		if len(url) > 2048 {
			err = errors.New("url too long")
			break
		}
		time.Sleep(time.Second)
		err = nil
		break
	}
	return err
}

func %s(key string, retries int) error {
	var err error
	for i := 0; i < retries; i++ {
		if key == "" {
			err = errors.New("key required")
			break
		}
		if len(key) > 256 {
			err = errors.New("key too long")
			break
		}
		time.Sleep(time.Second)
		err = nil
		break
	}
	return err
}
`

// TestChangedEndToEnd builds a temp git repo with two parallel function pairs
// in different files, commits them, then modifies one file. --changed must
// keep the candidate touching the diff and drop the one that doesn't.
func TestChangedEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+dir,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module testchanged\n\ngo 1.25\n")
	// Two parallel pairs: the miner finds one candidate per file.
	write("a.go", pairFile("CreateUserA", "CreateOrderA"))
	write("b.go", pairFileB("FetchWithRetryB", "LoadWithRetryB"))
	run("init")
	run("add", ".")
	run("commit", "-m", "base")

	// Sanity: without --changed both candidates appear.
	previous, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(previous) }()
	var both strings.Builder
	if err := Run(context.Background(), []string{"."}, &both); err != nil {
		t.Fatalf("patterns: %v", err)
	}
	if !strings.Contains(both.String(), "CreateUserA") || !strings.Contains(both.String(), "FetchWithRetryB") {
		t.Fatalf("expected both pairs without --changed, got:\n%s", both.String())
	}

	// Touch only a.go: add a comment line inside CreateUserA's body.
	dirty := strings.Replace(pairFile("CreateUserA", "CreateOrderA"),
		"return errors.New(\"name required\")",
		"return errors.New(\"name required\") // touched", 1)
	write("a.go", dirty)

	var changed strings.Builder
	if err := Run(context.Background(), []string{"--changed", "."}, &changed); err != nil {
		t.Fatalf("patterns --changed: %v", err)
	}
	text := changed.String()
	if !strings.Contains(text, "CreateUserA") {
		t.Fatalf("expected the diff-touched pair to be kept, got:\n%s", text)
	}
	if strings.Contains(text, "FetchWithRetryB") {
		t.Fatalf("expected the untouched pair to be dropped, got:\n%s", text)
	}
}

func pairFile(a, b string) string {
	out := pairTemplate
	out = strings.Replace(out, "%s", a, 1)
	out = strings.Replace(out, "%s", b, 1)
	return out
}

func pairFileB(a, b string) string {
	out := pairTemplateB
	out = strings.Replace(out, "%s", a, 1)
	out = strings.Replace(out, "%s", b, 1)
	return out
}
