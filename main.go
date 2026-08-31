package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/adapters/sqlite"
	"github.com/shanejonas/cyclo/application"
)

const defaultControlPort = 8197

type runOptions struct {
	controlPort int
	paths       []string
}

func main() {
	err := run(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) (result error) {
	if len(args) > 0 && args[0] == "--skill" {
		if len(args) != 1 {
			return errors.New("usage: cyclo --skill")
		}
		return application.WriteSkill(output)
	}

	options, err := parseRunOptions(args)
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

	analyzer := gocyclo.NewAnalyzer()
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
	err := flags.Parse(args)
	if err != nil {
		return runOptions{}, err
	}
	if *controlPort < 0 || *controlPort > 65535 {
		return runOptions{}, errors.New("control port must be between 0 and 65535")
	}

	return runOptions{controlPort: *controlPort, paths: flags.Args()}, nil
}
