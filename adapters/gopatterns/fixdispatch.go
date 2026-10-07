package gopatterns

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// fixApplier is a function that applies a FixSpec.
type fixApplier func(*patterns.FixSpec, []byte) ([]byte, error)

// appliers maps each fixable kind to its applier.
var appliers = map[patterns.CandidateKind]fixApplier{
	patterns.GuardClause:   applyGuardFix,
	patterns.ValueObject:   applyValueObjectFix,
	patterns.Parameterize:  applyParameterizeFix,
	patterns.TraitMethod:    applyInterfaceFix,
	patterns.CapabilitySet:  applyInterfaceFix,
	patterns.EnumDispatch:   applyEnumDispatchFix,
	patterns.GenericFn:      applyGenericFnFix,
}

// ApplyFix applies the transform for a candidate's FixSpec to src.
// It is a pure AST transform: no re-detection, the miner already did the
// analysis. Each kind has its own applier.
func ApplyFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	if spec == nil {
		return nil, fmt.Errorf("nil FixSpec")
	}
	apply, ok := appliers[spec.Kind]
	if !ok {
		return nil, fmt.Errorf("no fixer for kind %q", spec.Kind)
	}
	return apply(spec, src)
}

// parseSpec parses src and returns the file set and AST.
func parseSpec(spec *patterns.FixSpec, src []byte) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, spec.File, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", spec.File, err)
	}
	return fset, f, nil
}
