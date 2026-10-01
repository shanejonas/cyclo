package application

import (
	"slices"
	"testing"
)

func TestRemovingAnnotationPreservesPreviousModel(t *testing.T) {
	notes := []Annotation{{ID: "first"}, {ID: "second"}, {ID: "third"}}
	original := Model{annotations: slices.Clone(notes)}
	next, err := original.removeAnnotation("first")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(original.annotations, notes) || !slices.Equal(next.annotations, notes[1:]) {
		t.Fatalf("remove changes previous model: original=%v, next=%v", original.annotations, next.annotations)
	}
	next.annotations[0].Message = "changed"
	if original.annotations[1].Message != "" {
		t.Fatal("returned annotations still share storage with the previous model")
	}
}

func TestSavingAnnotationsFromOneModelKeepsIndependentResults(t *testing.T) {
	original := Model{annotations: make([]Annotation, 1, 3)}
	original.annotations[0] = Annotation{ID: "first"}
	left, err := original.saveAnnotation(Annotation{ID: "left"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := original.saveAnnotation(Annotation{ID: "right"})
	if err != nil {
		t.Fatal(err)
	}
	if left.annotations[1].ID != "left" || right.annotations[1].ID != "right" {
		t.Fatalf("saving reuses another result's storage: left=%v, right=%v", left.annotations, right.annotations)
	}
	left.annotations[0].Message = "changed"
	if original.annotations[0].Message != "" || right.annotations[0].Message != "" {
		t.Fatal("saved annotations share storage across model snapshots")
	}
}
