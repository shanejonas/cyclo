// Package gitchanged maps a git diff onto source line ranges so CLI
// commands can scope their work to functions the diff touches.
// Shared by check --changed and fix --changed.
package gitchanged

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// LineRange is an inclusive range of new-side line numbers touched by the
// git diff.
type LineRange struct {
	Start, End int
}

// Overlaps reports whether [start, end] intersects r.
func Overlaps(start, end int, r LineRange) bool {
	return start <= r.End && r.Start <= end
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func gitOutput(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.Output()
	return string(output), err
}

// GitRoot returns the repo top level for cwd.
func GitRoot(cwd string) (string, error) {
	output, err := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// ResolveBase picks the diff base: an explicit --base ref, else the
// merge-base of HEAD with main or master (so a branch diffs against its
// branch point, not the moving tip), else HEAD. An empty base means the repo
// has no commits yet, in which case only untracked files contribute changes.
func ResolveBase(root, explicit string) (string, error) {
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

// branchBase returns the merge-base of HEAD with main or master.
func branchBase(root string) (string, bool) {
	for _, ref := range []string{"main", "master"} {
		if output, err := gitOutput(root, "merge-base", "HEAD", ref); err == nil && strings.TrimSpace(output) != "" {
			return strings.TrimSpace(output), true
		}
	}
	return "", false
}

// ChangedRanges maps repo-root-relative paths to the new-side line ranges
// the diff touches. Untracked files count as fully changed.
func ChangedRanges(root, base string, paths []string) (map[string][]LineRange, error) {
	result := map[string][]LineRange{}
	if base != "" {
		args := append([]string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--unified=0", base, "--"}, paths...)
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

// RootSpecs converts CLI paths (relative to cwd) to repo-root-relative git
// pathspecs, defaulting to the whole tree.
func RootSpecs(paths []string, root, cwd string) []string {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	specs := make([]string, len(paths))
	for index, path := range paths {
		specs[index] = RootRelative(path, root, cwd)
	}
	return specs
}

// RootRelative resolves path (absolute or relative to cwd) against root.
func RootRelative(path, root, cwd string) string {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, path)
	}
	if rel, err := filepath.Rel(root, abs); err == nil {
		return rel
	}
	return path
}

// Diff is a git diff scoped to pathspecs: the repo root, the working
// directory it was computed from, and the per-file touch ranges.
type Diff struct {
	Root   string
	Cwd    string
	Ranges map[string][]LineRange
}

// NewDiff resolves the repo root and diff base, then maps the diff onto
// touch ranges for paths (repo-root-relative pathspecs, defaulting to the
// whole tree when empty).
func NewDiff(cwd string, paths []string, base string) (*Diff, error) {
	root, err := GitRoot(cwd)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveBase(root, base)
	if err != nil {
		return nil, err
	}
	ranges, err := ChangedRanges(root, resolved, RootSpecs(paths, root, cwd))
	if err != nil {
		return nil, err
	}
	return &Diff{Root: root, Cwd: cwd, Ranges: ranges}, nil
}

// Touched reports whether the line range [startLine, endLine] of path (any
// absolute path, or relative to the working directory) intersects the diff.
func (d *Diff) Touched(path string, startLine, endLine int) bool {
	fileRanges, ok := d.Ranges[RootRelative(path, d.Root, d.Cwd)]
	if !ok {
		return false
	}
	for _, r := range fileRanges {
		if Overlaps(startLine, endLine, r) {
			return true
		}
	}
	return false
}

// untrackedRanges treats every untracked Go file as fully changed.
func untrackedRanges(root string, paths []string) (map[string][]LineRange, error) {
	files, err := untrackedFiles(root, paths)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]LineRange, len(files))
	for _, path := range files {
		lines, err := countLines(filepath.Join(root, path))
		if err != nil {
			continue
		}
		result[path] = []LineRange{{Start: 1, End: lines}}
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
func parseFileRanges(output string) map[string][]LineRange {
	result := map[string][]LineRange{}
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

func parseHunkRange(line string) (LineRange, bool) {
	matches := hunkHeader.FindStringSubmatch(line)
	if matches == nil {
		return LineRange{}, false
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
	return LineRange{Start: start, End: end}, true
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
