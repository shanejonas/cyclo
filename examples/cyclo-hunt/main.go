// Command cyclo-hunt checks Go function initializers against equivalent
// named declarations, and reduces any mismatch to a standalone Go reproducer.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shanejonas/cyclo/internal/reducer"
)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: go run ./examples/cyclo-hunt INPUT.go")
	}
	source, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	if _, valid := parseTargets(source); !valid {
		return fmt.Errorf("input must be self-contained, type-correct Go with function initializers")
	}
	return checkInput(source)
}

func checkInput(source []byte) error {
	workspace, err := os.MkdirTemp("", "cyclo-hunt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	path := filepath.Join(workspace, "candidate.go")
	failure, err := check(path, source)
	if err != nil {
		return err
	}
	if failure != "" {
		return saveFailure(path, source, failure)
	}
	fmt.Println("No initializer mismatches found in this input.")
	return nil
}

func saveFailure(path string, source []byte, failure string) error {
	reduced, err := reducer.Reduce(source, func(candidate []byte) (bool, error) {
		got, err := check(path, candidate)
		return got == failure, err
	})
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp(".", ".cyclo-hunt-")
	if err != nil {
		return err
	}
	for name, content := range map[string][]byte{"seed.go": source, "reduced.go": reduced, "failure.txt": []byte(failure + "\n")} {
		err := os.WriteFile(filepath.Join(directory, name), content, 0600)
		if err != nil {
			return err
		}
	}
	fmt.Printf("%d -> %d bytes; saved %s\n%s\n", len(source), len(reduced), directory, strings.TrimSpace(string(reduced)))
	return fmt.Errorf("Cyclo invariant failed: %s", failure)
}
