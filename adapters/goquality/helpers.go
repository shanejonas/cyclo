package goquality

import (
	"slices"
	"strings"

	"github.com/shanejonas/cyclo/domain/quality"
)

func attachHelpers(facts, available []quality.Function) []quality.Function {
	index := map[string]quality.Function{}
	for _, function := range available {
		index[quality.NormalizeCallee(function.Name)] = function
	}
	for position, function := range facts {
		facts[position].Helpers = reachableHelpers(function, index)
	}
	return facts
}

func reachableHelpers(function quality.Function, index map[string]quality.Function) []quality.Helper {
	seen := map[string]bool{quality.NormalizeCallee(function.Name): true}
	pending := slices.Clone(function.Calls)
	var helpers []quality.Helper
	for len(pending) > 0 {
		call := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if !call.Local || call.Dynamic {
			continue
		}
		name := quality.NormalizeCallee(call.Callee)
		target, found := index[name]
		if !found || seen[name] {
			continue
		}
		seen[name] = true
		helpers = append(helpers, quality.Helper{Name: target.Name, Mutations: target.Mutations, Calls: target.Calls, Effects: target.Effects})
		pending = append(pending, target.Calls...)
	}
	slices.SortFunc(helpers, func(a, b quality.Helper) int { return strings.Compare(a.Name, b.Name) })
	return helpers
}
