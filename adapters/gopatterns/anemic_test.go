package gopatterns

import (
	"context"
	"testing"
)

func loadAnemic(t *testing.T) *Extraction {
	t.Helper()
	ex, err := Extract(context.Background(), "testdata/anemic", []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func TestFindAnemicModelsDetectsOrder(t *testing.T) {
	ex := loadAnemic(t)
	if len(ex.AnemicModels) != 1 {
		t.Fatalf("expected 1 anemic model, got %d", len(ex.AnemicModels))
	}
	hit := ex.AnemicModels[0]
	if hit.TypeName != "Order" {
		t.Fatalf("expected Order, got %q", hit.TypeName)
	}
	if len(hit.Funcs) != 3 {
		t.Fatalf("expected 3 funcs, got %d: %v", len(hit.Funcs), hit.Funcs)
	}
	if hit.Line <= 0 || hit.EndLine <= hit.Line {
		t.Fatalf("bad struct range: %+v", hit)
	}
}

func TestFindAnemicModelsSkipsMethodful(t *testing.T) {
	ex := loadAnemic(t)
	for _, hit := range ex.AnemicModels {
		if hit.TypeName == "Healthy" {
			t.Fatal("Healthy has a method and should not be anemic")
		}
	}
}

func TestFindAnemicModelsSkipsTooFewFuncs(t *testing.T) {
	ex := loadAnemic(t)
	for _, hit := range ex.AnemicModels {
		if hit.TypeName == "Sparse" {
			t.Fatal("Sparse has only 2 funcs and should not be anemic")
		}
	}
}

func TestFindAnemicModelsSkipsUnexported(t *testing.T) {
	ex := loadAnemic(t)
	for _, hit := range ex.AnemicModels {
		if hit.TypeName == "hidden" {
			t.Fatal("hidden is unexported and should not be considered")
		}
	}
}
