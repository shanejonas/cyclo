// Package goquality extracts policy-free facts from type-checked Go packages.
package goquality

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/shanejonas/cyclo/domain/quality"
	"golang.org/x/tools/go/packages"
)

type Analyzer struct {
	Root       string
	Tests      bool
	BuildFlags []string
}

// Extract respects the active Go build configuration. It loads enclosing
// packages for file arguments but emits only requested files.
func (a Analyzer) Extract(ctx context.Context, paths []string) ([]quality.Function, error) {
	root, err := analysisRoot(a.Root)
	if err != nil {
		return nil, err
	}
	patterns, selected, err := loadPatterns(root, paths)
	if err != nil {
		return nil, err
	}
	config := &packages.Config{Context: ctx, Dir: root, Mode: packages.LoadSyntax | packages.NeedForTest, Tests: a.Tests, BuildFlags: a.BuildFlags}
	pkgs, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load Go packages: %w", err)
	}
	if err := packageErrors(pkgs, root); err != nil {
		return nil, err
	}
	return extractPackages(pkgs, root, selected)
}

func extractPackages(pkgs []*packages.Package, root string, selected selection) ([]quality.Function, error) {
	facts := []quality.Function{}
	for _, pkg := range pkgs {
		functions, err := extractPackage(pkg, root, selected)
		if err != nil {
			return nil, err
		}
		facts = append(facts, functions...)
	}
	// Evaluation validates and deduplicates test variants. Extraction also emits
	// one record per physical function for consumers saving raw facts.
	return uniqueFacts(facts)
}

func uniqueFacts(facts []quality.Function) ([]quality.Function, error) {
	slices.SortFunc(facts, compareFacts)
	for index := 1; index < len(facts); index++ {
		if compareFacts(facts[index-1], facts[index]) == 0 && !reflect.DeepEqual(facts[index-1], facts[index]) {
			return nil, fmt.Errorf("conflicting Go build variants for %s:%d", facts[index].Path, facts[index].Line)
		}
	}
	facts = slices.CompactFunc(facts, func(a, b quality.Function) bool { return compareFacts(a, b) == 0 })
	return facts, nil
}

func analysisRoot(root string) (string, error) {
	if root == "" {
		return os.Getwd()
	}
	return filepath.Abs(root)
}

type selection struct{ directories, files []string }

func loadPatterns(root string, paths []string) ([]string, selection, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	patterns := []string{}
	selected := selection{}
	for _, input := range paths {
		pattern, err := selected.addInput(root, input)
		if err != nil {
			return nil, selected, err
		}
		patterns = append(patterns, pattern)
	}
	slices.Sort(patterns)
	return slices.Compact(patterns), selected, nil
}

func inputPath(root, input string) (string, error) {
	if strings.Contains(input, "...") {
		return "", fmt.Errorf("use directory or .go file paths, not package patterns: %q", input)
	}
	absolute := input
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, input)
	}
	absolute = filepath.Clean(absolute)
	if _, err := relativePath(root, absolute); err != nil {
		return "", err
	}
	return absolute, nil
}

func (s *selection) addInput(root, input string) (string, error) {
	absolute, err := inputPath(root, input)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect input %q: %w", input, err)
	}
	if info.IsDir() {
		s.directories = append(s.directories, absolute)
		return filepath.Join(absolute, "..."), nil
	}
	if !info.Mode().IsRegular() || filepath.Ext(absolute) != ".go" {
		return "", fmt.Errorf("input must be a directory or regular Go file: %q", input)
	}
	s.files = append(s.files, absolute)
	return "file=" + absolute, nil
}

func (s selection) includes(file string) bool {
	if slices.Contains(s.files, file) {
		return true
	}
	for _, directory := range s.directories {
		if file == directory || strings.HasPrefix(file, directory+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func relativePath(root, file string) (string, error) {
	path, err := filepath.Rel(root, file)
	if err != nil {
		return "", err
	}
	if path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source must be inside analysis root: %s", filepath.Base(file))
	}
	return filepath.ToSlash(path), nil
}

func packageErrors(pkgs []*packages.Package, root string) error {
	messages := []string{}
	for _, pkg := range pkgs {
		for _, err := range pkg.Errors {
			messages = append(messages, strings.ReplaceAll(err.Error(), root+string(filepath.Separator), ""))
		}
	}
	slices.Sort(messages)
	if len(messages) > 0 {
		return fmt.Errorf("Go analysis fails:\n%s", strings.Join(slices.Compact(messages), "\n"))
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no Go packages match the requested paths")
	}
	return nil
}

func extractPackage(pkg *packages.Package, root string, selected selection) ([]quality.Function, error) {
	facts := []quality.Function{}
	available := []quality.Function{}
	for _, file := range pkg.Syntax {
		filename := pkg.Fset.PositionFor(file.Pos(), false).Filename
		path, err := relativePath(root, filename)
		if err != nil {
			continue
		}
		functions, err := extractFile(pkg, file, path, filename)
		if err != nil {
			return nil, err
		}
		available = append(available, functions...)
		if selected.includes(filename) && !ast.IsGenerated(file) {
			facts = append(facts, functions...)
		}
	}
	return attachHelpers(facts, available), nil
}

func extractFile(pkg *packages.Package, file *ast.File, path, filename string) ([]quality.Function, error) {
	source, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return extractDeclarations(pkg, file, path, source), nil
}

func extractDeclarations(pkg *packages.Package, file *ast.File, path string, source []byte) []quality.Function {
	facts := []quality.Function{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			facts = append(facts, extractInitializers(pkg, declaration, path, source)...)
			continue
		}
		if function.Body == nil {
			continue
		}
		facts = append(facts, extractFunction(pkg, function, path, source))
	}
	return facts
}

func extractInitializers(pkg *packages.Package, declaration ast.Decl, path string, source []byte) []quality.Function {
	facts := []quality.Function{}
	ast.Inspect(declaration, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if !ok {
			return true
		}
		pos := physicalPosition(pkg.Fset, literal.Pos())
		name := fmt.Sprintf("%s.<literal@%d:%d>", pkg.PkgPath, pos.Line, pos.Column)
		// No Doc: a declaration's suppression cannot target an embedded literal.
		function := &ast.FuncDecl{Name: ast.NewIdent(name), Type: literal.Type, Body: literal.Body}
		facts = append(facts, extractFunction(pkg, function, path, source))
		return false
	})
	return facts
}

func compareFacts(a, b quality.Function) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Line != b.Line {
		return a.Line - b.Line
	}
	if a.Column != b.Column {
		return a.Column - b.Column
	}
	return strings.Compare(a.Name, b.Name)
}

func physicalPosition(fset *token.FileSet, pos token.Pos) token.Position {
	return fset.PositionFor(pos, false)
}
