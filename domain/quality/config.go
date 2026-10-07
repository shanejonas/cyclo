package quality

import (
	"fmt"
	"slices"
)

type Rule struct {
	Enabled bool  `toml:"enabled"`
	Max     int64 `toml:"max"`
}

type Config struct {
	FnLength          Rule    `toml:"fn_length"`
	FnParams          Rule    `toml:"fn_params"`
	MutationPerTarget Rule    `toml:"mutation_per_target"`
	MutatedTargets    Rule    `toml:"mutated_targets"`
	SideEffectDensity Rule    `toml:"side_effect_density"`
	Aggregate         Rule    `toml:"aggregate"`
	Repository        Rule    `toml:"repository"`
	MutableIdentity   Rule    `toml:"mutable_identity"`
	MinStatements     int     `toml:"min_statements"`
	CountSelf         bool    `toml:"count_self"`
	Granularity       string  `toml:"granularity"`
	Weights           Weights `toml:"weights"`
	// Prefixes are appended to defaults by the config adapter. Later entries
	// win equal-length ties, so projects can override classifications.
	Prefixes []Prefix `toml:"prefixes"`
	// Grouping controls fix-group formation: which violations must be fixed
	// together. A violation's footprint is its function plus, for caller
	// rules, its callers up to CallerHops, because a signature fix edits
	// every call site.
	Grouping Grouping `toml:"grouping"`
}

type Weights struct {
	Mutation int64 `toml:"mutation"`
	IO       int64 `toml:"io"`
	Network  int64 `toml:"network"`
	Global   int64 `toml:"global"`
	Unsafe   int64 `toml:"unsafe"`
	Time     int64 `toml:"time"`
	Random   int64 `toml:"random"`
	Panic    int64 `toml:"panic"`
	Unknown  int64 `toml:"unknown"`
}

type Prefix struct {
	Path string `toml:"path"`
	Kind Kind   `toml:"kind"`
}

// Grouping decides which violations must be fixed together.
type Grouping struct {
	CallerRules []string `toml:"caller_rules"`
	CallerHops  int      `toml:"caller_hops"`
}

func DefaultConfig() Config {
	return Config{
		FnLength: Rule{true, 50}, FnParams: Rule{true, 4},
		MutationPerTarget: Rule{true, 3}, MutatedTargets: Rule{true, 3},
		SideEffectDensity: Rule{true, 500}, MinStatements: 3, Granularity: "root",
		Aggregate: Rule{true, 1}, Repository: Rule{true, 0}, MutableIdentity: Rule{true, 0},
		Weights: Weights{1, 3, 3, 2, 4, 1, 1, 0, 1}, Prefixes: defaultPrefixes(),
		Grouping: Grouping{CallerRules: []string{"fn_params"}, CallerHops: 1},
	}
}

func (c Config) Validate() error {
	if !slices.Contains([]string{"root", "field"}, c.Granularity) {
		return fmt.Errorf("granularity must be root or field")
	}
	if !within(int64(c.MinStatements), 1_000_000) {
		return fmt.Errorf("min_statements must be between 0 and 1000000")
	}
	for _, validate := range []func() error{c.validateLimits, c.validateWeights, c.validatePrefixes, c.validateGrouping} {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func within(value, maximum int64) bool { return value >= 0 && value <= maximum }

func (c Config) validateLimits() error {
	for _, rule := range []Rule{c.FnLength, c.FnParams, c.MutationPerTarget, c.MutatedTargets, c.SideEffectDensity} {
		if !within(rule.Max, 1_000_000_000) {
			return fmt.Errorf("rule limits must be between 0 and 1000000000")
		}
	}
	return nil
}

func (c Config) validateWeights() error {
	for _, kind := range kinds {
		if !within(c.Weights.value(kind), 1_000_000) {
			return fmt.Errorf("weight %s must be between 0 and 1000000", kind)
		}
	}
	return nil
}

func (c Config) validatePrefixes() error {
	for _, prefix := range c.Prefixes {
		if prefix.Path == "" || !validKind(prefix.Kind, true) {
			return fmt.Errorf("invalid classification prefix %q (%s)", prefix.Path, prefix.Kind)
		}
	}
	return nil
}

// validateGrouping rejects unknown caller rules so a typo never silently
// changes which violations are fixed together.
func (c Config) validateGrouping() error {
	for _, rule := range c.Grouping.CallerRules {
		if !slices.Contains(ruleIDs, rule) {
			return fmt.Errorf("unknown grouping caller rule %q", rule)
		}
	}
	if !within(int64(c.Grouping.CallerHops), 1_000_000) {
		return fmt.Errorf("grouping caller_hops must be between 0 and 1000000")
	}
	return nil
}

func validKind(kind Kind, allowNone bool) bool {
	if allowNone && kind == None {
		return true
	}
	for _, known := range kinds {
		if known == kind {
			return true
		}
	}
	return false
}

func (w Weights) value(kind Kind) int64 {
	switch kind {
	case MutationEffect:
		return w.Mutation
	case IO:
		return w.IO
	case Network:
		return w.Network
	case Global:
		return w.Global
	case Unsafe:
		return w.Unsafe
	default:
		return w.environmentWeight(kind)
	}
}

func (w Weights) environmentWeight(kind Kind) int64 {
	switch kind {
	case Time:
		return w.Time
	case Random:
		return w.Random
	case Panic:
		return w.Panic
	default:
		return w.Unknown
	}
}

func defaultPrefixes() []Prefix {
	return slices.Concat(
		stdlibPrefixes(),
		thirdPartyPrefixes(),
	)
}

// thirdPartyPrefixes classifies non-stdlib packages the tool knows are
// pure. Unknown third-party calls stay Unknown on purpose: after the
// stdlib table, unknown means "a call cyclo cannot see into".
func thirdPartyPrefixes() []Prefix {
	return []Prefix{
		{Path: "charm.land/lipgloss", Kind: None},
		{Path: "github.com/charmbracelet/lipgloss", Kind: None},
		{Path: "github.com/charmbracelet/x/ansi.", Kind: None},
	}
}
