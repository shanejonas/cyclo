package gopatterns

import (
	"context"
	"testing"
)

func loadDomainService(t *testing.T) *Extraction {
	t.Helper()
	ex, err := Extract(context.Background(), "testdata/domainservice", []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func TestFindDomainServicesDetectsTransfer(t *testing.T) {
	ex := loadDomainService(t)
	if len(ex.DomainServices) != 1 {
		t.Fatalf("expected 1 domain service, got %d", len(ex.DomainServices))
	}
	hit := ex.DomainServices[0]
	if hit.FuncName != "Transfer" {
		t.Fatalf("expected Transfer, got %q", hit.FuncName)
	}
	if len(hit.Types) != 2 {
		t.Fatalf("expected 2 types, got %v", hit.Types)
	}
	typeSet := map[string]bool{}
	for _, ty := range hit.Types {
		typeSet[ty] = true
	}
	if !typeSet["Account"] || !typeSet["Money"] {
		t.Fatalf("expected Account and Money, got %v", hit.Types)
	}
	if hit.Line <= 0 || hit.EndLine <= hit.Line {
		t.Fatalf("bad func range: %+v", hit)
	}
}

func TestFindDomainServicesSkipsSingleType(t *testing.T) {
	ex := loadDomainService(t)
	for _, hit := range ex.DomainServices {
		if hit.FuncName == "SingleType" {
			t.Fatal("SingleType touches one struct and should not be a service")
		}
	}
}

func TestFindDomainServicesSkipsGlobalMutation(t *testing.T) {
	ex := loadDomainService(t)
	for _, hit := range ex.DomainServices {
		if hit.FuncName == "GlobalSink" {
			t.Fatal("GlobalSink mutates a global and is not stateless")
		}
	}
}

func TestFindDomainServicesSkipsNoFieldAccess(t *testing.T) {
	ex := loadDomainService(t)
	for _, hit := range ex.DomainServices {
		if hit.FuncName == "NoFieldAccess" {
			t.Fatal("NoFieldAccess touches no fields and should not be a service")
		}
	}
}
