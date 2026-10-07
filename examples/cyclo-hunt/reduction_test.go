package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/shanejonas/cyclo/adapters/reducer"
)

func TestReductionPreservesSpecificFunctionFailure(t *testing.T) {
	source := []byte("package p\nvar idle = func() {}\nvar branch = func() {\n if true {}\n}\nvar nested = func() { if true { if false {} } }\n")
	directory := t.TempDir()
	predicate := func(candidate []byte) (bool, error) {
		failure, err := simulatedMissingBranchScore(directory, candidate)
		return failure == "equivalent-function-score:branch", err
	}
	reduced, err := reducer.Reduce(source, predicate)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reduced, []byte("idle")) || bytes.Contains(reduced, []byte("nested")) {
		t.Fatalf("unrelated functions remain: %s", reduced)
	}
	ok, err := predicate(reduced)
	if err != nil || !ok {
		t.Fatalf("failure lost: %s, %v", reduced, err)
	}
	if len(reduced) >= len(source) {
		t.Fatal("did not shrink")
	}
}

func simulatedMissingBranchScore(directory string, source []byte) (string, error) {
	targets, valid := parseTargets(source)
	if !valid {
		return "", nil
	}
	baselineSource, err := namedBaseline(targets)
	if err != nil {
		return "", err
	}
	baseline, err := analyze(filepath.Join(directory, "baseline.go"), baselineSource)
	if err != nil {
		return "", err
	}
	actual, err := analyze(filepath.Join(directory, "candidate.go"), source)
	if err != nil {
		return "", err
	}
	for index := range actual.Files[0].Functions {
		function := &actual.Files[0].Functions[index]
		if function.Name == "branch" {
			function.CognitiveComplexity = 0
		}
	}
	return compareReports(targets, source, baseline, actual), nil
}
