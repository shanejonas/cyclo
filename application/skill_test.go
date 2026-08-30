package application

import (
	"strings"
	"testing"
)

func TestWriteSkillRequiresAWriter(t *testing.T) {
	err := WriteSkill(nil)
	if err == nil {
		t.Fatal("WriteSkill(nil) succeeded")
	}
}

func TestSkillScopesPullRequestAnalysisToChangedGoFiles(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	want := "git diff --name-only -z --diff-filter=ACMR main -- '*.go' | xargs -0 -r -o cyclo"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("skill does not scope analysis to the pull-request diff:\n%s", output.String())
	}
}

func TestSkillExplainsTheAgentControlAPI(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"rpc.discover", "cyclo.getState", "cyclo.waitForChange", "cyclo.selectFile", "cyclo.selectFunction",
		"cyclo.revealLines", "cyclo.annotateLines", "cyclo.removeAnnotation", "cyclo.refresh",
	}
	for _, method := range want {
		if !strings.Contains(output.String(), method) {
			t.Fatalf("skill does not mention %s:\n%s", method, output.String())
		}
	}
}

func TestSkillTreatsTheTUIAsASharedScreen(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"shared screen",
		"rpc()",
		"JSON-RPC `error` envelope",
		"runtime OpenRPC document is the authority",
		"instead of polling",
	}
	for _, instruction := range want {
		if !strings.Contains(output.String(), instruction) {
			t.Fatalf("skill does not explain %q:\n%s", instruction, output.String())
		}
	}
}

func TestSkillExplainsPersistentAnnotations(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"persist across restarts", "XDG_STATE_HOME", "annotations.db"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("skill does not explain %q:\n%s", text, output.String())
		}
	}
}

func TestSkillExplainsAnnotationNavigation(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"change files from any pane", "previous or next note across the report", "amber diamonds"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("skill does not explain %q:\n%s", text, output.String())
		}
	}
}

func TestSkillExplainsCognitiveComplexity(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"`COG`", "purple", "cyclomaticDiagnostics", "cognitiveDiagnostics"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("skill does not explain %q:\n%s", text, output.String())
		}
	}
}

func TestSkillExplainsAutomaticGitDiffs(t *testing.T) {
	var output strings.Builder
	err := WriteSkill(&output)
	if err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{"automatically shows the diff", "`main` or `master`", "Green `+` gutters", "Red `−` rows"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("skill does not explain %q:\n%s", text, output.String())
		}
	}
}
