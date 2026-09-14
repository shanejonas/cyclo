//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReducerSubprocess(t *testing.T) {
	if os.Getenv("CYCLO_REDUCER_SUBPROCESS") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg != "--" {
			continue
		}
		err := run(os.Args[index+1:], os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(2)
}

func reducerSubprocess(t *testing.T, ctx context.Context, args ...string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestReducerSubprocess$", "--", "bug-reducer"}, args...)...)
	command.Env = append(os.Environ(), "CYCLO_REDUCER_SUBPROCESS=1")
	return command
}

func TestSIGTERMSavesAcceptedInput(t *testing.T) {
	source := []byte("x\n")
	saved := terminateReduction(t, source)
	if !bytes.Equal(saved, source) {
		t.Fatalf("SIGTERM lost accepted input: saved %q, want %q", saved, source)
	}
}

func terminateReduction(t *testing.T, source []byte) []byte {
	t.Helper()
	root := t.TempDir()
	input := filepath.Join(root, "input")
	script := filepath.Join(root, "checker.sh")
	err := os.WriteFile(input, source, 0600)
	if err != nil {
		t.Fatal(err)
	}
	checker := "if [ ! -e \"$CYCLO_ACCEPTED\" ]; then\n : > \"$CYCLO_ACCEPTED\"\n exit 0\nfi\necho $$ > \"$CYCLO_CHECKER_PID\"\nexec sleep 60\n"
	err = os.WriteFile(script, []byte(checker), 0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := reducerSubprocess(t, ctx, "--timeout", "1m", input, "--", "sh", script)
	pidPath := filepath.Join(root, "checker.pid")
	command.Env = append(command.Env, "CYCLO_ACCEPTED="+filepath.Join(root, "accepted"), "CYCLO_CHECKER_PID="+pidPath)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err = command.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	pid := awaitCheckerPID(t, pidPath)
	defer syscall.Kill(-pid, syscall.SIGKILL)
	err = command.Process.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	if ctx.Err() != nil {
		t.Fatalf("reducer hung: %s", output.String())
	}
	saved, err := os.ReadFile(input + ".reduced")
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func awaitCheckerPID(t *testing.T, path string) int {
	t.Helper()
	for range 200 {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("checker never entered its second invocation")
	return 0
}

func TestReducerRejectsFIFOWithoutHanging(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.pipe")
	err := syscall.Mkfifo(input, 0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	output, err := reducerSubprocess(t, ctx, input, "--", "true").CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("reading a FIFO hangs before the checker timeout applies")
	}
	if err == nil || !strings.Contains(string(output), "regular file") {
		t.Fatalf("expected regular-file error: %s, %v", output, err)
	}
}

func TestReducerRejectsUnboundedDeviceInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := reducerSubprocess(t, ctx, "/dev/zero", "--", "true").CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("device input was not rejected promptly")
	}
	if err == nil || !strings.Contains(string(output), "regular file") {
		t.Fatalf("expected regular-file error: %s, %v", output, err)
	}
}
