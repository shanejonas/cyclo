package application

import (
	"fmt"
	"image"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	treemapHeight   = 11
	analyticsHeight = treemapHeight + 3
)

func (m Model) analyticsLines(width int) []string {
	widths := analyticsWidths(width)
	chart := m.complexityTreemap(widths[0])
	return joinedRows(
		[][]string{
			m.treemapLines(chart),
			m.treemapLegend(widths[1]),
		},
		widths,
	)
}

func analyticsWidths(width int) []int {
	return []int{width - 35, 32}
}

func (m Model) treemapLines(s treemap) []string {
	lines := []string{blue.Render("COMPLEXITY TREEMAP")}
	lines = append(lines, s.render(m.fileIndex, m.functionIndex)...)
	count := float64(max(s.count, 1))
	return append(lines,
		text.Render(fmt.Sprintf("CC %d · peak %d · avg %.1f", s.total, s.peak, float64(s.total)/count)),
		muted.Render(fmt.Sprintf("COG %d · peak %d · avg %.1f", s.cognitiveTotal, s.cognitivePeak, float64(s.cognitiveTotal)/count)),
	)
}

func (m Model) treemapLegend(width int) []string {
	lines := []string{
		blue.Render("SELECTION"),
		muted.Render("Area: cyclomatic complexity"),
		muted.Render("Color: cognitive complexity"),
		muted.Render("COG scale"),
		treemapSwatch(0) + " 0  " + treemapSwatch(10) + " 10  " + treemapSwatch(20) + " 20  " + treemapSwatch(30) + " 30+",
		"",
	}
	if m.refreshing {
		return append(lines, muted.Render("Scanning Go code…"))
	}
	if m.err != nil {
		return append(lines, danger.Render("Scan failed · r to retry"))
	}
	file, ok := m.selectedFile()
	if !ok {
		return append(lines, muted.Render("No Go functions to map"))
	}
	lines = append(lines,
		text.Render(truncate(displayPath(m.report.Root, file.Path), width)),
		fmt.Sprintf("CC %d · COG %d", file.Total, file.CognitiveTotal),
	)
	function, ok := m.selectedFunction()
	if ok {
		lines = append(lines,
			text.Bold(true).Render(truncate(function.Name, width)),
			fmt.Sprintf("CC %d · COG %d", function.Complexity, function.CognitiveComplexity),
		)
	}
	return append(lines,
		muted.Render("White outline = selected"),
		muted.Render("Click a tile · j/k to move"),
		muted.Render(fmt.Sprintf("%d files · %d functions", len(m.report.Files), m.report.Functions)),
	)
}

func treemapSwatch(score int) string {
	return lipgloss.NewStyle().Foreground(treemapColor(score)).Render("█")
}

func (m Model) selectTreemap(message tea.MouseClickMsg) Model {
	if m.annotating || message.Button != tea.MouseLeft {
		return m
	}
	tile, ok := m.treemapTileAt(message.X, message.Y)
	if !ok {
		return m
	}
	m.fileIndex = tile.file
	m.functionIndex = max(tile.function, 0)
	m.focus = functionsPane
	if tile.function < 0 {
		m.focus = filesPane
	}
	m = m.resetSourceWorkspace()
	m.revision++
	return m
}

func (m Model) treemapTileAt(x int, y int) (treemapTile, bool) {
	if !m.showAnalytics(m.terminalWidth()) {
		return treemapTile{}, false
	}
	y -= 3 // Header, rule, chart title.
	widths := analyticsWidths(m.terminalWidth())
	if !image.Pt(x, y).In(image.Rect(0, 0, widths[0], treemapHeight)) {
		return treemapTile{}, false
	}
	chart := m.complexityTreemap(widths[0])
	return chart.tileAt(image.Pt(x, y*2))
}
