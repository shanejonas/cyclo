//go:build unix

package main

import (
	"os"
	"syscall"
)

func reducerSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
