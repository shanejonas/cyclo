package gocyclo

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/shanejonas/cyclo/domain"
)

var diffHunk = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type gitDiff struct {
	root string
	base string
}

func newGitDiff(path string) (gitDiff, bool) {
	root, err := gitText(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return gitDiff{}, false
	}

	root = strings.TrimSpace(root)
	for _, base := range []string{"main", "master", "origin/main", "origin/master", "HEAD"} {
		err = exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", base+"^{commit}").Run()
		if err == nil {
			return gitDiff{root: root, base: base}, true
		}
	}
	return gitDiff{}, false
}

func (d gitDiff) lines(path string) []domain.DiffLine {
	relative, err := filepath.Rel(d.root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}

	output, err := gitText(d.root, "diff", "--no-color", "--no-ext-diff", "--unified=0", d.base, "--", relative)
	if err != nil {
		return nil
	}
	return parseDiff(output)
}

func gitText(root string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	return string(output), err
}

func parseDiff(output string) []domain.DiffLine {
	result := make([]domain.DiffLine, 0)
	oldLine, newLine := 0, 0
	inHunk := false
	for _, line := range strings.Split(output, "\n") {
		matches := diffHunk.FindStringSubmatch(line)
		if matches != nil {
			oldLine, newLine = hunkLines(matches)
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		switch line[0] {
		case '-':
			result = append(result, domain.DiffLine{Kind: domain.DiffDeleted, OldLine: oldLine, NewLine: newLine, Text: line[1:]})
			oldLine++
		case '+':
			result = append(result, domain.DiffLine{Kind: domain.DiffAdded, OldLine: oldLine, NewLine: newLine, Text: line[1:]})
			newLine++
		case ' ':
			oldLine++
			newLine++
		}
	}
	return result
}

func hunkLines(matches []string) (int, int) {
	oldLine, _ := strconv.Atoi(matches[1])
	newLine, _ := strconv.Atoi(matches[3])
	newCount := 1
	if matches[4] != "" {
		newCount, _ = strconv.Atoi(matches[4])
	}
	if newCount == 0 {
		newLine++
	}
	return oldLine, newLine
}

func fileWithDiff(file domain.File, lines []domain.DiffLine) domain.File {
	for index := range file.Functions {
		function := &file.Functions[index]
		function.DiffLines = functionDiff(lines, function.Line, function.EndLine)
	}
	return file
}

func functionDiff(lines []domain.DiffLine, start int, end int) []domain.DiffLine {
	result := make([]domain.DiffLine, 0)
	for _, line := range lines {
		inside := line.Kind == domain.DiffAdded && start <= line.NewLine && line.NewLine <= end
		inside = inside || line.Kind == domain.DiffDeleted && start <= line.NewLine && line.NewLine <= end+1
		if inside {
			result = append(result, line)
		}
	}
	return result
}
