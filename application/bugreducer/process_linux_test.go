//go:build linux

package bugreducer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInterruptionStopsCheckerChild(t *testing.T) {
	source, err := os.ReadFile("testdata/timeout-child.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"timeout", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			leaked, err := checkerLeavesChild(t, source, mode)
			if err != nil {
				t.Fatal(err)
			}
			if leaked {
				t.Fatal("checker stopped but its child process is still running")
			}
		})
	}
}

func checkerLeavesChild(t *testing.T, source []byte, mode string) (bool, error) {
	return checkerLeavesChildWithOutput(t, source, mode, nil)
}

func checkerLeavesChildWithOutput(t *testing.T, source []byte, mode string, output *checkerOutput) (bool, error) {
	t.Helper()
	directory := t.TempDir()
	pidPath := filepath.Join(directory, "child.pid")
	t.Setenv("BUG_REDUCER_CHILD_PID", pidPath)
	c := checker{command: []string{"sh"}, path: filepath.Join(directory, "check.sh"), timeout: 100 * time.Millisecond, output: output}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	expectedError := context.DeadlineExceeded
	if mode == "cancellation" {
		c.timeout = time.Minute
		expectedError = context.Canceled
		timer := time.AfterFunc(100*time.Millisecond, cancel)
		defer timer.Stop()
	}
	if mode == "success" || mode == "rejection" {
		c.timeout = time.Second
		expectedError = nil
	}
	accepted, err := c.check(ctx, source)
	data, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		return false, fmt.Errorf("observe child: %w", readErr)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil || pid <= 0 {
		return false, fmt.Errorf("invalid child PID %q", data)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if accepted != (mode == "success") {
		return false, fmt.Errorf("unexpected acceptance %v in %s", accepted, mode)
	}
	if !errors.Is(err, expectedError) {
		return false, fmt.Errorf("checker error = %v, want %v", err, expectedError)
	}
	// A killed process may briefly remain a zombie until its new parent reaps it.
	// Once reaped, /proc disappears: ENOENT surfaces as os.IsNotExist while a
	// mid-exit read can surface ESRCH ("no such process"). Both mean gone.
	for range 50 {
		status, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if strings.Fields(string(status))[2] == "Z" {
			return false, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true, nil
}

func TestDashboardOutputPreservesChildCleanup(t *testing.T) {
	for _, mode := range []string{"success", "rejection", "timeout", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			fixture := "normal-exit-child.sh"
			if mode == "timeout" || mode == "cancellation" {
				fixture = "timeout-child.sh"
			}
			source, err := os.ReadFile(filepath.Join("testdata", fixture))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "rejection" {
				source = append(source, []byte("exit 1\n")...)
			}
			leaked, err := checkerLeavesChildWithOutput(t, source, mode, &checkerOutput{})
			if err != nil || leaked {
				t.Fatalf("child leaked = %v, checker error = %v", leaked, err)
			}
		})
	}
}

func TestNormalCheckerExitStopsChild(t *testing.T) {
	source, err := os.ReadFile("testdata/normal-exit-child.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "rejection"} {
		t.Run(mode, func(t *testing.T) {
			script := append([]byte(nil), source...)
			if mode == "rejection" {
				script = append(script, []byte("exit 1\n")...)
			}
			leaked, err := checkerLeavesChild(t, script, mode)
			if err != nil {
				t.Fatal(err)
			}
			if leaked {
				t.Fatal("checker exited but left a child running")
			}
		})
	}
}
