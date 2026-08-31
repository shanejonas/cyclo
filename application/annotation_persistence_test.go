package application

import (
	"path/filepath"
	"testing"

	"github.com/shanejonas/cyclo/adapters/sqlite"
	"github.com/shanejonas/cyclo/domain"
)

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
