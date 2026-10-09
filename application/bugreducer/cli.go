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
	"github.com/shanejonas/cyclo/adapters/reducer"
	"github.com/shanejonas/cyclo/adapters/treesitter"
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
	return reduction{ctx: ctx, options: options, source: source, check: check, destination: destination, output: output}.run()
}

// reduction carries one reduction run: the input, its checker, and where the
// result and the report go. The run's steps are methods on it so the values
// they all share don't thread through every signature.
type reduction struct {
	ctx         context.Context
	options     options
	source      []byte
	check       *checker
	destination *os.File
	output      io.Writer
}

// run reduces the input, showing the live dashboard on terminals.
func (r reduction) run() error {
	if r.options.tui {
		return r.runDashboard()
	}
	reduced, reduceErr := r.reduce(nil)
	return r.finish(reduced, reduceErr)
}

func (r reduction) reduce(observe func(reducer.Progress)) ([]byte, error) {
	predicate := func(candidate []byte) (bool, error) { return r.check.check(r.ctx, candidate) }
	if r.options.language == "go" {
		parser := treesitter.GoParser{Context: r.ctx, Library: r.options.goParser}
		return reducer.ReduceSyntaxWithProgress(r.source, predicate, observe, parser)
	}
	return reducer.ReduceWithProgress(r.source, predicate, observe)
}

// finish saves the reduced input and reports the run. A nil result means the
// seed was never accepted, so the destination is removed instead.
func (r reduction) finish(reduced []byte, reduceErr error) error {
	if reduced == nil {
		return errors.Join(reduceErr, r.destination.Close(), os.Remove(r.destination.Name()))
	}
	_, writeErr := r.destination.Write(reduced)
	closeErr := r.destination.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(reduceErr, writeErr, closeErr)
	}
	_, reportErr := fmt.Fprintf(r.output, "%d -> %d bytes; %d checks; saved %s\n", len(r.source), len(reduced), r.check.checks, r.destination.Name())
	return errors.Join(reduceErr, reportErr)
}

func terminalOutput(output io.Writer) bool {
	file, ok := output.(*os.File)
	return ok && term.IsTerminal(file.Fd()) && term.IsTerminal(os.Stdin.Fd())
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
