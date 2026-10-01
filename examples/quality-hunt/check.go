package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/shanejonas/cyclo/adapters/goquality"
	"github.com/shanejonas/cyclo/domain/quality"
)

var invalidCandidate = errors.New("candidate does not compile or run")

type runner struct {
	root, binary string
	timeout      time.Duration
}
type failure struct{ Name, Base, Rule string }
type evidence struct {
	Complete       bool
	Targets, Calls int
	Mutations      []string
	Effects        []quality.Kind
}

func (r runner) assess(ctx context.Context, source []byte, cases []specimen) (failure, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if err := equivalentSource(source, cases); err != nil {
		return failure{}, err
	}
	if err := r.write(source, cases); err != nil {
		return failure{}, err
	}
	observed, err := r.runOracle(ctx)
	if err != nil {
		return failure{}, err
	}
	facts, err := r.extract(ctx)
	if err != nil {
		return failure{}, err
	}
	report, err := quality.Evaluate(facts, quality.DefaultConfig())
	if err != nil {
		return failure{}, err
	}
	return compare(cases, observed, facts, report)
}

func (r runner) write(source []byte, cases []specimen) error {
	files := map[string][]byte{"go.mod": []byte("module example.com/quality-hunt\n\ngo 1.25.0\n"), "input.go": source, "oracle/main.go": oracle(cases)}
	if err := os.MkdirAll(filepath.Join(r.root, "oracle"), 0700); err != nil {
		return err
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(r.root, name), content, 0600); err != nil {
			return err
		}
	}
	return nil
}

func (r runner) runOracle(ctx context.Context) (map[string]bool, error) {
	command := exec.CommandContext(ctx, "go", "run", "./oracle")
	command.Dir = r.root
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if _, ok := err.(*exec.ExitError); ok {
		return nil, fmt.Errorf("%w: %s", invalidCandidate, stderr.String())
	}
	if err != nil {
		return nil, err
	}
	var observed map[string]bool
	if err := json.Unmarshal(output, &observed); err != nil {
		return nil, err
	}
	return observed, nil
}

func (r runner) extract(ctx context.Context) ([]quality.Function, error) {
	if r.binary == "" {
		return (goquality.Analyzer{Root: r.root}).Extract(ctx, []string{"input.go"})
	}
	command := exec.CommandContext(ctx, r.binary, "check", "--format", "facts", "input.go")
	command.Dir = r.root
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("analyzer failed: %w: %s", err, stderr.String())
	}
	var facts struct {
		Functions []quality.Function `json:"functions"`
	}
	if err := json.Unmarshal(output, &facts); err != nil {
		return nil, err
	}
	return facts.Functions, nil
}

func compare(cases []specimen, observed map[string]bool, facts []quality.Function, report quality.Report) (failure, error) {
	proofs := map[string]evidence{}
	results := map[string]quality.FunctionResult{}
	for _, result := range report.Functions {
		results[shortName(result.Name)] = result
	}
	for _, fact := range facts {
		proofs[shortName(fact.Name)] = signature(fact, results[shortName(fact.Name)])
	}
	for _, c := range cases {
		if err := validateObservation(c, observed, proofs); err != nil {
			return failure{}, err
		}
		if rule := violation(c, observed[c.Name], proofs, results); rule != "" {
			return failure{Name: c.Name, Base: c.Base, Rule: rule}, nil
		}
	}
	return failure{}, nil
}

func validateObservation(c specimen, observed map[string]bool, proofs map[string]evidence) error {
	changed, ok := observed[c.Name]
	if !ok || changed != c.Changed {
		return fmt.Errorf("%w: runtime expectation failed for %s", invalidCandidate, c.Name)
	}
	if _, ok := proofs[c.Name]; !ok {
		return fmt.Errorf("analyzer omitted %s", c.Name)
	}
	return nil
}

func violation(c specimen, changed bool, proofs map[string]evidence, results map[string]quality.FunctionResult) string {
	result := results[c.Name]
	if hiddenWrite(changed, result) {
		return "hidden-caller-write"
	}
	if ownedEffect(c.Clean, result) {
		return "owned-storage-effect"
	}
	if !reflect.DeepEqual(proofs[c.Base], proofs[c.Name]) {
		return "equivalent-syntax-evidence"
	}
	return ""
}

func signature(f quality.Function, result quality.FunctionResult) evidence {
	var mutations []string
	var effects []quality.Kind
	roots := map[string]int{}
	for _, mutation := range f.Mutations {
		if _, ok := roots[mutation.RootID]; !ok {
			roots[mutation.RootID] = len(roots)
		}
		mutations = append(mutations, fmt.Sprintf("%d:%s:%s", roots[mutation.RootID], mutation.FieldPath, mutation.Provenance))
	}
	for _, effect := range result.Effects {
		effects = append(effects, effect.Kind)
	}
	return evidence{Complete: result.Complete, Targets: result.MutatedTargets, Calls: result.UnclassifiedCalls, Mutations: mutations, Effects: effects}
}

func shortName(name string) string { return name[strings.LastIndex(name, ".")+1:] }

func hiddenWrite(changed bool, result quality.FunctionResult) bool {
	return changed && result.Complete && len(result.Effects) == 0
}

func ownedEffect(clean bool, result quality.FunctionResult) bool {
	return clean && (!result.Complete || len(result.Effects) > 0)
}
