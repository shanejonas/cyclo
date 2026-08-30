package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSkillFlagPrintsAnAgentSkillWithoutStartingTheTUI(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"--skill"}, &output)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(output.String(), "---\nname: cyclo\n") {
		t.Fatalf("skill output has no cyclo frontmatter:\n%s", output.String())
	}
	if strings.Contains(output.String(), "\x1b") {
		t.Fatalf("skill output contains TUI escape sequences: %q", output.String())
	}
}

func TestGoRunLaunchesCyclo(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_STATE_HOME", directory)
	source := filepath.Join(directory, "sample.go")
	err := os.WriteFile(source, []byte("package sample\n\nfunc Simple() {}\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "run", ".", "--control-port", "0", source)
	command.Stdin = strings.NewReader("q")
	command.Env = append(os.Environ(), "TERM=xterm-256color")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("go run . did not exit after q: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("go run . failed: %v\n%s", err, output)
	}
}

func TestParseControlPortAndPaths(t *testing.T) {
	options, err := parseRunOptions([]string{"--control-port", "9000", "one.go", "two.go"})
	if err != nil {
		t.Fatal(err)
	}
	if options.controlPort != 9000 || strings.Join(options.paths, ",") != "one.go,two.go" {
		t.Fatalf("options = %+v", options)
	}
}

func TestDefaultControlPort(t *testing.T) {
	options, err := parseRunOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if options.controlPort != 8197 {
		t.Fatalf("control port = %d, want 8197", options.controlPort)
	}
}

func TestRejectInvalidControlPort(t *testing.T) {
	_, err := parseRunOptions([]string{"--control-port", "70000"})
	if err == nil {
		t.Fatal("invalid control port succeeded")
	}
}
