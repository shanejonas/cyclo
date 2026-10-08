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

func TestPatternsReportsGuardClauses(t *testing.T) {
	t.Chdir("testdata/guard")
	var output strings.Builder
	if err := Run(context.Background(), []string{"."}, &output); err != nil {
		t.Fatalf("patterns must exit 0, got error: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "guard_clause") {
		t.Fatalf("expected a guard_clause candidate, got:\n%s", text)
	}
	if !strings.Contains(text, "parseFlag") {
		t.Fatalf("expected the inverted parseFlag to be flagged, got:\n%s", text)
	}
	// parseOther has a proper guard clause and must not be flagged as
	// guard_clause. (It may appear in semantic_clone output — that's a
	// different kind.)
	if strings.Contains(text, "guard_clause") && strings.Contains(text, "parseOther") {
		// Check if parseOther is in a guard_clause section specifically.
		// Simple heuristic: look for guard_clause followed by parseOther
		// before the next candidate header.
		lines := strings.Split(text, "\n")
		inGuardClause := false
		for _, line := range lines {
			if strings.Contains(line, "guard_clause") && strings.HasPrefix(strings.TrimSpace(line), "#") {
				inGuardClause = true
			} else if strings.HasPrefix(strings.TrimSpace(line), "#") {
				inGuardClause = false
			}
			if inGuardClause && strings.Contains(line, "parseOther") {
				t.Fatalf("proper guard clause parseOther must not be flagged as guard_clause, got:\n%s", text)
			}
		}
	}
}

func TestPatternsFindsTraitMethod(t *testing.T) {
	t.Chdir("testdata/traitmethod")
	var output strings.Builder
	if err := Run(context.Background(), []string{"."}, &output); err != nil {
		t.Fatalf("patterns must exit 0, got error: %v", err)
	}
	text := output.String()
	if !strings.Contains(text, "trait_method") {
		t.Fatalf("expected a trait_method candidate from the parallel Dog/Cat methods, got:\n%s", text)
	}
	if !strings.Contains(text, "sound") {
		t.Fatalf("expected the sound method hole in the candidate, got:\n%s", text)
	}
}
