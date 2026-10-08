package application

import (
	"context"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/application/patterncheck"
	"github.com/shanejonas/cyclo/domain/patterns"
)

// patternBackground is the line highlight for pattern findings. Distinct from
// the user-annotation bronze (44;34;14) and the selection slate (45;52;54).
const patternBackground = "\x1b[48;2;66;54;20m"

// PatternMarker is one visual marker for a pattern finding site.
type PatternMarker struct {
	Kind    string
	Path    string
	Line    int
	EndLine int
}

// patternsMsg carries background pattern-detection results.
type patternsMsg struct {
	markers []PatternMarker
	err     error
}

// loadPatternsCmd runs pattern detection in the background so TUI startup
// stays fast. Results arrive as patternsMsg.
func (m Model) loadPatternsCmd() tea.Cmd {
	paths := append([]string(nil), m.paths...)
	return func() tea.Msg {
		report, err := patterncheck.GetReport(context.Background(), paths)
		if err != nil {
			return patternsMsg{err: err}
		}
		return patternsMsg{markers: patternMarkersFromReport(report)}
	}
}

// patternMarkersFromReport converts pattern candidates' sites into markers.
func patternMarkersFromReport(report *patterns.PatternsReport) []PatternMarker {
	if report == nil {
		return nil
	}
	var out []PatternMarker
	for i := range report.Candidates {
		c := &report.Candidates[i]
		kind := string(c.Kind)
		for _, s := range c.Sites {
			out = append(out, PatternMarker{
				Kind:    kind,
				Path:    s.Path,
				Line:    s.Line,
				EndLine: s.EndLine,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// patternPathMatches reports whether a marker path and a file path refer to
// the same file, tolerating relative/absolute prefix differences.
func patternPathMatches(markerPath, filePath string) bool {
	if markerPath == filePath {
		return true
	}
	return strings.HasSuffix(markerPath, "/"+filePath) ||
		strings.HasSuffix(filePath, "/"+markerPath)
}

// selectedFilePath returns the selected file's path, or "" when none.
func (m Model) selectedFilePath() string {
	file, ok := m.selectedFile()
	if !ok {
		return ""
	}
	return file.Path
}

// patternMarkersAtLine returns markers covering path:line, deduplicated by kind.
func patternMarkersAtLine(markers []PatternMarker, path string, line int) []PatternMarker {
	var out []PatternMarker
	seen := map[string]bool{}
	for _, mk := range markers {
		if patternMarkerCovers(mk, path, line) && !seen[mk.Kind] {
			seen[mk.Kind] = true
			out = append(out, mk)
		}
	}
	return out
}

// patternMarkerCovers reports whether the marker's range includes the line.
func patternMarkerCovers(mk PatternMarker, path string, line int) bool {
	if !patternPathMatches(mk.Path, path) {
		return false
	}
	end := mk.EndLine
	if end <= 0 {
		end = mk.Line
	}
	return mk.Line <= line && line <= end
}

// visiblePatternMarkers returns markers for the selected file, or nil when
// pattern highlights are toggled off.
func (m Model) visiblePatternMarkers() []PatternMarker {
	if !m.showPatterns || len(m.patternMarkers) == 0 {
		return nil
	}
	path := m.selectedFilePath()
	if path == "" {
		return nil
	}
	var out []PatternMarker
	for _, mk := range m.patternMarkers {
		if patternPathMatches(mk.Path, path) {
			out = append(out, mk)
		}
	}
	return out
}

// togglePatterns flips pattern highlight visibility.
func (m Model) togglePatterns() Model {
	m.showPatterns = !m.showPatterns
	return m
}

// highlightPatternLine applies the pattern background to a rendered line when
// markers cover the line. Annotation and selection highlights are applied
// afterwards and take precedence.
func highlightPatternLine(rendered string, markers []PatternMarker) string {
	if len(markers) == 0 {
		return rendered
	}
	return withBackground(rendered, patternBackground)
}

// sourcePatternRows renders one row per distinct pattern kind at the line,
// mirroring the user-annotation row style but with a distinct glyph.
func sourcePatternRows(markers []PatternMarker, width int) []string {
	if len(markers) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(markers))
	seen := map[string]bool{}
	for _, mk := range markers {
		if !seen[mk.Kind] {
			seen[mk.Kind] = true
			kinds = append(kinds, mk.Kind)
		}
	}
	sort.Strings(kinds)
	prefix := muted.Render("      ╰─") + amber.Bold(true).Render("▲ ")
	message := "pattern: " + strings.Join(kinds, ", ")
	return patternMessageRows(message, prefix, width)
}

// patternMessageRows wraps a pattern message like annotationMessageRows.
func patternMessageRows(message string, prefix string, width int) []string {
	prefixWidth := ansi.StringWidth(prefix)
	indent := strings.Repeat(" ", prefixWidth)
	messageWidth := max(width-prefixWidth, 1)
	rows := strings.Split(ansi.Wrap(message, messageWidth, ""), "\n")
	for index, row := range rows {
		rows[index] = prefix + amber.Render(row)
		prefix = indent
	}
	return rows
}

// patternMarkersStartingAt returns markers whose range starts at the line.
// Used for the label row, which shows once per finding, while the background
// highlight covers the whole range.
func patternMarkersStartingAt(markers []PatternMarker, path string, line int) []PatternMarker {
	var out []PatternMarker
	seen := map[string]bool{}
	for _, mk := range markers {
		if !patternPathMatches(mk.Path, path) || mk.Line != line || seen[mk.Kind] {
			continue
		}
		seen[mk.Kind] = true
		out = append(out, mk)
	}
	return out
}

// sourcePatternRowCount counts display rows added by pattern markers, for
// scroll math. Mirrors sourceAnnotationRowCount.
func sourcePatternRowCount(markers []PatternMarker, path string, start int, end int, width int) int {
	count := 0
	for line := start; line <= end; line++ {
		count += len(sourcePatternRows(patternMarkersStartingAt(markers, path, line), width))
	}
	return count
}
