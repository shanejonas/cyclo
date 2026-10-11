package patterns

import "testing"

func TestAggregateCandidates(t *testing.T) {
	facts := []*MiningFacts{
		{
			ID:   "pkg.checkout",
			Name: "checkout",
			Path: "order.go",
			Line: 10,
			AggregateMods: []AggregateModHit{
				{TypeName: "Order"},
				{TypeName: "OrderLine"},
			},
		},
		{
			ID:   "pkg.refund",
			Name: "refund",
			Path: "order.go",
			Line: 50,
			AggregateMods: []AggregateModHit{
				{TypeName: "Order"},
				{TypeName: "OrderLine"},
			},
		},
		{
			ID:   "pkg.single",
			Name: "single",
			Path: "other.go",
			Line: 10,
			AggregateMods: []AggregateModHit{
				{TypeName: "User"},
			},
		},
	}
	cands := aggregateCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want 1", len(cands))
	}
	c := cands[0]
	if c.Kind != Aggregate {
		t.Errorf("kind = %q, want aggregate", c.Kind)
	}
	if c.FixSpec != nil {
		t.Error("FixSpec should be nil (detection-only)")
	}
	if c.Breakdown.Support != 2 {
		t.Errorf("support = %d, want 2", c.Breakdown.Support)
	}
}

func TestAggregateCandidatesSingleTypeSkipped(t *testing.T) {
	facts := []*MiningFacts{{
		ID:            "pkg.f",
		Name:          "f",
		Path:          "a.go",
		Line:          1,
		AggregateMods: []AggregateModHit{{TypeName: "Order"}},
	}}
	if cands := aggregateCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0", len(cands))
	}
}

func TestRepositoryCandidates(t *testing.T) {
	facts := []*MiningFacts{{
		ID:   "pkg.checkout",
		Name: "checkout",
		Path: "order.go",
		Line: 10,
		DbCalls: []DbCallHit{
			{Line: 12, Call: "db.Query"},
			{Line: 15, Call: "db.Exec"},
		},
	}}
	cands := repositoryCandidates(facts)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates, want 1", len(cands))
	}
	c := cands[0]
	if c.Kind != Repository {
		t.Errorf("kind = %q, want repository", c.Kind)
	}
	if c.FixSpec != nil {
		t.Error("FixSpec should be nil (detection-only)")
	}
}

func TestRepositoryCandidatesNoneSkipped(t *testing.T) {
	facts := []*MiningFacts{{
		ID:   "pkg.f",
		Name: "f",
		Path: "a.go",
		Line: 1,
	}}
	if cands := repositoryCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0", len(cands))
	}
}

func TestFactoryCandidatesOverFieldCapSkipped(t *testing.T) {
	// 6 fields with logic in 2 functions would previously qualify, but the
	// factory would need 6 params, violating the fn_params quality gate.
	// The pattern stays dormant until param bundling is designed.
	facts := []*MiningFacts{
		{
			ID:   "pkg.a",
			Name: "a",
			Path: "x.go",
			Line: 10,
			FactoryLits: []FactoryHit{
				{Line: 12, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: true},
			},
		},
		{
			ID:   "pkg.b",
			Name: "b",
			Path: "y.go",
			Line: 20,
			FactoryLits: []FactoryHit{
				{Line: 22, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: true},
			},
		},
	}
	if cands := factoryCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0 (over fn_params cap)", len(cands))
	}
}

func TestFactoryCandidatesSingleFunctionSkipped(t *testing.T) {
	facts := []*MiningFacts{{
		ID:   "pkg.a",
		Name: "a",
		Path: "x.go",
		Line: 10,
		FactoryLits: []FactoryHit{
			{Line: 12, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: true},
		},
	}}
	if cands := factoryCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0", len(cands))
	}
}

func TestFactoryCandidatesNoLogicSkipped(t *testing.T) {
	// Plain field assignment in 2+ functions: no construction logic,
	// so no factory. This is the "just field shuffling" case Shane rejected.
	facts := []*MiningFacts{
		{
			ID:   "pkg.a",
			Name: "a",
			Path: "x.go",
			Line: 10,
			FactoryLits: []FactoryHit{
				{Line: 12, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: false},
			},
		},
		{
			ID:   "pkg.b",
			Name: "b",
			Path: "y.go",
			Line: 20,
			FactoryLits: []FactoryHit{
				{Line: 22, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: false},
			},
		},
	}
	if cands := factoryCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0 (no construction logic)", len(cands))
	}
}

func TestFactoryCandidatesMixedLogicSkipped(t *testing.T) {
	// Only one site has logic: the "pattern" isn't scattered, so no factory.
	// Extracting would impose site A's validation on site B (behavior change).
	facts := []*MiningFacts{
		{
			ID:   "pkg.a",
			Name: "a",
			Path: "x.go",
			Line: 10,
			FactoryLits: []FactoryHit{
				{Line: 12, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: true},
			},
		},
		{
			ID:   "pkg.b",
			Name: "b",
			Path: "y.go",
			Line: 20,
			FactoryLits: []FactoryHit{
				{Line: 22, TypeName: "Config", NumFields: 6, DeclFile: "config.go", HasLogic: false},
			},
		},
	}
	if cands := factoryCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0 (only one site has logic)", len(cands))
	}
}

func TestFactoryCandidatesSmallLiteralSkipped(t *testing.T) {
	facts := []*MiningFacts{
		{
			ID:   "pkg.a",
			Name: "a",
			Path: "x.go",
			Line: 10,
			FactoryLits: []FactoryHit{
				{Line: 12, TypeName: "Config", NumFields: 3, DeclFile: "config.go", HasLogic: true},
			},
		},
		{
			ID:   "pkg.b",
			Name: "b",
			Path: "y.go",
			Line: 20,
			FactoryLits: []FactoryHit{
				{Line: 22, TypeName: "Config", NumFields: 3, DeclFile: "config.go", HasLogic: true},
			},
		},
	}
	if cands := factoryCandidates(facts); len(cands) != 0 {
		t.Errorf("got %d candidates, want 0 (under field threshold)", len(cands))
	}
}
