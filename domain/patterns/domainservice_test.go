package patterns

import (
	"strings"
	"testing"
)

func TestDomainServiceCandidatesProposeService(t *testing.T) {
	hits := []DomainServiceHit{{
		FuncName: "Transfer",
		Path:     "bank.go",
		Line:     20,
		EndLine:  30,
		Types:    []string{"Account", "Money"},
	}}
	cands := domainServiceCandidates(hits)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Kind != DomainService {
		t.Fatalf("expected domain_service kind, got %q", c.Kind)
	}
	if c.FixSpec != nil {
		t.Fatal("domain_service is detection-only; FixSpec must be nil")
	}
	if !strings.Contains(c.Observation, "Transfer") || !strings.Contains(c.Observation, "Account") {
		t.Fatalf("observation should name func and types: %q", c.Observation)
	}
	if !strings.Contains(c.PossibleRefactor, "TransferService") {
		t.Fatalf("refactor should suggest a service type: %q", c.PossibleRefactor)
	}
}

func TestDomainServiceSkipsSingleType(t *testing.T) {
	hits := []DomainServiceHit{{
		FuncName: "Calculate",
		Path:     "x.go",
		Line:     1,
		EndLine:  5,
		Types:    []string{"Order"},
	}}
	if cands := domainServiceCandidates(hits); len(cands) != 0 {
		t.Fatalf("expected no candidate for 1 type, got %d", len(cands))
	}
}

func TestDomainServiceEmptyHits(t *testing.T) {
	if cands := domainServiceCandidates(nil); len(cands) != 0 {
		t.Fatalf("expected no candidates for nil hits, got %d", len(cands))
	}
}

func TestAnemicModelSuppressesServiceFuncs(t *testing.T) {
	hits := []AnemicModelHit{{
		TypeName: "Account",
		Path:     "bank.go",
		Line:     10,
		EndLine:  15,
		Funcs:    []string{"GetBalance", "Deposit", "Transfer"},
	}}
	services := []DomainServiceHit{{
		FuncName: "Transfer",
		Types:    []string{"Account", "Money"},
	}}
	cands := anemicModelCandidates(hits, services)
	// Transfer filtered out leaves 2 funcs < 3 threshold: no candidate.
	if len(cands) != 0 {
		t.Fatalf("expected suppressed candidate, got %d", len(cands))
	}
}

func TestAnemicModelKeepsNonServiceFuncs(t *testing.T) {
	hits := []AnemicModelHit{{
		TypeName: "Account",
		Path:     "bank.go",
		Line:     10,
		EndLine:  15,
		Funcs:    []string{"GetBalance", "Deposit", "Withdraw", "Transfer"},
	}}
	services := []DomainServiceHit{{
		FuncName: "Transfer",
		Types:    []string{"Account", "Money"},
	}}
	cands := anemicModelCandidates(hits, services)
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if strings.Contains(c.PossibleRefactor, "Transfer") {
		t.Fatalf("refactor should not mention suppressed func: %q", c.PossibleRefactor)
	}
	if !strings.Contains(c.Observation, "3 functions") {
		t.Fatalf("observation should count 3 remaining funcs: %q", c.Observation)
	}
}

func TestDomainServiceWiredIntoRun(t *testing.T) {
	hits := []DomainServiceHit{{
		FuncName: "Transfer",
		Path:     "bank.go",
		Line:     20,
		EndLine:  30,
		Types:    []string{"Account", "Money"},
	}}
	report := RunMining(nil, Options{DomainServices: hits})
	found := false
	for _, c := range report.Candidates {
		if c.Kind == DomainService {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Run should include domain_service candidates from Options")
	}
}

func TestDomainServiceOrderedBeforeAnemic(t *testing.T) {
	if FixKindRank(DomainService) >= FixKindRank(AnemicModel) {
		t.Fatalf("domain_service (rank %d) must sort before anemic_model (rank %d) for suppression",
			FixKindRank(DomainService), FixKindRank(AnemicModel))
	}
}
