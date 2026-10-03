package qualitycheck

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/shanejonas/cyclo/domain/quality"
)

// lineRange is an inclusive range of new-side line numbers touched by the git diff.
type lineRange struct {
	start, end int
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func overlaps(start, end int, r lineRange) bool {
	return start <= r.end && r.start <= end
}

func gitOutput(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.Output()
	return string(output), err
}

func gitRoot(cwd string) (string, error) {
	output, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// resolveBase picks the diff base: an explicit --base ref, else the merge-base
// of HEAD with main or master (so a branch diffs against its branch point,
// not the moving tip), else HEAD. An empty base means the repo has no commits
// yet, in which case only untracked files contribute changes.
func resolveBase(root, explicit string) (string, error) {
	if explicit != "" {
		return verifyBase(root, explicit)
	}
	if base, ok := branchBase(root); ok {
		return base, nil
	}
	if _, err := gitOutput(root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return "", nil
	}
	return "HEAD", nil
}

func verifyBase(root, explicit string) (string, error) {
	if _, err := gitOutput(root, "rev-parse", "--verify", "--quiet", explicit+"^{commit}"); err != nil {
		return "", fmt.Errorf("unknown base %q", explicit)
	}
	return explicit, nil
}

// branchBase returns the merge-base of HEAD with main or master, so a branch
// diffs against its branch point rather than the moving tip.
func branchBase(root string) (string, bool) {
	for _, ref := range []string{"main", "master"} {
		if output, err := gitOutput(root, "merge-base", "HEAD", ref); err == nil && strings.TrimSpace(output) != "" {
			return strings.TrimSpace(output), true
		}
	}
	return "", false
}

// changedRanges maps repo-root-relative paths to the new-side line ranges the
// diff touches. Untracked files count as fully changed.
func changedRanges(root, base string, paths []string) (map[string][]lineRange, error) {
	result := map[string][]lineRange{}
	if base != "" {
		args := append([]string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=0", base, "--"}, paths...)
		output, err := gitOutput(root, args...)
		if err != nil {
			return nil, fmt.Errorf("git diff: %w", err)
		}
		result = parseFileRanges(output)
	}
	untracked, err := untrackedRanges(root, paths)
	if err != nil {
		return nil, err
	}
	maps.Copy(result, untracked)
	return result, nil
}

// untrackedRanges treats every untracked Go file as fully changed.
func untrackedRanges(root string, paths []string) (map[string][]lineRange, error) {
	files, err := untrackedFiles(root, paths)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]lineRange, len(files))
	for _, path := range files {
		lines, err := countLines(filepath.Join(root, path))
		if err != nil {
			continue
		}
		result[path] = []lineRange{{1, lines}}
	}
	return result, nil
}

func untrackedFiles(root string, paths []string) ([]string, error) {
	args := append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, paths...)
	output, err := gitOutput(root, args...)
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	output = strings.TrimSuffix(output, "\x00")
	if output == "" {
		return nil, nil
	}
	var result []string
	for _, path := range strings.Split(output, "\x00") {
		if strings.HasSuffix(path, ".go") {
			result = append(result, path)
		}
	}
	return result, nil
}

func countLines(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	if len(content) == 0 {
		return 0, nil
	}
	return strings.Count(string(content), "\n") + 1, nil
}

// parseFileRanges extracts per-file new-side touch ranges from unified diff
// output. A pure-deletion hunk touches the single new-side line it precedes.
func parseFileRanges(output string) map[string][]lineRange {
	result := map[string][]lineRange{}
	var current string
	for line := range strings.SplitSeq(output, "\n") {
		if path, ok := diffNewPath(line); ok {
			current = path
			continue
		}
		if current == "" {
			continue
		}
		if r, ok := parseHunkRange(line); ok {
			result[current] = append(result[current], r)
		}
	}
	return result
}

func parseHunkRange(line string) (lineRange, bool) {
	matches := hunkHeader.FindStringSubmatch(line)
	if matches == nil {
		return lineRange{}, false
	}
	start, _ := strconv.Atoi(matches[1])
	count := 1
	if matches[2] != "" {
		count, _ = strconv.Atoi(matches[2])
	}
	end := start + count - 1
	if count == 0 {
		end = start
	}
	return lineRange{start, end}, true
}

// diffNewPath extracts the b/ (new-side) path from a "diff --git" header,
// handling git's quoting of paths with spaces.
func diffNewPath(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "diff --git ")
	if !ok {
		return "", false
	}
	if strings.HasPrefix(rest, "\"") {
		var old, new string
		if _, err := fmt.Sscanf(rest, "%q %q", &old, &new); err != nil {
			return "", false
		}
		return strings.TrimPrefix(new, "b/"), true
	}
	_, new, ok := strings.Cut(rest, " b/")
	if !ok {
		return "", false
	}
	return strings.TrimPrefix(new, "b/"), true
}

// touchedFunctions returns the set of fact keys (path + name) for functions
// whose line range intersects a diff touch range.
func touchedFunctions(facts []quality.Function, ranges map[string][]lineRange, root, cwd string) map[string]bool {
	result := map[string]bool{}
	for _, fact := range facts {
		rel := rootRelative(fact.Location.Path, root, cwd)
		fileRanges, ok := ranges[rel]
		if !ok {
			continue
		}
		start := fact.Location.Line
		end := start + strings.Count(fact.Source, "\n")
		for _, r := range fileRanges {
			if overlaps(start, end, r) {
				result[fact.Location.Path+"\x00"+fact.Location.Name] = true
				break
			}
		}
	}
	return result
}

func rootRelative(factPath, root, cwd string) string {
	abs := factPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, factPath)
	}
	if rel, err := filepath.Rel(root, abs); err == nil {
		return rel
	}
	return factPath
}

// filterChanged keeps only diagnostics whose function the diff touches.
func filterChanged(report quality.Report, touched map[string]bool) quality.Report {
	diagnostics := make([]quality.Diagnostic, 0, len(report.Diagnostics))
	for _, diagnostic := range report.Diagnostics {
		if touched[diagnostic.Path+"\x00"+diagnostic.Name] {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	report.Diagnostics = diagnostics
	return report
}
