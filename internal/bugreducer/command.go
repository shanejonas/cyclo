// Package bugreducer runs external bug checks against temporary candidate files.
package bugreducer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type checker struct {
	command []string
	path    string
	timeout time.Duration
	checks  int
}

func (c *checker) check(ctx context.Context, candidate []byte) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	err := writeCandidate(c.path, candidate)
	if err != nil {
		return false, fmt.Errorf("write candidate: %w", err)
	}
	c.checks++
	checkContext, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	args := append(append([]string(nil), c.command[1:]...), c.path)
	command := exec.CommandContext(checkContext, c.command[0], args...)
	command.WaitDelay = time.Second
	configureCancellation(command)
	err = command.Run()
	cleanupErr := cleanupChecker(command)
	if cleanupErr != nil {
		return false, fmt.Errorf("clean up checker: %w", errors.Join(err, cleanupErr))
	}
	return checkResult(checkContext, err)
}

func checkResult(ctx context.Context, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, fmt.Errorf("checker interrupted: %w", ctx.Err())
	}
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return false, nil
	}
	return false, fmt.Errorf("run checker: %w", err)
}

func newChecker(options options) (*checker, func(), error) {
	executable, err := exec.LookPath(options.command[0])
	if err != nil {
		return nil, nil, fmt.Errorf("find checker: %w", err)
	}
	temporaryRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return nil, nil, fmt.Errorf("resolve temporary directory: %w", err)
	}
	workspace, err := os.MkdirTemp(temporaryRoot, "cyclo-bug-reducer-")
	if err != nil {
		return nil, nil, fmt.Errorf("create workspace: %w", err)
	}
	command := append([]string{executable}, options.command[1:]...)
	return &checker{command: command, path: filepath.Join(workspace, filepath.Base(options.input)), timeout: options.timeout}, func() { os.RemoveAll(workspace) }, nil
}
