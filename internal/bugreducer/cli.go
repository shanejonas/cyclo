package bugreducer

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/shanejonas/cyclo/adapters/treesitter"
	"github.com/shanejonas/cyclo/internal/reducer"
)

const usage = `Usage: cyclo bug-reducer [--language lines|go] [--go-parser PATH] [--tui=false] [--output PATH] [--timeout 10s] INPUT -- CHECKER [ARGS...]

Minimize an input while preserving a reproducible bug.
The checker receives an absolute candidate file path as its last argument.
Exit 0 accepts the candidate; any ordinary nonzero exit rejects it.
The checker runs in your current directory.
Terminals show a live dashboard with checker output; scripts get a final summary.
Use --tui to force the dashboard, or --tui=false for plain output.
In the dashboard: tab changes panes, j/k scrolls, enter expands, q stops and saves.
Timeouts, signals, and launch errors stop reduction.
Default mode removes whole lines. --language go removes Tree-sitter Go syntax units.
Go mode requires the tree-sitter CLI and a configured Go grammar, or --go-parser PATH.
Go syntax is checked before the checker; the checker still decides whether the bug remains.
Input must be a regular file.
The original is preserved. Output defaults to INPUT.reduced and must not exist.
Reduction does not guarantee a globally smallest input.
`

type options struct {
	language string
	goParser string
	input    string
	output   string
	command  []string
	timeout  time.Duration
	tui      bool
}

func parseOptions(args []string, output io.Writer) (options, error) {
	flags := flag.NewFlagSet("bug-reducer", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Usage = func() { fmt.Fprint(output, usage) }
	result := options{}
	flags.StringVar(&result.language, "language", "lines", "reduction mode: lines or go (Tree-sitter)")
	flags.StringVar(&result.goParser, "go-parser", "", "Go parser dynamic library for Tree-sitter")
	flags.StringVar(&result.output, "output", "", "new output file")
	flags.DurationVar(&result.timeout, "timeout", 10*time.Second, "timeout per checker invocation")
	flags.BoolVar(&result.tui, "tui", terminalOutput(output), "show the live terminal dashboard")
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
	return result, validateMode(result)
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
	if options.tui {
		return runDashboard(ctx, options, source, check, destination, output)
	}
	reduced, reduceErr := reduceInput(ctx, options, source, check, nil)
	return finish(destination, reduced, reduceErr, check.checks, len(source), output)
}

func terminalOutput(output io.Writer) bool {
	file, ok := output.(*os.File)
	return ok && term.IsTerminal(file.Fd()) && term.IsTerminal(os.Stdin.Fd())
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

func validateMode(options options) error {
	if options.language != "lines" && options.language != "go" {
		return fmt.Errorf("unsupported reduction language: %s", options.language)
	}
	if options.goParser != "" && options.language != "go" {
		return errors.New("--go-parser requires --language go")
	}
	return nil
}

func reduceInput(ctx context.Context, options options, source []byte, check *checker, observe func(reducer.Progress)) ([]byte, error) {
	predicate := func(candidate []byte) (bool, error) { return check.check(ctx, candidate) }
	if options.language == "go" {
		parser := treesitter.GoParser{Context: ctx, Library: options.goParser}
		return reducer.ReduceSyntaxWithProgress(source, predicate, observe, parser)
	}
	return reducer.ReduceWithProgress(source, predicate, observe)
}
