package patterns

import (
	"strings"
	"testing"
)

func primitiveFact(name, path string, line int, params []ParamInfo) *MiningFacts {
	return &MiningFacts{
		ID:      "pkg." + name,
		Name:    name,
		Path:    path,
		Line:    line,
		EndLine: line + 10,
		Params:  params,
	}
}

func TestPrimitiveObsessionDetectsEmail(t *testing.T) {
	facts := []*MiningFacts{
		primitiveFact("SendEmail", "a.go", 10, []ParamInfo{{Name: "recipientEmail", Type: "string"}, {Name: "subject", Type: "string"}}),
		primitiveFact("ValidateEmail", "b.go", 20, []ParamInfo{{Name: "email", Type: "string"}}),
	}
	cs := primitiveObsessionCandidates(facts)
	if len(cs) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(cs))
	}
	c := cs[0]
	if c.Kind != PrimitiveObsession {
		t.Errorf("kind = %q, want primitive_obsession", c.Kind)
	}
	if c.ScoreMilli != 350 {
		t.Errorf("score = %d, want 350", c.ScoreMilli)
	}
	if !strings.Contains(c.PossibleRefactor, "type Email string") {
		t.Errorf("refactor should propose type Email string, got %q", c.PossibleRefactor)
	}
	if len(c.Sites) != 2 {
		t.Errorf("expected 2 sites, got %d", len(c.Sites))
	}
}

func TestPrimitiveObsessionNeedsTwoFuncs(t *testing.T) {
	facts := []*MiningFacts{
		primitiveFact("SendEmail", "a.go", 10, []ParamInfo{{Name: "email", Type: "string"}}),
	}
	if cs := primitiveObsessionCandidates(facts); len(cs) != 0 {
		t.Errorf("expected 0 candidates for single function, got %d", len(cs))
	}
}

func TestPrimitiveObsessionIgnoresNonConceptParams(t *testing.T) {
	facts := []*MiningFacts{
		primitiveFact("F", "a.go", 10, []ParamInfo{{Name: "count", Type: "int"}}),
		primitiveFact("G", "b.go", 20, []ParamInfo{{Name: "total", Type: "int"}}),
	}
	if cs := primitiveObsessionCandidates(facts); len(cs) != 0 {
		t.Errorf("expected 0 candidates, got %d", len(cs))
	}
}

func TestPrimitiveObsessionIgnoresNonPrimitiveTypes(t *testing.T) {
	// Params only carries basic types by construction; a struct param never
	// reaches the detector. This guards the type filter if that changes.
	facts := []*MiningFacts{
		primitiveFact("F", "a.go", 10, []ParamInfo{{Name: "email", Type: "Email"}}),
		primitiveFact("G", "b.go", 20, []ParamInfo{{Name: "email", Type: "Email"}}),
	}
	if cs := primitiveObsessionCandidates(facts); len(cs) != 0 {
		t.Errorf("expected 0 candidates for named types, got %d", len(cs))
	}
}

func TestPrimitiveObsessionGroupsByConceptNotName(t *testing.T) {
	facts := []*MiningFacts{
		primitiveFact("A", "a.go", 10, []ParamInfo{{Name: "userEmail", Type: "string"}}),
		primitiveFact("B", "b.go", 20, []ParamInfo{{Name: "emailAddress", Type: "string"}}),
	}
	cs := primitiveObsessionCandidates(facts)
	if len(cs) != 1 {
		t.Fatalf("expected 1 candidate grouping userEmail+emailAddress, got %d", len(cs))
	}
}

func TestPrimitiveObsessionWiredIntoRun(t *testing.T) {
	facts := []*MiningFacts{
		primitiveFact("SendEmail", "a.go", 10, []ParamInfo{{Name: "email", Type: "string"}}),
		primitiveFact("ValidateEmail", "b.go", 20, []ParamInfo{{Name: "email", Type: "string"}}),
	}
	report := RunMining(facts, Options{})
	found := false
	for _, c := range report.Candidates {
		if c.Kind == PrimitiveObsession {
			found = true
		}
	}
	if !found {
		t.Error("primitive_obsession candidate missing from Run output")
	}
}

func TestPrimitiveConcept(t *testing.T) {
	cases := map[string]string{
		"email":        "email",
		"userEmail":    "email",
		"emailAddress": "email",
		"phoneNumber":  "phone",
		"userId":       "id",
		"user_id":      "id",
		"firstName":    "name",
		"count":        "",
		"subject":      "",
		"to":           "",
		"valid":        "", // "valid" is one word; no "id" word inside
		"identifier":   "", // one word; "id" is not a separate word
		"hidden":       "",
	}
	for name, want := range cases {
		if got := primitiveConcept(name); got != want {
			t.Errorf("primitiveConcept(%q) = %q, want %q", name, got, want)
		}
	}
}
