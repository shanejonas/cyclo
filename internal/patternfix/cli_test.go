package patternfix

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempGo(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFixDryRun(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int) int {
	if x > 0 {
		println("positive")
		println(x)
	} else {
		return -1
	}
	return x
}
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "if x <= 0 {") {
		t.Errorf("expected diff with inverted guard, got:\n%s", s)
	}
	if !strings.Contains(s, "1 fixable") {
		t.Errorf("expected fix count, got:\n%s", s)
	}
	// Dry-run must not modify the file.
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "} else {") {
		t.Error("dry-run modified the file")
	}
}

func TestFixApply(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int) int {
	if x > 0 {
		println("positive")
		println(x)
	} else {
		return -1
	}
	return x
}
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{"--apply", path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	content, _ := os.ReadFile(path)
	s := string(content)
	if !strings.Contains(s, "if x <= 0 {") {
		t.Errorf("expected file rewritten, got:\n%s", s)
	}
	if strings.Contains(s, "} else {") {
		t.Errorf("else should be gone, got:\n%s", s)
	}
}

func TestFixNoCandidates(t *testing.T) {
	path := writeTempGo(t, `package main

func f(x int) int {
	if x < 0 {
		return -1
	}
	return x
}
`)
	var out bytes.Buffer
	err := Run(context.Background(), []string{path}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "0 fixable") {
		t.Errorf("expected 0 fixes, got:\n%s", out.String())
	}
}

func TestFixBadKind(t *testing.T) {
	var out bytes.Buffer
	err := Run(context.Background(), []string{"--kind", "bogus", "."}, &out)
	if err == nil {
		t.Error("expected error for bad kind")
	}
}
