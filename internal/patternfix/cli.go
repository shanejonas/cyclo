// Package patternfix is the IO boundary for cyclo fix: static auto-fix
// for patterns miner candidates. No LLM, no tokens — pure AST mechanics.
// Dry-run by default (prints a diff); --apply writes files.
package patternfix

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
)

const usage = `Usage: cyclo fix [OPTIONS] [DIRECTORIES OR GO FILES...]

Statically apply fixes for patterns miner candidates. No LLM, no tokens:
pure AST rewrites that are provably behavior-preserving.
Dry-run by default (shows a diff); --apply writes the files.

  --kind KIND   which fixes to apply: guard_clause (default), value_object, parameterize, anemic_model, or all
  --apply       write the fixes to disk (default: dry-run diff only)

Exit 0: always, on success (even with no fixes). Exit 2: parse or IO failure.
`

type options struct {
	kind  string
	apply bool
	paths []string
}

// Run finds fixable patterns in paths and either shows a diff (default)
// or writes the fixes (--apply).
func Run(ctx context.Context, args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	files, err := goFiles(opts.paths)
	if err != nil {
		return err
	}
	fixed, err := fixAll(ctx, files, opts, output)
	if err != nil {
		return err
	}
	if !opts.apply {
		fmt.Fprintf(output, "%d fixable %s candidate(s) (dry-run; use --apply to write)\n", fixed, opts.kind)
	}
	return nil
}

// fixAll fixes each file, respecting context cancellation.
func fixAll(ctx context.Context, files []string, opts options, output io.Writer) (int, error) {
	var fixed int
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return fixed, err
		}
		n, err := fixFile(path, opts, output)
		if err != nil {
			return fixed, err
		}
		fixed += n
	}
	return fixed, nil
}

// fixFile applies fixes to one file. In dry-run mode it prints a diff;
// with --apply it writes the file. Returns the number of fixes.
func fixFile(path string, opts options, output io.Writer) (int, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	out, fixes, err := fixSource(path, src, opts.kind)
	if err != nil {
		return 0, err
	}
	if len(fixes) == 0 {
		return 0, nil
	}
	if opts.apply {
		return applyFixes(path, out, fixes, output)
	}
	return showDiff(path, src, out, fixes, output)
}

// kindFixers maps fix kinds to their implementations. Adding a new kind
// is a new entry here plus its fixer function.
var kindFixers = map[string]func(*token.FileSet, *ast.File, []byte) ([]byte, []gopatterns.Fix, error){
	"guard_clause": fixGuardClause,
	"value_object": fixValueObjectKind,
	"parameterize": fixParameterizeKind,
	"anemic_model": fixAnemicModelKind,
}

// fixSource parses src and returns the fixed source and the fixes applied.
func fixSource(path string, src []byte, kind string) ([]byte, []gopatterns.Fix, error) {
	if kind == "all" {
		return fixAllKinds(path, src)
	}
	fix, ok := kindFixers[kind]
	if !ok {
		return nil, nil, fmt.Errorf("unknown kind %q", kind)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return fix(fset, f, src)
}

// fixValueObjectKind applies value-object fixes.
func fixValueObjectKind(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []gopatterns.Fix, error) {
	fixed, valueFixes, err := gopatterns.FixValueObjects(fset, f, src)
	if err != nil {
		return nil, nil, err
	}
	return fixed, toFixes(valueFixes), nil
}

// fixAllKinds applies guard_clause, value_object, parameterize, then
// anemic_model, re-parsing between kinds so positions stay valid.
// fixGuardClause applies guard-clause fixes.
func fixGuardClause(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []gopatterns.Fix, error) {
	fixed, guardFixes, err := gopatterns.FixInvertedGuards(fset, f, src)
	if err != nil {
		return nil, nil, err
	}
	return fixed, toFixes(guardFixes), nil
}

// fixParameterizeKind applies parameterize fixes.
func fixParameterizeKind(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []gopatterns.Fix, error) {
	fixed, paramFixes, err := gopatterns.FixParameterize(fset, f, src, nil)
	if err != nil {
		return nil, nil, err
	}
	return fixed, toFixes(paramFixes), nil
}

// fixAnemicModelKind applies anemic-model fixes.
func fixAnemicModelKind(fset *token.FileSet, f *ast.File, src []byte) ([]byte, []gopatterns.Fix, error) {
	fixed, anemicFixes, err := gopatterns.FixAnemicModels(fset, f, src)
	if err != nil {
		return nil, nil, err
	}
	return fixed, toFixes(anemicFixes), nil
}

// toFixes converts typed fixes to the Fix interface.
func toFixes[T gopatterns.Fix](fixes []T) []gopatterns.Fix {
	out := make([]gopatterns.Fix, len(fixes))
	for i, fx := range fixes {
		out[i] = fx
	}
	return out
}

func fixAllKinds(path string, src []byte) ([]byte, []gopatterns.Fix, error) {
	src1, fixes1, err := fixSource(path, src, "guard_clause")
	if err != nil {
		return nil, nil, err
	}
	src2, fixes2, err := fixSource(path, src1, "value_object")
	if err != nil {
		return nil, nil, err
	}
	src3, fixes3, err := fixSource(path, src2, "parameterize")
	if err != nil {
		return nil, nil, err
	}
	src4, fixes4, err := fixSource(path, src3, "anemic_model")
	if err != nil {
		return nil, nil, err
	}
	return src4, append(append(append(fixes1, fixes2...), fixes3...), fixes4...), nil
}

// applyFixes writes the fixed source and reports.
func applyFixes(path string, out []byte, fixes []gopatterns.Fix, output io.Writer) (int, error) {
	if err := os.WriteFile(path, out, 0644); err != nil {
		return 0, err
	}
	byKind := map[string]int{}
	for _, fx := range fixes {
		byKind[fx.FixKind()]++
	}
	for kind, n := range byKind {
		fmt.Fprintf(output, "fixed %d %s in %s\n", n, kind, path)
	}
	return len(fixes), nil
}

// showDiff prints a unified diff for dry-run.
func showDiff(path string, src, out []byte, fixes []gopatterns.Fix, output io.Writer) (int, error) {
	diff, err := unifiedDiff(path, src, out)
	if err != nil {
		for _, fx := range fixes {
			fmt.Fprintf(output, "%s:%d: would fix %s\n", path, fx.FixLine(), fx.FixKind())
		}
		return len(fixes), nil
	}
	io.WriteString(output, diff)
	return len(fixes), nil
}

// unifiedDiff returns a unified diff of old -> new via diff -u.
func unifiedDiff(path string, old, new []byte) (string, error) {
	oldFile, err := os.CreateTemp("", "cyclo-fix-old-*.go")
	if err != nil {
		return "", err
	}
	defer os.Remove(oldFile.Name())
	newFile, err := os.CreateTemp("", "cyclo-fix-new-*.go")
	if err != nil {
		return "", err
	}
	defer os.Remove(newFile.Name())
	if _, err := oldFile.Write(old); err != nil {
		return "", err
	}
	if _, err := newFile.Write(new); err != nil {
		return "", err
	}
	oldFile.Close()
	newFile.Close()

	cmd := exec.Command("diff", "-u", "--label", "a/"+path, "--label", "b/"+path, oldFile.Name(), newFile.Name())
	out, _ := cmd.Output()
	// diff exits 1 when files differ; that's expected.
	return string(out), nil
}

// goFiles resolves paths to a list of .go files. Directories are walked;
// .go files are used directly. Test files and vendor/ are skipped.
func goFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	var out []string
	for _, p := range paths {
		files, err := resolvePath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, files...)
	}
	slices.Sort(out)
	return out, nil
}

// resolvePath returns the .go files for a single path.
func resolvePath(p string) ([]string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(p, ".go") {
			return []string{p}, nil
		}
		return nil, nil
	}
	return walkGoFiles(p)
}

// walkGoFiles walks dir for .go files, skipping vendor/, dot-dirs, and tests.
func walkGoFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if skipDir(info) {
			return filepath.SkipDir
		}
		if isGoFile(info, path) {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// skipDir reports whether a directory should not be walked.
func skipDir(info os.FileInfo) bool {
	return info.IsDir() && (info.Name() == "vendor" || strings.HasPrefix(info.Name(), "."))
}

// isGoFile reports whether path is a non-test Go file.
func isGoFile(info os.FileInfo, path string) bool {
	return !info.IsDir() &&
		strings.HasSuffix(path, ".go") &&
		!strings.HasSuffix(path, "_test.go")
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("fix", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "guard_clause", "which fixes to apply")
	apply := flags.Bool("apply", false, "write fixes to disk")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	result := options{kind: *kind, apply: *apply, paths: flags.Args()}
	return result, result.validate()
}

func (opts options) validate() error {
	if !slices.Contains([]string{"guard_clause", "value_object", "parameterize", "anemic_model", "all"}, opts.kind) {
		return fmt.Errorf("kind must be guard_clause, value_object, parameterize, anemic_model, or all")
	}
	return nil
}
