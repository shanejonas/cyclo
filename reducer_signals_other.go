//go:build !unix

package main

import "os"

func reducerSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
