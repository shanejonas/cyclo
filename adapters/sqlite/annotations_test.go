package sqlite

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shanejonas/cyclomatic-complexity-tui/domain"
)

func TestAnnotationsSurviveReopeningTheDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.db")
	annotation := domain.Annotation{
		ID: "annotation-1", Path: "application/view.go", Function: "renderSourceCodeLines",
		FunctionLine: 295, StartLine: 300, EndLine: 314, Message: "Keep this flat", Text: "switch {",
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	err = store.SaveAnnotation("/workspace/cyclo", annotation)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Close()
	if err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	annotations, err := store.ListAnnotations("/workspace/cyclo")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(annotations, []domain.Annotation{annotation}) {
		t.Fatalf("annotations = %#v, want %#v", annotations, []domain.Annotation{annotation})
	}

	err = store.DeleteAnnotation("/workspace/cyclo", annotation.ID)
	if err != nil {
		t.Fatal(err)
	}
	annotations, err = store.ListAnnotations("/workspace/cyclo")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 0 {
		t.Fatalf("annotations after delete = %#v", annotations)
	}
}
