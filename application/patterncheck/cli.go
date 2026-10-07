// Package patterncheck is the IO boundary for cyclo's patterns miner: it
// proposes latent shared abstractions (interfaces, params structs, generics)
// found by structural mining. It is informational only: it always exits 0 on
// success and never feeds the quality gate. Pattern facts live apart from
// quality facts and are never passed to quality.Evaluate.
package patterncheck

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
	"github.com/shanejonas/cyclo/domain/patterns"
)

const usage = `Usage: cyclo patterns [OPTIONS] [DIRECTORIES OR GO FILES...]

Mine the codebase for latent shared abstractions: structurally parallel
functions that suggest interfaces, params structs, or generics, plus
inverted conditionals that want to be guard clauses.
Informational only: always exits 0, never a quality gate.
  --format FORMAT   text (default) or json
  --threshold N     minimum WL similarity (0-1000) for clustering [default 600]
  --cache           reuse WL refinements from .cyclo/patterns-cache.json,
                    speeding up repeat runs on large repos

Exit 0: always, on success (even with no candidates). Exit 2: extraction or
analysis failure.
`

const wlCachePath = ".cyclo/patterns-cache.json"

type options struct {
	format    string
	threshold uint
	useCache  bool
	paths     []string
}

// Run extracts PDGs for paths, mines them for abstraction candidates, and
// writes the report. It returns nil on success regardless of how many (or
// how few) candidates the miner finds; only extraction, analysis, or IO
// failures are errors.
func Run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	pdgs, err := gopatterns.Extract(ctx, "", opts.paths)
	if err != nil {
		return err
	}
	params := patterns.DefaultParams()
	params.ThresholdMilli = uint32(opts.threshold)
	cache, saveCache, err := openWlCache(opts.useCache)
	if err != nil {
		return err
	}
	defer saveCache()
	report := patterns.Run(toFacts(pdgs.Funcs), patterns.Options{
		Params:            params,
		AnemicModels:      pdgs.AnemicModels,
		MissingIdentities: pdgs.MissingIdentities,
		DomainServices:    pdgs.DomainServices,
		WlCache:           cache,
	})
	if opts.format == "json" {
		out, err := patterns.JSON(&report)
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, out+"\n")
		return err
	}
	_, err = io.WriteString(output, patterns.Text(&report)+"\n")
	return err
}

// openWlCache loads the WL cache when enabled, returning the cache and a
// deferred save. A missing cache file is fine (cold start); corrupt files
// error out. Save failures are best-effort and never fail the mining run.
func openWlCache(enabled bool) (cache *patterns.WlCache, save func(), err error) {
	save = func() {}
	if !enabled {
		return nil, save, nil
	}
	cache = patterns.NewWlCache()
	if err := cache.Load(wlCachePath); err != nil {
		return nil, save, err
	}
	return cache, func() { _ = cache.Save(wlCachePath) }, nil
}

// toFacts converts extracted PDGs to miner facts. The signature key derives
// from the parameter type classes, so functions with the same shape of
// signature block together in sigmine.
func toFacts(pdgs []gopatterns.FuncPdg) []*patterns.FuncFacts {
	facts := make([]*patterns.FuncFacts, 0, len(pdgs))
	for _, fp := range pdgs {
		fp := fp
		facts = append(facts, funcFactsOf(fp))
	}
	return facts
}

// funcFactsOf converts one extracted function to miner facts.
func funcFactsOf(fp gopatterns.FuncPdg) *patterns.FuncFacts {
	return &patterns.FuncFacts{
		ID:                fp.Name,
		Name:              fp.Name,
		Path:              fp.Path,
		Line:              fp.Line,
		EndLine:           fp.EndLine,
		Pdg:               &fp.Pdg,
		SigKey:            gopatterns.SigKeyOf(fp),
		SelfTy:            fp.SelfTy,
		GuardClauses:      guardHits(fp.GuardClauses),
		EnumDispatches:    dispatchHits(fp.EnumDispatches),
		TypeSwitches:      typeSwitchHits(fp.TypeSwitches),
		EntityIdentities:  entityHits(fp.EntityIdentities),
		MutableIdentities: mutableHits(fp.MutableIdentities),
		AggregateMods:     fp.AggregateMods,
		DbCalls:           fp.DbCalls,
		FactoryLits:       fp.FactoryLits,
		SpecRules:         fp.SpecRules,
		Params:            fp.Params,
	}
}

func guardHits(hits []gopatterns.GuardClauseHit) []patterns.GuardClauseHit {
	var out []patterns.GuardClauseHit
	for _, g := range hits {
		out = append(out, patterns.GuardClauseHit{Line: g.Line, BodyStmts: g.BodyStmts})
	}
	return out
}

func dispatchHits(hits []gopatterns.EnumDispatchHit) []patterns.EnumDispatchHit {
	var out []patterns.EnumDispatchHit
	for _, d := range hits {
		out = append(out, patterns.EnumDispatchHit{Line: d.Line, NumCases: d.NumCases})
	}
	return out
}

func typeSwitchHits(hits []gopatterns.TypeSwitchHit) []patterns.TypeSwitchHit {
	var out []patterns.TypeSwitchHit
	for _, t := range hits {
		out = append(out, patterns.TypeSwitchHit{
			Line: t.Line, Bound: t.Bound, Expr: t.Expr,
			Method: t.Method, Types: t.Types, Args: t.Args,
		})
	}
	return out
}

func entityHits(hits []patterns.EntityIdentityHit) []patterns.EntityIdentityHit {
	var out []patterns.EntityIdentityHit
	for _, e := range hits {
		out = append(out, patterns.EntityIdentityHit{
			Line: e.Line, TypeName: e.TypeName, IDField: e.IDField,
			Fields: e.Fields, Left: e.Left, Right: e.Right,
		})
	}
	return out
}

func mutableHits(hits []patterns.MutableIdentityHit) []patterns.MutableIdentityHit {
	var out []patterns.MutableIdentityHit
	for _, m := range hits {
		out = append(out, patterns.MutableIdentityHit{
			Line: m.Line, Field: m.Field, FuncName: m.FuncName,
		})
	}
	return out
}
func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("patterns", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "text", "output format")
	threshold := flags.Uint("threshold", 600, "minimum WL similarity (0-1000)")
	useCache := flags.Bool("cache", false, "reuse WL refinements from .cyclo/patterns-cache.json")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	result := options{format: *format, threshold: *threshold, useCache: *useCache, paths: flags.Args()}
	return result, result.validate()
}

func (opts options) validate() error {
	if !slices.Contains([]string{"text", "json"}, opts.format) {
		return fmt.Errorf("format must be text or json")
	}
	if opts.threshold > 1000 {
		return fmt.Errorf("threshold must be between 0 and 1000")
	}
	return nil
}
