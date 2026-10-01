package quality

import "fmt"

func validateHelpers(f Function) error {
	if !within(int64(len(f.Helpers)), 1_000_000) {
		return fmt.Errorf("invalid helper count in %s", f.Name)
	}
	seen := map[string]bool{NormalizeCallee(f.Name): true}
	for _, helper := range f.Helpers {
		name := NormalizeCallee(helper.Name)
		if name == "" || seen[name] {
			return fmt.Errorf("invalid or duplicate helper name %q", helper.Name)
		}
		seen[name] = true
		body := Function{Location: f.Location, Mutations: helper.Mutations, Effects: helper.Effects, Calls: helper.Calls}
		if err := validateHelperBody(body); err != nil {
			return err
		}
	}
	return nil
}

func validateHelperBody(f Function) error {
	for _, validate := range []func(Function) error{validateCounts, validateMutations, validateEffects, validateCalls} {
		if err := validate(f); err != nil {
			return err
		}
	}
	return nil
}
