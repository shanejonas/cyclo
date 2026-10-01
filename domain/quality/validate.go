package quality

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

func validateFact(f Function) error {
	for _, validate := range []func(Function) error{validateLocation, validateCounts, validateMutations, validateEffects, validateCalls, validateHelpers} {
		if err := validate(f); err != nil {
			return err
		}
	}
	return nil
}

func portablePath(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && !path.IsAbs(value) && path.Clean(value) == value
}

func validateLocation(f Function) error {
	if !portablePath(f.Path) || f.Path == ".." || strings.HasPrefix(f.Path, "../") {
		return fmt.Errorf("fact path must be normalized and relative: %q", f.Path)
	}
	if !validPosition(f.Location) {
		return fmt.Errorf("invalid function location in %s", f.Path)
	}
	return nil
}

func validPosition(loc Location) bool { return loc.Line >= 1 && loc.Column >= 1 && loc.Name != "" }

func validateCounts(f Function) error {
	for _, value := range []int{f.Params, f.Statements, f.CodeLines, len(f.Mutations), len(f.Calls), len(f.Effects)} {
		if !within(int64(value), 1_000_000) {
			return fmt.Errorf("invalid fact count in %s:%d", f.Path, f.Line)
		}
	}
	return nil
}

func validateMutations(f Function) error {
	for _, mutation := range f.Mutations {
		if mutation.Root == "" || mutation.Line < 1 {
			return fmt.Errorf("invalid mutation in %s:%d", f.Path, f.Line)
		}
		if !slices.Contains([]Provenance{Local, External, Unknown}, mutation.Provenance) {
			return fmt.Errorf("invalid mutation provenance %q", mutation.Provenance)
		}
	}
	return nil
}

func validateEffects(f Function) error {
	for _, effect := range f.Effects {
		if !validKind(effect.Kind, false) || effect.Line < 1 {
			return fmt.Errorf("invalid intrinsic effect in %s:%d", f.Path, f.Line)
		}
	}
	return nil
}

func validateCalls(f Function) error {
	for _, call := range f.Calls {
		if call.Callee == "" || call.Line < 1 {
			return fmt.Errorf("invalid call in %s:%d", f.Path, f.Line)
		}
	}
	return nil
}
