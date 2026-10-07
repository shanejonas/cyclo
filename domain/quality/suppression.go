package quality

import (
	"slices"
	"strings"
)

var ruleIDs = []string{"fn_length", "fn_params", "mutation_per_target", "mutated_targets", "side_effect_density", "invalid_suppression", "aggregate", "repository", "mutable_identity"}

func suppression(comment string) ([]string, string) {
	comment = strings.TrimSpace(comment)
	if !strings.Contains(comment, "cyclo-allow") {
		return nil, ""
	}
	if !strings.HasPrefix(comment, "// cyclo-allow(") {
		return nil, "invalid suppression: expected // cyclo-allow(rules): reason"
	}
	rules, reason, ok := strings.Cut(strings.TrimPrefix(comment, "// cyclo-allow("), "):")
	if !ok || strings.TrimSpace(reason) == "" {
		return nil, "invalid suppression: reason is required"
	}
	return validateSuppressedRules(rules)
}

func validateSuppressedRules(rules string) ([]string, string) {
	allowed := []string{}
	for _, id := range strings.Split(rules, ",") {
		id = strings.TrimSpace(id)
		if !slices.Contains(ruleIDs, id) {
			return nil, "invalid suppression: unknown rule id"
		}
		if id == "invalid_suppression" {
			return nil, "invalid suppression: invalid_suppression cannot be suppressed"
		}
		allowed = append(allowed, id)
	}
	return allowed, ""
}
