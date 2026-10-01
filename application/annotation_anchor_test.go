package application

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/adapters/sqlite"
	"github.com/shanejonas/cyclo/domain"
)

func TestAnnotationNavigationRelocatesSavedNotesFromEveryPane(t *testing.T) {
	for _, focus := range []pane{filesPane, functionsPane, detailsPane} {
		for _, key := range []string{"[", "]"} {
			model := sourceWorkspaceModel()
			note, _ := model.newAnnotation(11, 11, "keep this value")
			model.annotations = []Annotation{note}
			function := &model.report.Files[0].Functions[0]
			function.Line, function.EndLine = 40, 53
			function.Source = strings.Replace(function.Source, "    value := 1", "    // inserted line\n    value := 1", 1)
			model = model.withReport(reportMsg{report: model.report})
			model.focus = focus
			model = pressSourceKey(t, model, key)
			if model.focus != detailsPane || model.activeAnnotationID != note.ID || model.sourceLine() != 42 {
				t.Fatalf("pane=%d key=%s: active=%q line=%d focus=%d", focus, key, model.activeAnnotationID, model.sourceLine(), model.focus)
			}
			if !strings.Contains(ansi.Strip(model.View().Content), note.Message) {
				t.Fatal("jump hides the relocated annotation")
			}
			if model.annotations[0].FunctionLine != 40 || model.annotations[0].StartLine != 42 {
				t.Fatalf("annotation coordinates are stale: %+v", model.annotations[0])
			}
		}
	}
}

func TestRemovedFunctionDoesNotAttachNotesToReusedLineNumber(t *testing.T) {
	model := sourceWorkspaceModel()
	note, _ := model.newAnnotation(11, 11, "old note")
	model.annotations = []Annotation{note}
	model.report.Files[0].Functions[0] = domain.Function{Name: "replacement", Line: 10, EndLine: 12, Source: "func replacement() {\n    return\n}"}
	model = model.withReport(reportMsg{report: model.report})
	model = pressSourceKey(t, model, "]")
	_, selected := model.selectedFunction()
	if model.activeAnnotationID != note.ID || selected || len(model.visibleAnnotations()) != 0 {
		t.Fatal("removed function's note attached to unrelated code")
	}
	if len(model.annotations) != 1 || model.annotations[0] != note {
		t.Fatal("unmatched note was changed or discarded")
	}
	if strings.Contains(ansi.Strip(model.functionRow(model.report.Files[0].Functions[0], 40, false)), "◆") {
		t.Fatal("replacement function is marked as annotated")
	}
	if !strings.Contains(ansi.Strip(model.footer(160)), "1 unmatched") {
		t.Fatal("navigation gives no indication that the saved note is unmatched")
	}
}

func TestAnnotationRangeClampsWhenItsFunctionShrinks(t *testing.T) {
	model := sourceWorkspaceModel()
	note, _ := model.newAnnotation(19, 20, "simplify this")
	model.annotations = []Annotation{note}
	function := &model.report.Files[0].Functions[0]
	function.Line, function.EndLine = 40, 42
	function.Source = "func inspect() error {\n    return nil\n}"
	model = model.withReport(reportMsg{report: model.report})
	model = pressSourceKey(t, model, "]")
	if model.sourceLine() != 42 || model.annotations[0].StartLine != 42 || model.annotations[0].EndLine != 42 {
		t.Fatalf("relocated range is outside the function: %+v", model.annotations[0])
	}
	if !strings.Contains(ansi.Strip(model.View().Content), note.Message) {
		t.Fatal("clamped annotation is not visible")
	}
}

func TestRenamedFunctionRequiresUniqueAnnotationText(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		model := sourceWorkspaceModel()
		note, _ := model.newAnnotation(11, 11, "keep this value")
		model.annotations = []Annotation{note}
		function := &model.report.Files[0].Functions[0]
		function.Name, function.Line, function.EndLine = "renamed", 40, 52
		if ambiguous {
			duplicate := *function
			duplicate.Name, duplicate.Line, duplicate.EndLine = "duplicate", 80, 92
			model.report.Files[0].Functions = append(model.report.Files[0].Functions, duplicate)
		}
		model = model.withReport(reportMsg{report: model.report})
		model = pressSourceKey(t, model, "]")
		_, selected := model.selectedFunction()
		if ambiguous && selected {
			t.Fatal("ambiguous text selected an arbitrary function")
		}
		if !ambiguous && (model.activeAnnotationID != note.ID || model.sourceLine() != 41 || model.annotations[0].Function != "renamed") {
			t.Fatalf("unique text failed to relocate the note: %+v", model.annotations[0])
		}
	}
}

func TestRelocationPreservesCRLFAndTrailingSnippetRows(t *testing.T) {
	function := domain.Function{Line: 40, Source: "prefix\r\n\tfirst\r\n\tsecond\r\n"}
	annotation := Annotation{
		FunctionLine: 10, StartLine: 11, EndLine: 13,
		Text: "\tfirst\r\n\tsecond\r\n",
	}
	relocated := relocateAnnotation(annotation, function)
	if relocated.StartLine != 41 || relocated.EndLine != 43 {
		t.Fatalf("relocated range = %d-%d, want 41-43", relocated.StartLine, relocated.EndLine)
	}
}

func TestReloadRelocatesNotesWithoutOverwritingSavedAnchors(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "annotations.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	model := sourceWorkspaceModel()
	model.report.Root = "/workspace"
	model.annotationStore = store
	note, _ := model.newAnnotation(11, 11, "persisted note")
	model, err = model.saveAnnotation(note)
	if err != nil {
		t.Fatal(err)
	}
	model.report.Files[0].Functions[0].Line = 40
	model.report.Files[0].Functions[0].EndLine = 52
	restarted := Model{annotationStore: store}.withReport(reportMsg{report: model.report})
	restarted = pressSourceKey(t, restarted, "]")
	if restarted.activeAnnotationID != note.ID || restarted.sourceLine() != 41 {
		t.Fatal("reloaded annotation did not follow its function")
	}
	saved, err := store.ListAnnotations(model.report.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0] != note {
		t.Fatal("derived relocation overwrote the saved anchor")
	}
}
