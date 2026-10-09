package quality

import (
	"slices"
	"strings"
)

var ruleIDs = []string{"fn_length", "fn_params", "mutation_per_target", "mutated_targets", "side_effect_density", "invalid_suppression", "aggregate", "repository", "mutable_identity", "nilerr", "forcetypeassert", "typednil"}

func suppression(comment string) ([]string, string) {
	comment = strings.TrimSpace(comment)
	// Support both // cyclo-allow(rules): reason and //lint:ignore rules reason.
	// The lint:ignore form is the standard Go ecosystem convention.
	if strings.HasPrefix(comment, "//lint:ignore") {
		return parseLintIgnore(comment)
	}
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

// parseLintIgnore parses `//lint:ignore rule1, rule2 reason text`.
func parseLintIgnore(comment string) ([]string, string) {
	rest := strings.TrimSpace(strings.TrimPrefix(comment, "//lint:ignore"))
	parts := strings.Fields(rest)
	rules, reasonParts := splitRulesReason(parts)
	if len(rules) == 0 {
		return nil, "invalid suppression: expected rule IDs"
	}
	if len(reasonParts) == 0 {
		return nil, "invalid suppression: reason is required"
	}
	return validateSuppressedRules(strings.Join(rules, ","))
}

// splitRulesReason splits fields into rule IDs (leading) and reason (rest).
func splitRulesReason(parts []string) ([]string, []string) {
	var rules []string
	for i, p := range parts {
		clean := strings.TrimSuffix(p, ",")
		if !isRuleID(clean) {
			return rules, parts[i:]
		}
		rules = append(rules, clean)
	}
	return rules, nil
}

// isRuleID reports whether s looks like a rule ID (lowercase, digits, underscore).
func isRuleID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isRuleChar(r) {
			return false
		}
	}
	return true
}

// isRuleChar reports whether r is valid in a rule ID.
func isRuleChar(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_'
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
