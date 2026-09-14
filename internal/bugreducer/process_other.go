//go:build !unix

package bugreducer

import "os/exec"

// Other platforms retain exec.CommandContext's direct-process cancellation.
func configureCancellation(command *exec.Cmd) {}

func cleanupChecker(command *exec.Cmd) error { return nil }
