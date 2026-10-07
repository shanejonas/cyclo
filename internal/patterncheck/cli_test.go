package patterncheck

import (
	"context"
	"strings"
	"testing"
)

// shapesFixture is the gopatterns extractor fixture, a separate module with
// structurally parallel function pairs.
const shapesFixture = "../../adapters/gopatterns/testdata/shapes"

func TestPatternsFindsCandidatesAndExitsZero(t *testing.T) {
	t.Chdir(shapesFixture)
	var output strings.Builder
	if err := Run(context.Background(), []string{"."}, &output); err != nil {
		t.Fatalf("patterns must exit 0 on success, got error: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "candidates") {
		t.Fatalf("expected candidates section, got:\n%s", text)
	}
	if !strings.Contains(text, "CreateUser") || !strings.Contains(text, "CreateOrder") {
		t.Fatalf("expected the CreateUser/CreateOrder parallel pair, got:\n%s", text)
	}
}

func TestPatternsEmptyReportStillExitsZero(t *testing.T) {
	t.Chdir("testdata/single")
	var output strings.Builder
	if err := Run(context.Background(), []string{"."}, &output); err != nil {
		t.Fatalf("empty report must still exit 0, got error: %v", err)
	}
	if !strings.Contains(output.String(), "0 candidates") {
		t.Fatalf("expected empty candidates section, got:\n%s", output.String())
	}
}

func TestPatternsJSONFormat(t *testing.T) {
	t.Chdir(shapesFixture)
	var output strings.Builder
	if err := Run(context.Background(), []string{"--format", "json", "."}, &output); err != nil {
		t.Fatalf("json format: %v", err)
	}
	if !strings.Contains(output.String(), `"candidates"`) {
		t.Fatalf("expected JSON candidates key, got:\n%s", output.String())
	}
}

func TestPatternsRejectsBadOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--format", "yaml"},
		{"--threshold", "1001"},
	} {
		var output strings.Builder
		if err := Run(context.Background(), args, &output); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}
