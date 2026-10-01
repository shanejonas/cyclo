package bugreducer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a checker, avoiding shell dependencies and testing
// argument boundaries with paths and marker values containing spaces.
func TestCheckerProcess(t *testing.T) {
	mode := os.Getenv("BUG_REDUCER_TEST_MODE")
	if mode == "" {
		return
	}
	if message := os.Getenv("BUG_REDUCER_TEST_OUTPUT"); message != "" {
		fmt.Fprintln(os.Stdout, message)
		fmt.Fprintln(os.Stderr, "stderr marker")
	}
	if mode == "timeout" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	path := os.Args[len(os.Args)-1]
	candidate, err := os.ReadFile(path)
	if err != nil {
		os.Exit(2)
	}
	if mode == "link" {
		err := os.Remove(path)
		if err != nil {
			os.Exit(2)
		}
		err = os.Link(os.Getenv("BUG_REDUCER_LINK_TARGET"), path)
		if err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if mode == "empty" {
		os.Exit(0)
	}
	if mode == "json" {
		var value struct {
			Trigger bool `json:"trigger"`
		}
		err := json.Unmarshal(candidate, &value)
		if err == nil && value.Trigger {
			os.Exit(0)
		}
		os.Exit(1)
	}
	marker := os.Args[len(os.Args)-2]
	if bytes.Contains(candidate, []byte(marker)) {
		os.Exit(0)
	}
	os.Exit(1)
}

func checkerCommand(t *testing.T, mode string) []string {
	t.Helper()
	t.Setenv("BUG_REDUCER_TEST_MODE", mode)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return []string{executable, "-test.run=^TestCheckerProcess$", "--", "specific failure"}
}

func TestExternalCheckerReducesText(t *testing.T) {
	command := checkerCommand(t, "marker")
	input := filepath.Join(t.TempDir(), "input with spaces.txt")
	original := []byte("noise\nspecific failure\nmore noise\n")
	err := os.WriteFile(input, original, 0600)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	args := append([]string{input, "--"}, command...)
	err = Run(context.Background(), args, &output)
	if err != nil {
		t.Fatal(err)
	}
	reduced, err := os.ReadFile(input + ".reduced")
	if err != nil {
		t.Fatal(err)
	}
	if string(reduced) != "specific failure\n" {
		t.Fatalf("reduced = %q", reduced)
	}
	unchanged, err := os.ReadFile(input)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatalf("original changed: %q, %v", unchanged, err)
	}
	if !strings.Contains(output.String(), "saved ") {
		t.Fatalf("output = %q", output.String())
	}
	err = Run(context.Background(), args, &output)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrote output: %v", err)
	}
}

func TestRejectedSeedLeavesNoOutput(t *testing.T) {
	command := checkerCommand(t, "marker")
	input := filepath.Join(t.TempDir(), "input")
	err := os.WriteFile(input, []byte("no bug"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), append([]string{input, "--"}, command...), &bytes.Buffer{})
	if err == nil {
		t.Fatal("accepted seed without the bug")
	}
	_, err = os.Stat(input + ".reduced")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected output: %v", err)
	}
}

func TestCheckerTimeoutAndMissingExecutable(t *testing.T) {
	command := checkerCommand(t, "timeout")
	check := checker{command: command, path: filepath.Join(t.TempDir(), "candidate"), timeout: 50 * time.Millisecond}
	_, err := check.check(context.Background(), []byte("anything"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
	check.command = []string{filepath.Join(t.TempDir(), "missing")}
	_, err = check.check(context.Background(), []byte("anything"))
	var launch *os.PathError
	var lookup *exec.Error
	if !errors.As(err, &launch) && !errors.As(err, &lookup) {
		t.Fatalf("launch failure = %v", err)
	}
}

func TestFinishSavesBestCandidateOnInterruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "best")
	destination, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = finish(destination, []byte("best"), context.Canceled, 4, 100, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != "best" {
		t.Fatalf("saved = %q, %v", saved, err)
	}
}

func TestOptionsRejectMissingCommandAndInvalidTimeout(t *testing.T) {
	for _, args := range [][]string{{}, {"input"}, {"input", "--"}, {"--timeout", "0", "input", "--", "checker"}} {
		if _, err := parseOptions(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestExternalCheckerOwnsJSONValidity(t *testing.T) {
	command := checkerCommand(t, "json")
	input := filepath.Join(t.TempDir(), "input.json")
	source := []byte("{\n  \"noise\": 42,\n  \"trigger\": true\n}\n")
	err := os.WriteFile(input, source, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), append([]string{input, "--"}, command...), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	reduced, err := os.ReadFile(input + ".reduced")
	if err != nil {
		t.Fatal(err)
	}
	if string(reduced) != "{\n  \"trigger\": true\n}\n" {
		t.Fatalf("result = %q", reduced)
	}
}

func TestGoReductionOptions(t *testing.T) {
	settings, err := parseOptions([]string{"--language", "go", "--go-parser", "/parser/go.so", "input.go", "--", "checker"}, io.Discard)
	if err != nil || settings.language != "go" || settings.goParser != "/parser/go.so" {
		t.Fatalf("options: %+v, %v", settings, err)
	}
	for _, flags := range [][]string{{"--language", "rust"}, {"--go-parser", "/parser/go.so"}} {
		args := append(flags, "input.go", "--", "checker")
		if _, err := parseOptions(args, io.Discard); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestMissingGoParserRemovesUnacceptedOutput(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "input.go")
	output := filepath.Join(directory, "reduced.go")
	if err := os.WriteFile(input, []byte("package p\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--language", "go", "--go-parser", filepath.Join(directory, "missing.so"), "--tui=false", "--output", output, input, "--"}
	args = append(args, checkerCommand(t, "marker")...)
	err := Run(context.Background(), args, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "Tree-sitter") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unaccepted output remains: %v", err)
	}
}
