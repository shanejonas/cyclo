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
	MinStatements     int     `toml:"min_statements"`
	CountSelf         bool    `toml:"count_self"`
	Granularity       string  `toml:"granularity"`
	Weights           Weights `toml:"weights"`
	// Prefixes are appended to defaults by the config adapter. Later entries
	// win equal-length ties, so projects can override classifications.
	Prefixes []Prefix `toml:"prefixes"`
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

func DefaultConfig() Config {
	return Config{
		FnLength: Rule{true, 50}, FnParams: Rule{true, 4},
		MutationPerTarget: Rule{true, 3}, MutatedTargets: Rule{true, 3},
		SideEffectDensity: Rule{true, 500}, MinStatements: 3, Granularity: "root",
		Weights: Weights{1, 3, 3, 2, 4, 1, 1, 0, 1}, Prefixes: defaultPrefixes(),
	}
}

func (c Config) Validate() error {
	if !slices.Contains([]string{"root", "field"}, c.Granularity) {
		return fmt.Errorf("granularity must be root or field")
	}
	if !within(int64(c.MinStatements), 1_000_000) {
		return fmt.Errorf("min_statements must be between 0 and 1000000")
	}
	for _, validate := range []func() error{c.validateLimits, c.validateWeights, c.validatePrefixes} {
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
		purePrefixes(),
		[]Prefix{
			{"net.Dial", Network}, {"net.Listen", Network}, {"net.Lookup", Network},
			{"net.Conn.", Network}, {"net.TCPConn.", Network}, {"net.UDPConn.", Network},
			{"net.UnixConn.", Network}, {"net.Listener.Accept", Network}, {"net.Listener.Close", Network},
			{"net/http.Get", Network}, {"net/http.Head", Network}, {"net/http.Post", Network},
			{"net/http.ListenAndServe", Network}, {"net/http.Serve", Network},
			{"net/http.Client.Do", Network}, {"net/http.Client.Get", Network}, {"net/http.Client.Head", Network}, {"net/http.Client.Post", Network},
			{"net/http.Server.Serve", Network}, {"net/http.Server.ListenAndServe", Network}, {"net/http.Transport.RoundTrip", Network},
			{"net/http.NewRequest", None}, {"net/url.Parse", None},
			{"io.Copy", IO}, {"io.ReadAll", IO}, {"io.ReadFull", IO}, {"io.ReadAtLeast", IO}, {"io.WriteString", IO},
			{"io.Reader.Read", IO}, {"io.Writer.Write", IO}, {"io.Closer.Close", IO},
			{"io/fs.ReadFile", IO}, {"io/fs.ReadDir", IO}, {"io/fs.Stat", IO}, {"io/fs.Glob", IO}, {"io/fs.WalkDir", IO},
			{"time.Now", Time}, {"time.Since", Time}, {"time.Until", Time}, {"time.Sleep", Time},
			{"time.NewTimer", Time}, {"time.NewTicker", Time}, {"time.After", Time},
			{"math/rand.", Random}, {"math/rand/", Random}, {"crypto/rand.", Random},
			{"unsafe.", Unsafe},
		},
		surgicalPrefixes(),
		[]Prefix{
			{"os/exec.Command", None},
			{"error.Error", None},
		},
	)
}

// surgicalPrefixes classifies packages that mix pure and effectful calls
// one name at a time.
func surgicalPrefixes() []Prefix {
	return slices.Concat(
		withPathKind("os.", IO, []string{"Open", "Create", "ReadFile", "WriteFile", "ReadDir", "Mkdir", "Remove", "Rename", "Stat", "Lstat", "Chmod", "Chown", "Chtimes", "Truncate", "Link", "Symlink", "Readlink", "Getenv", "LookupEnv", "Environ", "Setenv", "Unsetenv", "Clearenv", "Getwd", "Chdir", "Exit", "StartProcess", "FindProcess"}),
		withPathKind("os.File.", IO, []string{"Read", "Write", "Close", "Seek", "Sync", "Stat", "Truncate", "Chmod", "Chown", "Readdir", "ReadDir"}),
		withPathKind("os/exec.Cmd.", IO, []string{"Run", "Start", "Wait", "Output", "CombinedOutput"}),
		withPathKind("fmt.", IO, []string{"Print", "Printf", "Println", "Fprint", "Fprintf", "Fprintln", "Scan", "Scanf", "Scanln", "Fscan", "Fscanf", "Fscanln"}),
		withPathKind("fmt.", None, []string{"Sprintf", "Sprint", "Sprintln", "Errorf"}),
		// Pure path manipulation. Abs, EvalSymlinks and Glob stay
		// unclassified: they consult the working directory or the disk.
		withPathKind("path/filepath.", None, []string{"Base", "Dir", "Ext", "Join", "Split", "Clean", "IsAbs", "Rel", "Match", "SplitList", "ToSlash", "FromSlash", "VolumeName"}),
	)
}

func withPathKind(path string, kind Kind, names []string) []Prefix {
	result := make([]Prefix, len(names))
	for index, name := range names {
		result[index] = Prefix{path + name, kind}
	}
	return result
}

// purePrefixes lists calls that are pure by construction: immutable data
// readers and string builders whose calls used to fall through to Unknown
// and inflate side-effect density. Kept narrow where a package mixes pure
// and effectful calls (os, fmt, path/filepath stay surgical in
// defaultPrefixes, as do the go/types entry points); more specific prefixes
// still win by longest match, so math/rand stays Random and fmt.Print*
// stays IO.
func purePrefixes() []Prefix {
	paths := []string{
		"strings.", "strconv.", "unicode.", "unicode/utf8.",
		"cmp.", "slices.", "maps.", "math.", "sort.", "errors.", "path.",
		"charm.land/lipgloss", "github.com/charmbracelet/lipgloss",
		"github.com/charmbracelet/x/ansi.",
		// go/types values are immutable once checking completes; these are
		// pure readers over checked type information. Config.Check and the
		// importer stay unclassified on purpose.
		"go/types.Object.", "go/types.Type.", "go/types.Info.",
		"go/types.Selection.", "go/types.Union.", "go/types.Tuple.",
		"go/types.Signature.", "go/types.Array.", "go/types.Slice.",
		"go/types.Map.", "go/types.Chan.", "go/types.Pointer.",
		"go/types.Basic.", "go/types.Struct.", "go/types.Interface.",
	}
	result := make([]Prefix, len(paths))
	for index, path := range paths {
		result[index] = Prefix{path, None}
	}
	return result
}
