package bugreducer

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/shanejonas/cyclo/internal/reducer"
)

const usage = `Usage: cyclo bug-reducer [--output PATH] [--timeout 10s] INPUT -- CHECKER [ARGS...]

Minimize an input while preserving a reproducible bug.
The checker receives an absolute candidate file path as its last argument.
Exit 0 accepts the candidate; any ordinary nonzero exit rejects it.
The checker runs in your current directory, with output suppressed.
Timeouts, signals, and launch errors stop reduction.
Only the checker decides validity; no Go parsing or Cyclo-specific check is applied.
Input must be a regular file.
The original is preserved. Output defaults to INPUT.reduced and must not exist.
Reduction removes whole lines; it does not guarantee a globally smallest input.
`

type options struct {
	input   string
	output  string
	command []string
	timeout time.Duration
}

func parseOptions(args []string, output io.Writer) (options, error) {
	flags := flag.NewFlagSet("bug-reducer", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { fmt.Fprint(output, usage) }
	result := options{}
	flags.StringVar(&result.output, "output", "", "new output file")
	flags.DurationVar(&result.timeout, "timeout", 10*time.Second, "timeout per checker invocation")
	err := flags.Parse(args)
	if err != nil {
		return options{}, err
	}
	rest := flags.Args()
	if len(rest) < 3 || rest[1] != "--" {
		return options{}, errors.New(usage)
	}
	if result.timeout <= 0 {
		return options{}, errors.New("timeout must be positive")
	}
	result.input, result.command = rest[0], rest[2:]
	if result.output == "" {
		result.output = result.input + ".reduced"
	}
	return result, nil
}

func Run(ctx context.Context, args []string, output io.Writer) error {
	options, err := parseOptions(args, output)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	return runReduction(ctx, options, output)
}

func runReduction(ctx context.Context, options options, output io.Writer) error {
	source, err := readInput(options.input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	check, cleanup, err := newChecker(options)
	if err != nil {
		return err
	}
	defer cleanup()
	destination, err := os.OpenFile(options.output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	reduced, reduceErr := reducer.Reduce(source, func(candidate []byte) (bool, error) { return check.check(ctx, candidate) })
	return finish(destination, reduced, reduceErr, check.checks, len(source), output)
}

func finish(destination *os.File, reduced []byte, reduceErr error, checks, original int, output io.Writer) error {
	if reduced == nil {
		return errors.Join(reduceErr, destination.Close(), os.Remove(destination.Name()))
	}
	_, writeErr := destination.Write(reduced)
	closeErr := destination.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(reduceErr, writeErr, closeErr)
	}
	_, reportErr := fmt.Fprintf(output, "%d -> %d bytes; %d checks; saved %s\n", original, len(reduced), checks, destination.Name())
	return errors.Join(reduceErr, reportErr)
}
