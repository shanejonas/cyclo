// Package gopatterns extracts rstyle-schema program-dependence graphs from Go
// functions. It is the Phase 1 foundation of the patterns-miner port: the PDG
// extractor whose output feeds the (Phase 2) normalize → WL → cluster → align
// → candidates pipeline.
//
// Unlike adapters/goquality (quality policy facts), this package produces
// structural dependence graphs for abstraction mining. It is intentionally
// decoupled from goquality: different schema, different selection needs, and
// Phase 1 must not touch existing quality paths.
package gopatterns

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
	"golang.org/x/tools/go/packages"
)

// FuncPdg is the dependence graph of one function.
type FuncPdg struct {
	Name string
	Path string
	Line int
	// EndLine is the function's closing line.
	EndLine int
	Pdg     patterns.Pdg
	// GuardClauses are inverted conditionals in this function that want
	// to be guard clauses (AST-level finding, not PDG-derived).
	GuardClauses []GuardClauseHit
	// EnumDispatches are enum-value switches that want to be dispatch
	// tables (AST-level finding, not PDG-derived).
	EnumDispatches []EnumDispatchHit
	// TypeSwitches are type switches in this function whose arms all call
	// the same method on the case-bound value (AST-level finding).
	TypeSwitches []TypeSwitchHit
	// Params are the function's primitive-typed parameters. Used for
	// data-clump detection (value object proposals).
	Params []patterns.ParamInfo
}

// Extraction is the full result of package analysis: per-function PDGs
// plus package-level type findings (anemic models).
type Extraction struct {
	Funcs []FuncPdg
	// AnemicModels are exported methodless structs with 3+ functions
	// operating on their fields (DDD anemic domain model detection).
	AnemicModels []patterns.AnemicModelHit
}

// Extract loads the packages enclosing paths and returns PDGs per
// function plus anemic-model hits. Paths are directories (".", "./...")
// or .go files, resolved under root.
func Extract(ctx context.Context, root string, paths []string) (*Extraction, error) {
	abs, err := absRoot(root)
	if err != nil {
		return nil, err
	}
	query, err := patternsQuery(abs, paths)
	if err != nil {
		return nil, err
	}
	pkgs, err := loadPatternPackages(ctx, abs, query)
	if err != nil {
		return nil, err
	}
	out := &Extraction{}
	for _, pkg := range pkgs {
		out.Funcs = append(out.Funcs, packagePdgs(pkg, abs)...)
		out.AnemicModels = append(out.AnemicModels, findAnemicModels(pkg, abs)...)
	}
	return out, nil
}

func absRoot(root string) (string, error) {
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return cwd, nil
	}
	return filepath.Abs(root)
}

func loadPatternPackages(ctx context.Context, dir string, query []string) ([]*packages.Package, error) {
	config := &packages.Config{Context: ctx, Dir: dir, Mode: packages.LoadSyntax}
	pkgs, err := packages.Load(config, query...)
	if err != nil {
		return nil, fmt.Errorf("load Go packages: %w", err)
	}
	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, strings.ReplaceAll(e.Error(), dir+string(filepath.Separator), ""))
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("Go analysis fails:\n%s", strings.Join(errs, "\n"))
	}
	return pkgs, nil
}

func packagePdgs(pkg *packages.Package, root string) []FuncPdg {
	out := []FuncPdg{}
	for _, file := range pkg.Syntax {
		filename := pkg.Fset.PositionFor(file.Pos(), false).Filename
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			out = append(out, extractFunc(pkg, fn, filepath.ToSlash(rel)))
		}
	}
	return out
}

func patternsQuery(root string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	query := []string{}
	for _, p := range paths {
		q, err := patternInput(root, p)
		if err != nil {
			return nil, err
		}
		query = append(query, q)
	}
	return query, nil
}

func patternInput(root, input string) (string, error) {
	if strings.Contains(input, "...") {
		return "", fmt.Errorf("use directory or .go file paths, not package patterns: %q", input)
	}
	abs := input
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, input)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("inspect input %q: %w", input, err)
	}
	if info.IsDir() {
		return filepath.Join(abs, "..."), nil
	}
	if filepath.Ext(abs) != ".go" {
		return "", fmt.Errorf("input must be a directory or Go file: %q", input)
	}
	return "file=" + abs, nil
}

func extractFunc(pkg *packages.Package, fn *ast.FuncDecl, path string) FuncPdg {
	name := fn.Name.Name
	if object, ok := pkg.TypesInfo.Defs[fn.Name].(*types.Func); ok {
		name = patterns.FuncID(object)
	}
	pos := pkg.Fset.PositionFor(fn.Pos(), false)
	end := pkg.Fset.PositionFor(fn.End(), false)
	b := &builder{
		info:  pkg.TypesInfo,
		fset:  pkg.Fset,
		binds: map[types.Object]int{},
	}
	b.params(fn)
	b.stmt(fn.Body)
	return FuncPdg{
		Name:         name,
		Path:         path,
		Line:         pos.Line,
		EndLine:      end.Line,
		Pdg:          patterns.Pdg{Nodes: b.nodes, Edges: b.edges},
		GuardClauses:   findGuardClauses(fn, pkg.Fset),
		EnumDispatches: findEnumDispatches(fn, pkg.Fset),
		TypeSwitches:   findTypeSwitches(fn, pkg.Fset),
		Params:       primitiveParams(fn, pkg.TypesInfo),
	}
}

// primitiveParams returns the function's basic-typed parameters as
// (name, type) pairs. Named types are excluded: they are already domain
// types and need no value-object proposal.
func primitiveParams(fn *ast.FuncDecl, info *types.Info) []patterns.ParamInfo {
	if fn.Type.Params == nil {
		return nil
	}
	var out []patterns.ParamInfo
	for _, field := range fn.Type.Params.List {
		basic, ok := info.TypeOf(field.Type).(*types.Basic)
		if !ok {
			continue
		}
		for _, name := range field.Names {
			out = append(out, patterns.ParamInfo{Name: name.Name, Type: basic.Name()})
		}
	}
	return out
}

// ctrlFrame is one level of the control stack: nodes lowered inside it gain a
// Ctrl edge from owner with this arm index. This is lexical nesting (like
// rstyle's pdg.rs), not full post-dominator control dependence.
type ctrlFrame struct {
	owner int
	arm   int
}

// loopCtx records an enclosing iteration for the loop-element
// canonicalization: coll[counter] resolves to the Iterate node, so
// `for i ... { f(names[i]) }` and `for _, n := range names { f(n) }`
// produce the same shape.
type loopCtx struct {
	counter types.Object // loop counter variable (nil for range loops)
	coll    int          // node index of the iterated collection
	iterate int          // Iterate node index
}

// builder lowers one function body to a PDG. Bindings are flow-insensitive
// (first binding wins, like rstyle): a variable use resolves to its
// declaration/parameter/loop-variable node.
type builder struct {
	info  *types.Info
	fset  *token.FileSet
	nodes []patterns.PdgNode
	edges []patterns.PdgEdge
	binds map[types.Object]int
	ctrl  []ctrlFrame
	loops []loopCtx
}

// none is returned by expr when an expression produces no PDG node
// (a variable use resolved through binds, blank, or unresolvable).
const none = -1

func (b *builder) line(pos token.Pos) int {
	return b.fset.PositionFor(pos, false).Line
}

// node appends a node, wiring a Ctrl edge from the enclosing control owner.
func (b *builder) node(n patterns.PdgNode) int {
	index := len(b.nodes)
	b.nodes = append(b.nodes, n)
	if len(b.ctrl) > 0 {
		top := b.ctrl[len(b.ctrl)-1]
		b.edges = append(b.edges, patterns.PdgEdge{From: top.owner, To: index, Kind: patterns.Ctrl, ArgPos: top.arm})
	}
	return index
}

func (b *builder) data(from, to, pos int) {
	if from == none {
		return
	}
	b.edges = append(b.edges, patterns.PdgEdge{From: from, To: to, Kind: patterns.Data, ArgPos: pos})
}

func (b *builder) under(owner, arm int, f func()) {
	b.ctrl = append(b.ctrl, ctrlFrame{owner, arm})
	f()
	b.ctrl = b.ctrl[:len(b.ctrl)-1]
}

// bind records the node a variable resolves to. First binding wins:
// reassignments do not move the binding (flow-insensitive, like rstyle).
func (b *builder) bind(obj types.Object, node int) {
	if obj == nil || node == none {
		return
	}
	if _, ok := b.binds[obj]; !ok {
		b.binds[obj] = node
	}
}

func (b *builder) lookup(obj types.Object) (int, bool) {
	n, ok := b.binds[obj]
	return n, ok
}

func (b *builder) params(fn *ast.FuncDecl) {
	add := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			typ := patterns.TypeClass(b.info.TypeOf(field.Type))
			for _, name := range field.Names {
				obj := b.info.Defs[name]
				n := b.node(patterns.PdgNode{Kind: patterns.Param, TyClass: typ, Line: b.line(name.Pos())})
				b.bind(obj, n)
			}
			if len(field.Names) == 0 {
				b.node(patterns.PdgNode{Kind: patterns.Param, TyClass: typ, Line: b.line(field.Pos())})
			}
		}
	}
	add(fn.Recv)
	add(fn.Type.Params)
}
