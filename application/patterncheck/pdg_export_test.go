package patterncheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
)

func TestPDGJSONExport(t *testing.T) {
	t.Chdir(shapesFixture)
	var output strings.Builder
	if err := Run(context.Background(), []string{"--format", "pdg-json", "."}, &output); err != nil {
		t.Fatal(err)
	}
	var documents []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output.String()), &documents); err != nil {
		t.Fatal(err)
	}
	if len(documents) == 0 {
		t.Fatal("no function graphs")
	}
	for _, doc := range documents {
		if string(doc["schema_version"]) != `"0.1.0"` {
			t.Fatal("wrong schema version")
		}
		if _, ok := doc["candidates"]; ok {
			t.Fatal("export ran report path")
		}
		if len(doc["extraction_evidence"]) == 0 || len(doc["nodes"]) == 0 {
			t.Fatal("incomplete export")
		}
	}
}

var errExportWriter = errors.New("export writer failed")

type brokenExportWriter struct{}

func (brokenExportWriter) Write([]byte) (int, error) { return 0, errExportWriter }

func TestPDGExportWriterAndCancellation(t *testing.T) {
	if err := writePDGArray(context.Background(), brokenExportWriter{}, nil); !errors.Is(err, errExportWriter) {
		t.Fatalf("writer error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output strings.Builder
	if err := writePDGArray(ctx, &output, []gopatterns.FuncPdg{{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	output.Reset()
	if err := writePDGArray(context.Background(), &output, nil); err != nil {
		t.Fatal(err)
	}
	var documents []any
	if err := json.Unmarshal([]byte(output.String()), &documents); err != nil || documents == nil || len(documents) != 0 {
		t.Fatalf("empty export: %q %v", output.String(), err)
	}
}

func TestPDGExportRejectsCacheAndInvalidGraphs(t *testing.T) {
	if _, err := parseOptions([]string{"--format", "pdg-json", "--cache"}); err == nil {
		t.Fatal("export accepted a mining cache option")
	}
	if err := validateExport([]gopatterns.FuncPdg{{Name: "invalid"}}); err == nil {
		t.Fatal("invalid graph accepted")
	}
}
