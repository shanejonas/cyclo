package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/adapters/goquality"
	"github.com/shanejonas/cyclo/adapters/sqlite"
	"github.com/shanejonas/cyclo/application"
	"github.com/shanejonas/cyclo/internal/bugreducer"
	"github.com/shanejonas/cyclo/internal/patterncheck"
	"github.com/shanejonas/cyclo/internal/qualitycheck"
)

const defaultControlPort = 8197

type runOptions struct {
	controlPort int
	paths       []string
	config      string
}

func main() {
	err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(commandExitCode(os.Args[1:], err))
	}
}

// commandExitCode maps a failed subcommand to its process exit code.
// Findings are exit 1; hard failures exit 2. patterns never reports
// findings, so any of its errors is a hard failure.
func commandExitCode(args []string, err error) int {
	if len(args) == 0 {
		return 1
	}
	switch args[0] {
	case "check":
		if errors.Is(err, qualitycheck.ErrFindings) {
			return 1
		}
		return 2
	case "patterns":
		return 2
	default:
		return 1
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return runDefault(args, output)
	}
	return runCommand(args, output)
}

func runCommand(args []string, output io.Writer) error {
	if args[0] == "--skill" {
		return writeSkill(args, output)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), reducerSignals()...)
	defer cancel()
	return runSubcommand(ctx, args, output)
}

// runSubcommand dispatches the headless subcommands; anything else runs
// the default TUI.
func runSubcommand(ctx context.Context, args []string, output io.Writer) error {
	switch args[0] {
	case "check":
		return qualitycheck.Run(ctx, args[1:], output)
	case "bug-reducer":
		return bugreducer.Run(ctx, args[1:], output)
	case "patterns":
		return patterncheck.Run(ctx, args[1:], output)
	default:
		return runDefault(args, output)
	}
}

func runDefault(args []string, output io.Writer) error {
	options, err := parseRunOptions(args)
	if err != nil {
		return err
	}
	return runTUI(options, output)
}

func runTUI(options runOptions, output io.Writer) (result error) {
	config, err := qualitycheck.LoadConfig(options.config)
	if err != nil {
		return err
	}
	statePath, err := sqlite.StatePath()
	if err != nil {
		return err
	}
	store, err := sqlite.Open(statePath)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, store.Close())
	}()

	control, err := application.NewControlServer(options.controlPort)
	if err != nil {
		return err
	}

	analyzer := goquality.ReportAnalyzer{Complexity: gocyclo.NewAnalyzer(), Config: config, Tests: true}
	model := application.NewModel(analyzer, options.paths).
		WithAnnotationStore(store).
		WithControlPort(control.Port())
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(output))
	control.Start(program.Send)
	_, programErr := program.Run()
	closeErr := control.Close()
	return errors.Join(programErr, closeErr)
}

func parseRunOptions(args []string) (runOptions, error) {
	flags := flag.NewFlagSet("cyclo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	controlPort := flags.Int("control-port", defaultControlPort, "localhost JSON-RPC control port")
	config := flags.String("config", "", "quality TOML policy")
	if err := flags.Parse(args); err != nil {
		return runOptions{}, err
	}
	if *controlPort < 0 || *controlPort > 65535 {
		return runOptions{}, errors.New("control port must be between 0 and 65535")
	}

	return runOptions{controlPort: *controlPort, paths: flags.Args(), config: *config}, nil
}

func writeSkill(args []string, output io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: cyclo --skill")
	}
	return application.WriteSkill(output)
}
