package application

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/adapters/sqlite"
	"github.com/shanejonas/cyclo/domain"
)

func TestDraftSaveFailurePreservesTheDraftAndSelection(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "annotations.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	model := sourceWorkspaceModel().WithAnnotationStore(store)
	model = model.selectFromAnchor(model.sourceLine())
	model.visualSelectionActive = true
	model.annotating = true
	model.annotationDraft = "  keep this draft  "
	model.activeAnnotationID = "existing-note"
	selection := model.lineSelection

	next := model.saveDraftAnnotation()

	if next.annotationError == nil || !strings.HasPrefix(next.annotationError.Error(), "save annotation:") {
		t.Fatalf("save error = %v, want contextual persistence failure", next.annotationError)
	}
	if next.annotating || !next.visualSelectionActive || next.lineSelection != selection || next.annotationDraft != model.annotationDraft {
		t.Fatal("save failure must close input while preserving the draft and selection")
	}
	if len(next.annotations) != 0 || next.activeAnnotationID != model.activeAnnotationID || next.nextAnnotationID != 1 {
		t.Fatal("save failure must preserve notes and reserve the attempted annotation ID")
	}
}

func TestModelReloadsPersistedAnnotations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.db")
	report := domain.Report{Root: "/workspace/cyclo"}
	annotation := Annotation{ID: "annotation-1", Message: "Keep this flat"}

	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	model := Model{report: report, annotationStore: store}
	model, err = model.saveAnnotation(annotation)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Close()
	if err != nil {
		t.Fatal(err)
	}

	store, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	restarted := Model{annotationStore: store}.withReport(reportMsg{report: report})
	if len(restarted.annotations) != 1 || restarted.annotations[0].Message != annotation.Message {
		t.Fatalf("restarted annotations = %#v, want %#v", restarted.annotations, []Annotation{annotation})
	}
	if restarted.nextAnnotationID != 1 {
		t.Fatalf("next annotation sequence = %d, want 1", restarted.nextAnnotationID)
	}
}
