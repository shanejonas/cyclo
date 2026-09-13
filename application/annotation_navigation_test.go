package application

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/domain"
)

func annotationCycleModel() Model {
	report := domain.Report{
		Files: []domain.File{
			{
				Path: "a.go",
				Functions: []domain.Function{
					{
						Name:    "live",
						Line:    1,
						EndLine: 3,
						Source:  "func live() {\n    value := 1\n}",
					},
				},
			},
			{
				Path: "b.go",
				Functions: []domain.Function{
					{
						Name:    "replacement",
						Line:    1,
						EndLine: 3,
						Source:  "func replacement() {\n    return\n}",
					},
				},
			},
		},
	}

	annotations := []Annotation{
		{ID: "n1", Path: "a.go", Function: "live", FunctionLine: 1, StartLine: 2, EndLine: 2, Message: "msg1", Text: "    value := 1"},
		{ID: "n2", Path: "b.go", Function: "removed", FunctionLine: 10, StartLine: 11, EndLine: 11, Message: "msg2", Text: "text2"},
		{ID: "n3", Path: "b.go", Function: "removed", FunctionLine: 10, StartLine: 21, EndLine: 21, Message: "msg3", Text: "text3"},
		{ID: "n4", Path: "b.go", Function: "removed", FunctionLine: 10, StartLine: 31, EndLine: 31, Message: "msg4", Text: "text4"},
	}

	model := Model{width: 120, height: 40, report: report, annotations: annotations}
	return model.withReport(reportMsg{report: report})
}

func TestAnnotationNavigationAdvancesAndWrapsAllSavedNotes(t *testing.T) {
	for _, focus := range []pane{filesPane, functionsPane, detailsPane} {
		model := annotationCycleModel()
		model.focus = focus
		for _, step := range []struct {
			key string
			ids []string
		}{
			{"]", []string{"n1", "n2", "n3", "n4", "n1", "n2", "n3", "n4", "n1"}},
			{"[", []string{"n4", "n3", "n2", "n1", "n4", "n3", "n2", "n1"}},
		} {
			for _, id := range step.ids {
				revision := model.revision
				model = pressSourceKey(t, model, step.key)
				if model.activeAnnotationID != id || model.revision != revision+1 {
					t.Fatalf("pane=%d key=%s: active=%s revision=%d, want %s/%d", focus, step.key, model.activeAnnotationID, model.revision, id, revision+1)
				}
				if id == "n1" {
					continue
				}
				if _, selected := model.selectedFunction(); selected {
					t.Fatal("unmatched note selected current code")
				}
				number := strings.TrimPrefix(id, "n")
				view := ansi.Strip(model.View().Content)
				for _, want := range []string{"msg" + number, "text" + number, "unmatched", "notes (" + number + "/4"} {
					if !strings.Contains(view, want) {
						t.Fatalf("note %s hides %q", id, want)
					}
				}
			}
		}
	}
}

func TestMissingFileAnnotationRemainsNavigable(t *testing.T) {
	for _, width := range []int{60, 120} {
		model := Model{width: width, height: 30, annotations: []Annotation{
			{ID: "missing", Path: "missing.go", Function: "removed", FunctionLine: 1, StartLine: 2, EndLine: 2, Message: "missing message", Text: "missing text"},
		}}
		for _, key := range []string{"]", "]", "[", "]"} {
			model = pressSourceKey(t, model, key)
			if model.activeAnnotationID != "missing" || len(model.annotations) != 1 {
				t.Fatal("missing-file note was skipped or lost")
			}
			if _, selected := model.selectedFunction(); selected {
				t.Fatal("missing-file note selected current code")
			}
			view := ansi.Strip(model.View().Content)
			for _, want := range []string{"missing message", "missing text", "unmatched"} {
				if !strings.Contains(view, want) {
					t.Fatalf("missing-file view hides %q", want)
				}
			}
		}
	}
}

func TestSavedAnnotationScrollResetsWhenAdvancing(t *testing.T) {
	for _, width := range []int{60, 120} {
		lines := make([]string, 40)
		for i := range lines {
			lines[i] = fmt.Sprintf("saved line %d", i+1)
		}
		model := Model{width: width, height: 30, annotations: []Annotation{
			{ID: "long", Path: "gone.go", Function: "gone", FunctionLine: 1, StartLine: 1, EndLine: 40, Message: "long message", Text: strings.Join(lines, "\n")},
			{ID: "short", Path: "gone.go", Function: "gone", FunctionLine: 1, StartLine: 41, EndLine: 41, Message: "short message", Text: "short saved code"},
		}}
		model = pressSourceKey(t, model, "]")
		for range 100 {
			model = pressSourceKey(t, model, "j")
		}
		if model.sourceOffset == 0 || model.activeAnnotationID != "long" || !strings.Contains(ansi.Strip(model.View().Content), "saved line 40") {
			t.Fatal("cannot scroll to the end of a saved snippet")
		}
		model = pressSourceKey(t, model, "]")
		if model.activeAnnotationID != "short" || model.sourceOffset != 0 || !strings.Contains(ansi.Strip(model.View().Content), "short saved code") {
			t.Fatal("next saved note did not reset scrolling")
		}
		model = pressSourceKey(t, model, "d")
		if len(model.annotations) != 1 || model.annotations[0].ID != "long" {
			t.Fatal("could not remove the selected unmatched note")
		}
		model = pressSourceKey(t, model, "]")
		if model.activeAnnotationID != "long" {
			t.Fatal("navigation failed after removing an unmatched note")
		}
	}
}
