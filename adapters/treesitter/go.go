// Package treesitter parses Go using the Tree-sitter CLI, keeping Cyclo builds
// independent of CGO. A configured Go grammar or an explicit parser library is required.
package treesitter

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/shanejonas/cyclo/adapters/reducer"
)

type GoParser struct {
	Context context.Context
	Library string
}

type node struct {
	XMLName     xml.Name
	StartRow    int    `xml:"srow,attr"`
	StartColumn int    `xml:"scol,attr"`
	EndRow      int    `xml:"erow,attr"`
	EndColumn   int    `xml:"ecol,attr"`
	Children    []node `xml:",any"`
}

type document struct {
	Root node `xml:"source>source_file"`
}

func (p GoParser) Valid(source []byte) (bool, error) {
	_, valid, err := p.parse(source)
	return valid, err
}

func (p GoParser) Deletions(source []byte) ([]reducer.Deletion, error) {
	root, valid, err := p.parse(source)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("cannot reduce syntax rejected by Tree-sitter")
	}
	lines := lineOffsets(source)
	var deletions []reducer.Deletion
	if err := collect(root, lines, len(source), &deletions); err != nil {
		return nil, err
	}
	for index, span := range deletions {
		deletions[index] = wholeLine(source, trailingSemicolon(source, span))
	}
	sort.SliceStable(deletions, func(i, j int) bool {
		return deletions[i].End-deletions[i].Start > deletions[j].End-deletions[j].Start
	})
	return deletions, nil
}

func (p GoParser) parse(source []byte) (node, bool, error) {
	file, err := os.CreateTemp("", "cyclo-treesitter-*.go")
	if err != nil {
		return node{}, false, err
	}
	defer os.Remove(file.Name())
	if err := writeSource(file, source); err != nil {
		return node{}, false, err
	}
	args := []string{"parse", "--xml", "--scope", "source.go"}
	if p.Library != "" {
		args = []string{"parse", "--xml", "--lib-path", p.Library, "--lang-name", "go"}
	}
	args = append(args, file.Name())
	command := exec.CommandContext(p.Context, "tree-sitter", args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, runErr := command.Output()
	if err := p.Context.Err(); err != nil {
		return node{}, false, err
	}
	return decode(output, stderr.String(), runErr)
}

func writeSource(file *os.File, source []byte) error {
	_, err := file.Write(source)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func decode(output []byte, stderr string, runErr error) (node, bool, error) {
	var parsed document
	err := xml.NewDecoder(bytes.NewReader(output)).Decode(&parsed)
	if err != nil || parsed.Root.XMLName.Local != "source_file" {
		return node{}, false, fmt.Errorf("Tree-sitter Go parse failed: %v; XML: %v; %s", runErr, err, stderr)
	}
	if runErr == nil {
		return parsed.Root, true, nil
	}
	if exit, ok := runErr.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return parsed.Root, false, nil
	}
	return node{}, false, fmt.Errorf("Tree-sitter Go parse failed: %w; %s", runErr, stderr)
}

func lineOffsets(source []byte) []int {
	offsets := []int{0}
	for index, value := range source {
		if value == '\n' {
			offsets = append(offsets, index+1)
		}
	}
	return offsets
}

func collect(parent node, lines []int, size int, deletions *[]reducer.Deletion) error {
	if err := collectGroups(parent, lines, size, deletions); err != nil {
		return err
	}
	for _, child := range parent.Children {
		if removable(parent.XMLName.Local, child.XMLName.Local) {
			span, err := deletion(child, lines, size)
			if err != nil {
				return err
			}
			*deletions = append(*deletions, span)
		}
		if err := collect(child, lines, size, deletions); err != nil {
			return err
		}
	}
	return nil
}

func removable(parent, child string) bool {
	if parent == "source_file" {
		return child != "package_clause"
	}
	return parent == "statement_list"
}

func deletion(n node, lines []int, size int) (reducer.Deletion, error) {
	start, err := offset(lines, n.StartRow, n.StartColumn, size)
	if err != nil {
		return reducer.Deletion{}, err
	}
	end, err := offset(lines, n.EndRow, n.EndColumn, size)
	if err != nil {
		return reducer.Deletion{}, err
	}
	if start >= end {
		return reducer.Deletion{}, fmt.Errorf("empty Tree-sitter node: %s", n.XMLName.Local)
	}
	return reducer.Deletion{Start: start, End: end, Unit: n.XMLName.Local}, nil
}

func offset(lines []int, row, column, size int) (int, error) {
	if row < 0 || row >= len(lines) || column < 0 {
		return 0, fmt.Errorf("invalid Tree-sitter position: %d:%d", row, column)
	}
	end := size
	if row+1 < len(lines) {
		end = lines[row+1] - 1
	}
	position := lines[row] + column
	if position > end {
		return 0, fmt.Errorf("Tree-sitter column exceeds line: %d:%d", row, column)
	}
	return position, nil
}

// Include indentation and the line ending only when the unit occupies whole lines.
func wholeLine(source []byte, span reducer.Deletion) reducer.Deletion {
	start := bytes.LastIndexByte(source[:span.Start], '\n') + 1
	end := bytes.IndexByte(source[span.End:], '\n')
	if end < 0 {
		end = len(source)
	} else {
		end += span.End
	}
	if len(bytes.TrimSpace(source[start:span.Start])) == 0 && len(bytes.TrimSpace(source[span.End:end])) == 0 {
		span.Start, span.End = start, min(end+1, len(source))
	}
	return span
}

// Adjacent units can depend on one another, so also try deleting groups.
func collectGroups(parent node, lines []int, size int, deletions *[]reducer.Deletion) error {
	units := removableChildren(parent)
	for count := len(units); count > 1; count = (count + 1) / 2 {
		label := fmt.Sprintf("%s (%d units)", parent.XMLName.Local, count)
		for start := 0; start+count <= len(units); start++ {
			first, last := units[start], units[start+count-1]
			group := node{
				XMLName:  xml.Name{Local: label},
				StartRow: first.StartRow, StartColumn: first.StartColumn,
				EndRow: last.EndRow, EndColumn: last.EndColumn,
			}
			span, err := deletion(group, lines, size)
			if err != nil {
				return err
			}
			*deletions = append(*deletions, span)
		}
	}
	return nil
}

func removableChildren(parent node) []node {
	var units []node
	for _, child := range parent.Children {
		if removable(parent.XMLName.Local, child.XMLName.Local) {
			units = append(units, child)
		}
	}
	return units
}

func trailingSemicolon(source []byte, span reducer.Deletion) reducer.Deletion {
	for index, value := range source[span.End:] {
		if value == ' ' || value == '\t' {
			continue
		}
		if value == ';' {
			span.End += index + 1
		}
		break
	}
	return span
}
