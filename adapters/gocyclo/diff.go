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

	output, err := gitText(d.root, "--literal-pathspecs", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--unified=0", d.base, "--", relative)
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

func (d gitDiff) apply(file domain.File) domain.File {
	if d.root == "" {
		return file
	}
	return fileWithDiff(file, d.lines(file.Path))
}

type diffPosition struct {
	oldLine int
	newLine int
}

func parseDiff(output string) []domain.DiffLine {
	result := make([]domain.DiffLine, 0)
	position := diffPosition{}
	inHunk := false
	for line := range strings.SplitSeq(output, "\n") {
		matches := diffHunk.FindStringSubmatch(line)
		if matches != nil {
			position.oldLine, position.newLine = hunkLines(matches)
			inHunk = true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		change := position.advance(line)
		if change.Kind != "" {
			result = append(result, change)
		}
	}
	return result
}

func (p *diffPosition) advance(line string) domain.DiffLine {
	change := domain.DiffLine{OldLine: p.oldLine, NewLine: p.newLine, Text: line[1:]}
	switch line[0] {
	case '-':
		change.Kind = domain.DiffDeleted
		p.oldLine++
	case '+':
		change.Kind = domain.DiffAdded
		p.newLine++
	case ' ':
		*p = diffPosition{oldLine: p.oldLine + 1, newLine: p.newLine + 1}
	}
	return change
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
		if diffInsideFunction(line, start, end) {
			result = append(result, line)
		}
	}
	return result
}

func diffInsideFunction(line domain.DiffLine, start int, end int) bool {
	switch line.Kind {
	case domain.DiffDeleted:
		end++
	case domain.DiffAdded:
	default:
		return false
	}
	return start <= line.NewLine && line.NewLine <= end
}
