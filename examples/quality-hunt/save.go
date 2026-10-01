package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type failureRecord struct {
	Failure failure
	Cases   []specimen
	Checks  int
}

func saveFailure(directory string, record failureRecord, reduced []byte) error {
	metadata, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(directory, "failure.json"), append(metadata, '\n'), 0600); err != nil {
		return err
	}
	if reduced == nil {
		return nil
	}
	return os.WriteFile(filepath.Join(directory, "reduced.go.txt"), reduced, 0600)
}

func loadInput(directory string) ([]specimen, []byte, error) {
	if directory == "" {
		cases, err := corpus()
		return cases, program(cases), err
	}
	metadata, err := os.ReadFile(filepath.Join(directory, "failure.json"))
	if err != nil {
		return nil, nil, err
	}
	var record failureRecord
	if err := json.Unmarshal(metadata, &record); err != nil {
		return nil, nil, err
	}
	if len(record.Cases) == 0 {
		return nil, nil, fmt.Errorf("failure metadata has no cases")
	}
	source, err := os.ReadFile(filepath.Join(directory, "reduced.go.txt"))
	return record.Cases, source, err
}
