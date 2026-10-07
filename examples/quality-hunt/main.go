// Command quality-hunt generates ownership examples, checks runtime behavior
// and equivalent-syntax invariants, and reduces mismatches with Tree-sitter.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/shanejonas/cyclo/adapters/treesitter"
	"github.com/shanejonas/cyclo/adapters/reducer"
)

type options struct {
	binary, library, output, recheck string
	timeout                          time.Duration
}

func main() {
	settings := options{}
	flag.StringVar(&settings.binary, "cyclo", "", "optional frozen Cyclo binary; default uses this checkout's analyzer")
	flag.StringVar(&settings.library, "go-parser", "", "Tree-sitter Go parser library, needed only for reduction")
	flag.StringVar(&settings.output, "output", "", "new failure directory; default is a temporary directory")
	flag.DurationVar(&settings.timeout, "timeout", 30*time.Second, "timeout per runtime and analysis check")
	flag.StringVar(&settings.recheck, "recheck", "", "recheck a saved failure directory without reducing again")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := run(ctx, settings); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, settings options) error {
	settings, err := validSettings(settings)
	if err != nil {
		return err
	}
	cases, source, err := loadInput(settings.recheck)
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "cyclo-quality-hunt-work-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	check := runner{root: root, binary: settings.binary, timeout: settings.timeout}
	return check.checkInput(ctx, settings, cases, source)
}

func (check runner) checkInput(ctx context.Context, settings options, cases []specimen, source []byte) error {
	mismatch, err := check.assess(ctx, source, cases)
	if err != nil {
		return err
	}
	if mismatch.Rule != "" {
		if settings.recheck != "" {
			return fmt.Errorf("confirmed %s in %s", mismatch.Rule, mismatch.Name)
		}
		return check.reduceFailure(ctx, settings, cases, mismatch)
	}
	fmt.Printf("PASS: %d programs; runtime and syntax-equivalence checks agree.\n", len(cases))
	return nil
}

func binaryPath(binary string) (string, error) {
	if binary == "" {
		return "", nil
	}
	return filepath.Abs(binary)
}

func failingPair(cases []specimen, mismatch failure) []specimen {
	var pair []specimen
	for _, c := range cases {
		if c.Name == mismatch.Base || c.Name == mismatch.Name {
			pair = append(pair, c)
		}
	}
	return pair
}

func (check runner) reduceFailure(ctx context.Context, settings options, cases []specimen, mismatch failure) error {
	pair := failingPair(cases, mismatch)
	source := program(cases)
	if err := check.confirmFailure(ctx, source, pair, mismatch); err != nil {
		return err
	}
	directory, err := failureDirectory(settings.output)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "seed.go.txt"), source, 0600); err != nil {
		return err
	}
	attempts := 0
	predicate := func(candidate []byte) (bool, error) {
		attempts++
		got, err := check.assess(ctx, candidate, pair)
		if errors.Is(err, invalidCandidate) {
			return false, nil
		}
		return got == mismatch, err
	}
	parser := treesitter.GoParser{Context: ctx, Library: settings.library}
	reduced, reduceErr := reducer.ReduceSyntaxWithProgress(source, predicate, nil, parser)
	// The seed mismatch is already confirmed even if parsing never starts.
	if reduced == nil {
		reduced = source
	}
	saveErr := saveFailure(directory, failureRecord{Failure: mismatch, Cases: pair, Checks: attempts}, reduced)
	return errors.Join(fmt.Errorf("%s in %s; %d -> %d bytes, %d checks; evidence saved %s", mismatch.Rule, mismatch.Name, len(source), len(reduced), attempts, directory), reduceErr, saveErr)
}

func failureDirectory(path string) (string, error) {
	if path == "" {
		return os.MkdirTemp("", "cyclo-quality-hunt-failure-")
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return "", err
	}
	return path, nil
}

func validSettings(settings options) (options, error) {
	if settings.timeout <= 0 {
		return options{}, fmt.Errorf("timeout must be positive")
	}
	binary, err := binaryPath(settings.binary)
	settings.binary = binary
	return settings, err
}

func (check runner) confirmFailure(ctx context.Context, source []byte, pair []specimen, mismatch failure) error {
	confirmed, err := check.assess(ctx, source, pair)
	if err != nil {
		return err
	}
	if confirmed != mismatch {
		return fmt.Errorf("mismatch does not reproduce for selected pair: %+v", mismatch)
	}
	return nil
}
