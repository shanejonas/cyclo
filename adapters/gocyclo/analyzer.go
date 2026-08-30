package gocyclo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cyclo "github.com/fzipp/gocyclo"
	"github.com/shanejonas/cyclomatic-complexity-tui/domain"
	cognit "github.com/uudashr/gocognit"
)

type Analyzer struct{}

func NewAnalyzer() Analyzer {
	return Analyzer{}
}

func (Analyzer) Analyze(paths []string) (domain.Report, error) {
	files, roots, err := sourceFiles(paths)
	if err != nil {
		return domain.Report{}, err
	}

	report := domain.Report{Root: commonRoot(roots)}
	diff, hasDiff := newGitDiff(report.Root)
	if hasDiff {
		report.DiffBase = diff.base
	}
	for _, path := range files {
		file, err := analyzeFile(path)
		if err != nil {
			return domain.Report{}, err
		}

		if hasDiff {
			file = fileWithDiff(file, diff.lines(path))
		}
		report.Files = append(report.Files, file)
		report.Functions += len(file.Functions)
		report.Total += file.Total
		report.CognitiveTotal += file.CognitiveTotal
	}
	if report.Functions > 0 {
		report.Average = float64(report.Total) / float64(report.Functions)
		report.CognitiveAverage = float64(report.CognitiveTotal) / float64(report.Functions)
	}

	return report, nil
}

func sourceFiles(paths []string) ([]string, []string, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("at least one path is required")
	}

	files := map[string]struct{}{}
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		root, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve %q: %w", path, err)
		}

		info, err := os.Stat(root)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect %q: %w", path, err)
		}

		roots = append(roots, root)
		if !info.IsDir() {
			if filepath.Ext(root) != ".go" {
				return nil, nil, fmt.Errorf("%q is not a Go source file", path)
			}
			files[root] = struct{}{}
			continue
		}

		err = filepath.WalkDir(root, collectGoFile(root, files))
		if err != nil {
			return nil, nil, fmt.Errorf("scan %q: %w", path, err)
		}
	}

	result := make([]string, 0, len(files))
	for path := range files {
		result = append(result, path)
	}
	sort.Strings(result)

	return result, roots, nil
}

func collectGoFile(root string, files map[string]struct{}) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root && entry.IsDir() && skipDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			return nil
		}

		files[path] = struct{}{}
		return nil
	}
}

func skipDirectory(name string) bool {
	if name == "vendor" || name == "testdata" {
		return true
	}

	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func analyzeFile(path string) (domain.File, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return domain.File{}, fmt.Errorf("read %q: %w", path, err)
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, source, parser.ParseComments)
	if err != nil {
		return domain.File{}, fmt.Errorf("parse %q: %w", path, err)
	}

	stats := cyclo.AnalyzeASTFile(parsed, fileSet, nil)
	cognitiveStats := cognitiveStatsByOffset(parsed, fileSet)
	ranges := functionRanges(parsed, fileSet)
	functions := make([]domain.Function, 0, len(stats))
	for _, stat := range stats {
		functionRange := ranges[stat.Pos.Offset]
		cognitive := cognitiveStats[stat.Pos.Offset]
		functions = append(functions, domain.Function{
			Package:               stat.PkgName,
			Name:                  stat.FuncName,
			Complexity:            stat.Complexity,
			CyclomaticDiagnostics: cyclomaticDiagnostics(functionRange.node, fileSet),
			CognitiveComplexity:   cognitive.Complexity,
			CognitiveDiagnostics:  cognitiveDiagnostics(cognitive.Diagnostics),
			Line:                  stat.Pos.Line,
			EndLine:               functionRange.endLine,
			Column:                stat.Pos.Column,
			Source:                sourceText(source, stat.Pos.Offset, functionRange.endOffset),
		})
	}
	sort.SliceStable(functions, func(left int, right int) bool {
		if functions[left].Complexity != functions[right].Complexity {
			return functions[left].Complexity > functions[right].Complexity
		}
		if functions[left].Line != functions[right].Line {
			return functions[left].Line < functions[right].Line
		}
		return functions[left].Name < functions[right].Name
	})

	file := domain.File{Path: path, Functions: functions}
	for _, function := range functions {
		file.Total += function.Complexity
		file.Peak = max(file.Peak, function.Complexity)
		file.CognitiveTotal += function.CognitiveComplexity
		file.CognitivePeak = max(file.CognitivePeak, function.CognitiveComplexity)
	}
	if len(functions) > 0 {
		file.Average = float64(file.Total) / float64(len(functions))
		file.CognitiveAverage = float64(file.CognitiveTotal) / float64(len(functions))
	}

	return file, nil
}

func cognitiveStatsByOffset(file *ast.File, fileSet *token.FileSet) map[int]cognit.Stat {
	result := map[int]cognit.Stat{}
	for _, stat := range cognit.ComplexityStatsWithDiagnostic(file, fileSet, nil, true) {
		result[stat.Pos.Offset] = stat
	}
	return result
}

func cognitiveDiagnostics(diagnostics []cognit.Diagnostic) []domain.CognitiveDiagnostic {
	result := make([]domain.CognitiveDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, domain.CognitiveDiagnostic{
			Increment: diagnostic.Inc,
			Nesting:   diagnostic.Nesting,
			Kind:      diagnostic.Text,
			Line:      diagnostic.Pos.Line,
			Column:    diagnostic.Pos.Column,
		})
	}
	return result
}

type functionRange struct {
	endOffset int
	endLine   int
	node      ast.Node
}

func functionRanges(file *ast.File, fileSet *token.FileSet) map[int]functionRange {
	ranges := map[int]functionRange{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			start := fileSet.Position(node.Pos())
			end := fileSet.Position(node.End())
			ranges[start.Offset] = functionRange{endOffset: end.Offset, endLine: end.Line, node: node}
		}
		return true
	})

	return ranges
}

func cyclomaticDiagnostics(node ast.Node, fileSet *token.FileSet) []domain.CyclomaticDiagnostic {
	if node == nil {
		return nil
	}

	result := []domain.CyclomaticDiagnostic{cyclomaticDiagnostic("function", node.Pos(), fileSet)}
	ast.Inspect(node, func(node ast.Node) bool {
		kind, position := cyclomaticIncrement(node)
		if kind != "" {
			result = append(result, cyclomaticDiagnostic(kind, position, fileSet))
		}
		return true
	})
	return result
}

func cyclomaticIncrement(node ast.Node) (string, token.Pos) {
	switch node := node.(type) {
	case *ast.IfStmt:
		return "if", node.Pos()
	case *ast.ForStmt, *ast.RangeStmt:
		return "for", node.Pos()
	case *ast.CaseClause:
		if node.List != nil {
			return "case", node.Pos()
		}
	case *ast.CommClause:
		if node.Comm != nil {
			return "case", node.Pos()
		}
	case *ast.BinaryExpr:
		if node.Op == token.LAND || node.Op == token.LOR {
			return node.Op.String(), node.OpPos
		}
	}

	return "", token.NoPos
}

func cyclomaticDiagnostic(kind string, position token.Pos, fileSet *token.FileSet) domain.CyclomaticDiagnostic {
	location := fileSet.Position(position)
	return domain.CyclomaticDiagnostic{Kind: kind, Line: location.Line, Column: location.Column}
}

func sourceText(source []byte, start int, end int) string {
	if start < 0 || end <= start || end > len(source) {
		return ""
	}

	return string(source[start:end])
}

func commonRoot(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	root := directory(paths[0])
	for _, path := range paths[1:] {
		candidate := directory(path)
		for !inside(root, candidate) {
			parent := filepath.Dir(root)
			if parent == root {
				return root
			}
			root = parent
		}
	}

	return root
}

func directory(path string) string {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return path
	}

	return filepath.Dir(path)
}

func inside(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
