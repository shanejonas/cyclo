//go:build unix

package bugreducer

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Shell checkers commonly launch compilers or test runners. Cancel the whole
// process group so those children cannot continue after the checker times out.
func configureCancellation(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return killCheckerGroup(command.Process.Pid) }
}

func killCheckerGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

// The shell may exit normally while background children are still running.
func cleanupChecker(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := killCheckerGroup(command.Process.Pid)
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
