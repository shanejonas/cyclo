package application

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/domain"
)

const (
	wideMinimum            = 80
	analyticsMinimum       = 100
	analyticsHeightMinimum = 30
)

var (
	green               = lipgloss.NewStyle().Foreground(lipgloss.Color("#66a678"))
	blue                = lipgloss.NewStyle().Foreground(lipgloss.Color("#6999b0"))
	amber               = lipgloss.NewStyle().Foreground(lipgloss.Color("#ddc27d"))
	cognitiveColor      = lipgloss.Color("#a78bfa")
	cognitiveBackground = lipgloss.Color("#2b2140")
	cognitive           = lipgloss.NewStyle().Foreground(cognitiveColor)
	danger              = lipgloss.NewStyle().Foreground(lipgloss.Color("#d16969"))
	text                = lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd3d5"))
	muted               = lipgloss.NewStyle().Foreground(lipgloss.Color("#768286"))
	border              = lipgloss.NewStyle().Foreground(lipgloss.Color("#3a464a"))
)

func (m Model) View() tea.View {
	content := m.wideView()
	if m.width > 0 && m.width < wideMinimum {
		content = m.narrowView()
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "cyclo"
	return view
}

func (m Model) wideView() string {
	width := m.terminalWidth()
	lines := []string{m.header(width), rule(width)}
	if m.showAnalytics(width) {
		lines = append(lines, m.analyticsLines(width)...)
		lines = append(lines, rule(width))
	}

	workspaceHeight := m.workspaceHeight(len(lines))
	widths := workspaceWidths(width)
	panes := [][]string{
		m.fileTable(widths[0], workspaceHeight),
		m.functionTable(widths[1], workspaceHeight),
		m.sourceLines(widths[2], workspaceHeight, "Source"),
	}
	lines = append(lines, joinedRows(panes, widths)...)
	lines = append(lines, m.footer(width))

	return m.renderFrame(lines)
}

func (m Model) narrowView() string {
	width := max(m.width, 1)
	height := 0
	if m.height > 0 {
		height = max(m.height-2, 2)
	}

	lines := []string{m.header(width)}
	switch m.focus {
	case functionsPane:
		lines = append(lines, m.functionTable(width, height)...)
	case detailsPane:
		lines = append(lines, m.sourceLines(width, height, "Details")...)
	default:
		lines = append(lines, m.fileTable(width, height)...)
	}
	lines = append(lines, m.footer(width))

	return m.renderFrame(lines)
}

func (m Model) terminalWidth() int {
	if m.width > 0 {
		return m.width
	}

	return 120
}

func (m Model) showAnalytics(width int) bool {
	return width >= analyticsMinimum && (m.height == 0 || m.height >= analyticsHeightMinimum)
}

func (m Model) workspaceHeight(linesAbove int) int {
	if m.height > 0 {
		return max(m.height-linesAbove-1, 5)
	}

	return 24
}

func workspaceWidths(width int) []int {
	files := max(width*28/100, 22)
	functions := max(width*31/100-10, 24)
	source := max(width-files-functions-6, 1)

	return []int{files, functions, source}
}

func (m Model) header(width int) string {
	root := m.report.Root
	if root == "" {
		root = strings.Join(m.paths, " ")
	}
	left := green.Bold(true).Render("CYCLO") + "  " + muted.Render(root)
	summary := fmt.Sprintf(
		"%d funcs · cc %.1f · cog %.1f",
		m.report.Functions,
		m.report.Average,
		m.report.CognitiveAverage,
	)
	if m.controlPort > 0 {
		summary += fmt.Sprintf(" · RPC :%d", m.controlPort)
	}
	right := muted.Render(summary)

	return joinSides(left, right, width)
}

func (m Model) footer(width int) string {
	if m.annotating {
		selection := m.lineSelection
		rangeLabel := ""
		if selection != nil {
			rangeLabel = fmt.Sprintf(" %d–%d", selection.StartLine, selection.EndLine)
		}
		keys := amber.Render("Annotate"+rangeLabel+": ") + text.Render(m.annotationDraft+"█") +
			muted.Render("  ·  ") + green.Render("enter") + muted.Render(" save  ·  ") +
			green.Render("esc") + muted.Render(" cancel")
		return truncate(keys, width)
	}
	if width < wideMinimum {
		keys := green.Render("tab") + muted.Render(" panes · ") +
			green.Render("j/k") + muted.Render(" move · ") +
			green.Render(",/.") + muted.Render(" files · ") +
			green.Render("[/]") + muted.Render(" notes · ") +
			green.Render("r") + muted.Render(" · ") + green.Render("q")
		if m.focus == detailsPane {
			keys += muted.Render(" · ") + green.Render("v/a/d") + muted.Render(" source")
		}
		return truncate(keys, width)
	}

	keys := green.Render("tab/shift+tab") + muted.Render(" panes · ") +
		green.Render("j/k") + muted.Render(" move · ") +
		green.Render(",/.") + muted.Render(" files · ") +
		green.Render("[/]") + muted.Render(" notes · ") +
		green.Render("r") + muted.Render(" refresh · ") +
		green.Render("q") + muted.Render(" quit")
	if m.focus == detailsPane {
		keys += muted.Render(" · ") + green.Render("v") + muted.Render(" select · ") +
			green.Render("a") + muted.Render(" note · ") +
			green.Render("d") + muted.Render(" remove")
	}
	return truncate(keys, width)
}

func (m Model) fileTable(width int, height int) []string {
	tableWidth := paneContentWidth(width)
	lines := []string{
		m.paneTitle(fmt.Sprintf("Files · %d", len(m.report.Files)), filesPane),
		muted.Render(fileTableHeader(tableWidth)),
	}
	if m.err != nil {
		return fitPaneLines(append(lines, danger.Render("Error: "+m.err.Error())), width, height)
	}
	if len(m.report.Files) == 0 {
		return fitPaneLines(append(lines, muted.Render("No Go files")), width, height)
	}

	for index, file := range m.report.Files {
		lines = append(lines, m.fileRow(file, tableWidth, index == m.fileIndex))
	}

	return tableWindow(lines, m.fileIndex, width, height)
}

func fileTableHeader(width int) string {
	if width < 30 {
		nameWidth := max(width-10, 1)
		return fmt.Sprintf("  %-*s %3s %3s", nameWidth, "FILE", "CC", "COG")
	}
	if width < 42 {
		nameWidth := max(width-15, 1)
		return fmt.Sprintf("  %-*s %3s %3s %3s", nameWidth, "FILE", "SUM", "MAX", "COG")
	}

	nameWidth := max(width-21, 1)
	return fmt.Sprintf("  %-*s %5s %4s %3s %3s", nameWidth, "FILE", "SUM", "AVG", "MAX", "COG")
}

func (m Model) fileRow(file domain.File, width int, selected bool) string {
	if width < 30 {
		nameWidth := max(width-10, 1)
		path := fileTablePath(m.report.Root, file.Path, nameWidth)
		name := text.Render(padText(path, nameWidth))
		peak := metricStyle(file.Peak).Render(fmt.Sprintf(" %3d", file.Peak))
		load := cognitive.Render(fmt.Sprintf(" %3d", file.CognitivePeak))
		return m.selection(name+peak+load, filesPane, selected, m.fileAnnotationCount(file.Path) > 0)
	}
	if width < 42 {
		nameWidth := max(width-15, 1)
		path := fileTablePath(m.report.Root, file.Path, nameWidth)
		name := text.Render(padText(path, nameWidth))
		total := muted.Render(fmt.Sprintf(" %3d ", file.Total))
		peak := metricStyle(file.Peak).Render(fmt.Sprintf("%3d", file.Peak))
		load := cognitive.Render(fmt.Sprintf(" %3d", file.CognitivePeak))
		return m.selection(name+total+peak+load, filesPane, selected, m.fileAnnotationCount(file.Path) > 0)
	}

	nameWidth := max(width-21, 1)
	path := fileTablePath(m.report.Root, file.Path, nameWidth)
	name := text.Render(padText(path, nameWidth))
	values := muted.Render(fmt.Sprintf(" %5d %4.1f ", file.Total, file.Average))
	peak := metricStyle(file.Peak).Render(fmt.Sprintf("%3d", file.Peak))
	load := cognitive.Render(fmt.Sprintf(" %3d", file.CognitivePeak))

	return m.selection(name+values+peak+load, filesPane, selected, m.fileAnnotationCount(file.Path) > 0)
}

func fileTablePath(root string, path string, width int) string {
	path = displayPath(root, path)
	if ansi.StringWidth(path) <= width {
		return path
	}

	return truncate(filepath.Base(path), width)
}

func (m Model) functionTable(width int, height int) []string {
	file, ok := m.selectedFile()
	tableWidth := paneContentWidth(width)
	lines := []string{
		m.paneTitle("Functions", functionsPane),
		muted.Render(functionTableHeader(tableWidth)),
	}
	if !ok || len(file.Functions) == 0 {
		return fitPaneLines(append(lines, muted.Render("No functions")), width, height)
	}

	for index, function := range file.Functions {
		lines = append(lines, m.functionRow(function, tableWidth, index == m.functionIndex))
	}

	return tableWindow(lines, m.functionIndex, width, height)
}

func functionTableHeader(width int) string {
	nameWidth := max(width-16, 1)
	return fmt.Sprintf("  %-*s %3s %3s %5s", nameWidth, "FUNCTION", "CC", "COG", "LINES")
}

func (m Model) functionRow(function domain.Function, width int, selected bool) string {
	nameWidth := max(width-16, 1)
	count := 0
	if file, ok := m.selectedFile(); ok {
		count = m.functionAnnotationCount(file.Path, function.Line)
	}
	name := text.Render(padText(truncate(function.Name, nameWidth), nameWidth))
	complexity := metricStyle(function.Complexity).Render(fmt.Sprintf(" %3d", function.Complexity))
	load := cognitive.Render(fmt.Sprintf(" %3d", function.CognitiveComplexity))
	lines := muted.Render(fmt.Sprintf(" %5d", functionLineCount(function)))

	return m.selection(name+complexity+load+lines, functionsPane, selected, count > 0)
}

func functionLineCount(function domain.Function) int {
	return max(function.EndLine-function.Line+1, 1)
}

func (m Model) sourceLines(width int, height int, title string) []string {
	contentWidth := paneContentWidth(width)
	titleLine := m.paneTitle(title, detailsPane)
	if count := len(m.visibleAnnotations()); count > 0 {
		noun := "NOTE"
		if count > 1 {
			noun = "NOTES"
		}
		titleLine += muted.Render(" · ") + amber.Bold(true).Render(fmt.Sprintf("◆ %d %s", count, noun))
	}
	file, fileOK := m.selectedFile()
	function, functionOK := m.selectedFunction()
	if functionOK {
		titleLine += diffTitle(m.report.DiffBase, function.DiffLines)
	}
	lines := []string{titleLine}
	if m.annotationError != nil {
		lines = append(lines, danger.Render(m.annotationError.Error()))
	}
	if !fileOK || !functionOK {
		return fitPaneLines(append(lines, muted.Render("No function selected")), width, height)
	}

	lines = append(
		lines,
		blue.Render(sourceLocation(m.report.Root, file.Path, function.Line, function.Column, contentWidth)),
		text.Render(function.Package+" · "+function.Name),
	)
	lines = append(lines, rule(contentWidth))
	code := m.selectedSourceCodeLines(function)
	contentStart := len(lines)
	available := max(height-len(lines), 0)
	if height == 0 {
		available = len(code)
	}
	lines = append(lines, m.sourceDisplayWindow(code, function, contentWidth, available)...)
	rendered := fitPaneLines(lines, width, height)
	if height == 0 {
		return rendered
	}
	position := m.sourceDisplayRowCount(0, m.sourceOffset-1)
	contentLength := m.sourceDisplayRowCount(0, len(code)-1)
	return verticalScrollbar(rendered, contentStart, position, contentLength, available, width)
}

func sourceCodeLines(function domain.Function) []string {
	return renderSourceCodeLines(function, Model{}, false)
}

func (m Model) selectedSourceCodeLines(function domain.Function) []string {
	return renderSourceCodeLines(function, m, true)
}

func renderSourceCodeLines(function domain.Function, model Model, interactive bool) []string {
	if function.Source == "" {
		return []string{muted.Render("Source unavailable")}
	}

	lines := normalizedSourceLines(function.Source)
	successfulReturns := successfulReturnLines(lines)
	result := make([]string, 0, len(lines))
	for index, line := range lines {
		lineNumber := function.Line + index
		numberStyle, styledLine := styledSourceLine(function, lineNumber, line, successfulReturns[index])
		gutter := muted.Render("  ")
		if interactive {
			gutter, numberStyle = model.sourceGutter(lineNumber, numberStyle)
		}
		diff := ""
		if len(function.DiffLines) > 0 {
			diff = diffMarker(function.DiffLines, lineNumber)
		}
		number := numberStyle.Render(fmt.Sprintf("%4d │ ", lineNumber))
		result = append(
			result,
			gutter+diff+number+strings.Repeat(" ", sourceOverlayPadding)+styledLine,
		)
	}

	prefixWidth := sourceLinePrefixWidth
	if len(function.DiffLines) > 0 {
		prefixWidth++
	}
	result = overlayErrorPaths(result, lines, prefixWidth)
	if interactive {
		for index := range result {
			result[index] = model.highlightSourceLine(function.Line+index, result[index])
		}
	}
	return result
}

func styledSourceLine(function domain.Function, lineNumber int, line string, successfulReturn bool) (lipgloss.Style, string) {
	cyclomaticLine := cyclomaticBearingLine(function.CyclomaticDiagnostics, lineNumber)
	cognitiveLine := cognitiveBearingLine(function.CognitiveDiagnostics, lineNumber)
	numberStyle := muted

	switch {
	case errorReturn(line):
		return numberStyle, styledReturn(line, danger, true)
	case successfulReturn:
		return numberStyle, styledReturn(line, green, false)
	case cyclomaticLine && cognitiveLine:
		style := amber.Background(cognitiveBackground)
		return numberStyle, style.Render(line)
	case cyclomaticLine:
		return numberStyle, amber.Render(line)
	case cognitiveLine:
		return numberStyle, text.Background(cognitiveBackground).Render(line)
	default:
		return numberStyle, text.Render(line)
	}
}

func normalizedSourceLines(source string) []string {
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		lines[index] = strings.ReplaceAll(strings.TrimSuffix(line, "\r"), "\t", "    ")
	}
	return lines
}

func (m Model) sourceGutter(line int, numberStyle lipgloss.Style) (string, lipgloss.Style) {
	cursor := m.focus == detailsPane && line == m.sourceLine()
	selected := m.lineSelection != nil && m.lineSelection.StartLine <= line && line <= m.lineSelection.EndLine
	marker := " "
	if selected {
		marker = "│"
		numberStyle = amber
	}
	if cursor {
		marker = "›"
		numberStyle = green
	}

	return green.Render(marker) + " ", numberStyle
}

func (m Model) sourceDisplayWindow(lines []string, function domain.Function, width int, height int) []string {
	if height <= 0 || len(lines) == 0 {
		return nil
	}

	result := make([]string, 0, height)
	for index := min(m.sourceOffset, len(lines)); index < len(lines) && len(result) < height; index++ {
		lineNumber := function.Line + index
		rows := m.sourceRowsAtLine(function, lineNumber, lines[index], width, index == len(lines)-1)
		remaining := height - len(result)
		if len(rows) > remaining {
			return append(result, rows[:remaining]...)
		}
		result = append(result, rows...)
	}
	return result
}

func (m Model) sourceRowsAtLine(
	function domain.Function,
	lineNumber int,
	source string,
	width int,
	last bool,
) []string {
	rows := sourceDeletedRows(function.DiffLines, lineNumber)
	rows = append(rows, source)
	rows = append(rows, m.sourceAnnotationRows(lineNumber, width)...)
	if last {
		rows = append(rows, sourceDeletedRows(function.DiffLines, function.EndLine+1)...)
	}
	return rows
}

func diffTitle(base string, lines []domain.DiffLine) string {
	added, deleted := diffCounts(lines)
	if added == 0 && deleted == 0 {
		return ""
	}
	return muted.Render(" · diff "+base+" · ") + green.Bold(true).Render(fmt.Sprintf("+%d", added)) +
		muted.Render(" ") + danger.Bold(true).Render(fmt.Sprintf("−%d", deleted))
}

func diffCounts(lines []domain.DiffLine) (int, int) {
	added, deleted := 0, 0
	for _, line := range lines {
		if line.Kind == domain.DiffAdded {
			added++
		}
		if line.Kind == domain.DiffDeleted {
			deleted++
		}
	}
	return added, deleted
}

func diffMarker(lines []domain.DiffLine, lineNumber int) string {
	for _, line := range lines {
		if line.Kind == domain.DiffAdded && line.NewLine == lineNumber {
			return green.Bold(true).Render("+")
		}
	}
	return muted.Render(" ")
}

func sourceDeletedRows(lines []domain.DiffLine, beforeLine int) []string {
	result := make([]string, 0)
	for _, line := range lines {
		if line.Kind != domain.DiffDeleted || line.NewLine != beforeLine {
			continue
		}
		body := strings.ReplaceAll(strings.TrimSuffix(line.Text, "\r"), "\t", "    ")
		prefix := muted.Render("  ") + danger.Bold(true).Render("−") +
			muted.Render(fmt.Sprintf("%4d │ ", line.OldLine))
		result = append(result, prefix+strings.Repeat(" ", sourceOverlayPadding)+danger.Render(body))
	}
	return result
}

func (m Model) sourceAnnotationRows(line int, width int) []string {
	annotations := m.visibleAnnotations()
	rows := make([]string, 0)
	for index, annotation := range annotations {
		if annotation.EndLine != line {
			continue
		}
		position := ""
		if len(annotations) > 1 {
			position = fmt.Sprintf("%d/%d ", index+1, len(annotations))
		}
		prefix := muted.Render("      ╰─") + amber.Bold(true).Render("◆ "+position)
		messageWidth := max(width-ansi.StringWidth(prefix), 1)
		messages := strings.Split(ansi.Wrap(annotation.Message, messageWidth, ""), "\n")
		for messageIndex, message := range messages {
			indent := strings.Repeat(" ", ansi.StringWidth(prefix))
			if messageIndex == 0 {
				indent = prefix
			}
			rows = append(rows, indent+amber.Render(message))
		}
	}
	return rows
}

func (m Model) highlightSourceLine(line int, rendered string) string {
	if m.lineSelection != nil && m.lineSelection.StartLine <= line && line <= m.lineSelection.EndLine {
		return withBackground(rendered, "\x1b[48;2;45;52;54m")
	}
	for _, annotation := range m.visibleAnnotations() {
		if annotation.StartLine <= line && line <= annotation.EndLine {
			return withBackground(rendered, "\x1b[48;2;44;34;14m")
		}
	}
	return rendered
}

func withBackground(rendered string, background string) string {
	rendered = strings.ReplaceAll(rendered, "\x1b[m", "\x1b[m"+background)
	return background + rendered + "\x1b[m"
}

func (m Model) sourceViewportHeight() int {
	if m.height == 0 {
		return len(m.selectedSourceLines())
	}
	if m.width > 0 && m.width < wideMinimum {
		return max(max(m.height-2, 2)-m.sourceHeaderHeight(), 0)
	}

	width := m.terminalWidth()
	linesAbove := 2
	if m.showAnalytics(width) {
		linesAbove += len(m.analyticsLines(width)) + 1
	}
	return max(m.workspaceHeight(linesAbove)-m.sourceHeaderHeight(), 0)
}

func (m Model) sourcePaneWidth() int {
	if m.width > 0 && m.width < wideMinimum {
		return max(m.width, 1)
	}

	return workspaceWidths(m.terminalWidth())[2]
}

func (m Model) sourceHeaderHeight() int {
	height := 4
	if m.annotationError != nil {
		height++
	}
	return height
}

func styledReturn(line string, style lipgloss.Style, finalValueOnly bool) string {
	valuesStart := returnedValuesStart(line)
	start := valuesStart
	end := len(line)
	if finalValueOnly {
		start = finalReturnedValueStart(line, valuesStart)
	} else {
		start = strings.Index(line, "return")
		end = successfulReturnedValuesEnd(line, valuesStart)
	}
	if start >= end {
		return text.Render(line)
	}

	result := text.Render(line[:start]) + style.Render(line[start:end])
	if end < len(line) {
		result += text.Render(line[end:])
	}

	return result
}

func returnedValuesStart(line string) int {
	start := strings.Index(line, "return") + len("return")
	for start < len(line) && line[start] == ' ' {
		start++
	}

	return start
}

func finalReturnedValueStart(line string, start int) int {
	depth := 0
	for index := len(line) - 1; index >= start; index-- {
		switch line[index] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			depth--
		case ',':
			if depth == 0 {
				return returnedValueAfter(line, index)
			}
		}
	}

	return start
}

func successfulReturnedValuesEnd(line string, start int) int {
	lastValue := finalReturnedValueStart(line, start)
	if strings.TrimSpace(line[lastValue:]) != "nil" {
		return len(line)
	}
	if lastValue == start {
		return start
	}

	return strings.LastIndex(line[:lastValue], ",")
}

func returnedValueAfter(line string, comma int) int {
	start := comma + 1
	for start < len(line) && line[start] == ' ' {
		start++
	}

	return start
}

func successfulReturnLines(lines []string) []bool {
	result := make([]bool, len(lines))
	for index, line := range lines {
		result[index] = returnLine(line) && !errorReturn(line)
	}

	return result
}

func returnLine(line string) bool {
	line = strings.TrimSpace(line)
	return line == "return" || strings.HasPrefix(line, "return ")
}

func errorReturn(line string) bool {
	if !returnLine(line) {
		return false
	}

	for _, marker := range []string{" err", "Err", "Error", "errors."} {
		if strings.Contains(line, marker) {
			return true
		}
	}

	return false
}

func splitIndent(line string) (string, string) {
	body := strings.TrimLeft(line, " ")
	return line[:len(line)-len(body)], body
}

func cyclomaticBearingLine(diagnostics []domain.CyclomaticDiagnostic, line int) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Line == line {
			return true
		}
	}

	return false
}

func cognitiveBearingLine(diagnostics []domain.CognitiveDiagnostic, line int) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Line == line {
			return true
		}
	}

	return false
}

func (m Model) paneTitle(label string, target pane) string {
	if m.focus == target {
		return green.Bold(true).Render(label)
	}

	return blue.Render(label)
}

func (m Model) selection(label string, target pane, selected bool, annotated bool) string {
	if !selected {
		if annotated {
			return amber.Render("◆ ") + label
		}
		return "  " + label
	}

	marker := blue.Bold(true)
	background := "\x1b[48;2;20;31;34m"
	if m.focus == target {
		marker = green.Bold(true)
		background = "\x1b[48;2;20;40;30m"
	}
	prefix := marker.Render("› ")
	if annotated {
		prefix = marker.Render("›") + amber.Render("◆")
	}
	return withBackground(prefix+label, background)
}

func tableWindow(lines []string, selected int, width int, height int) []string {
	if height == 0 || len(lines) <= height {
		return fitPaneLines(lines, width, height)
	}

	data := lines[2:]
	available := max(height-2, 0)
	start := min(max(selected-available/2, 0), max(len(data)-available, 0))
	end := min(start+available, len(data))
	visible := append([]string{}, lines[:2]...)
	visible = append(visible, data[start:end]...)

	return verticalScrollbar(fitPaneLines(visible, width, height), 2, start, len(data), available, width)
}

func paneContentWidth(width int) int {
	return max(width-paneRightInset(width), 1)
}

const scrollbarGutterWidth = 2

func paneRightInset(width int) int {
	if width < 30 {
		return 1
	}
	return scrollbarGutterWidth
}

func verticalScrollbar(
	lines []string,
	contentStart int,
	position int,
	contentLength int,
	viewportLength int,
	width int,
) []string {
	if contentStart >= len(lines) {
		return lines
	}

	trackHeight := min(viewportLength, len(lines)-contentStart)
	thumbStart, thumbHeight := scrollbarThumb(trackHeight, position, contentLength, trackHeight)
	if thumbHeight == 0 {
		return lines
	}

	gutterWidth := paneRightInset(width)
	contentWidth := max(width-gutterWidth, 0)
	for row := range trackHeight {
		index := contentStart + row
		gutter := strings.Repeat(" ", gutterWidth)
		if thumbStart <= row && row < thumbStart+thumbHeight {
			gutter = strings.Repeat(" ", gutterWidth-1) + muted.Render("█")
		}
		lines[index] = styledCell(lines[index], contentWidth) + gutter
	}
	return lines
}

func scrollbarThumb(
	trackHeight int,
	position int,
	contentLength int,
	viewportLength int,
) (int, int) {
	if trackHeight <= 0 || contentLength <= 0 || viewportLength >= contentLength {
		return 0, 0
	}

	height := max(min(trackHeight*viewportLength/contentLength, trackHeight), 1)
	maximumScroll := contentLength - viewportLength
	maximumStart := trackHeight - height
	start := (min(position, maximumScroll)*maximumStart + maximumScroll/2) / maximumScroll
	return start, height
}

func joinedRows(panes [][]string, widths []int) []string {
	rows := 0
	for _, pane := range panes {
		rows = max(rows, len(pane))
	}

	result := make([]string, 0, rows)
	for row := 0; row < rows; row++ {
		cells := make([]string, len(panes))
		for column, pane := range panes {
			value := ""
			if row < len(pane) {
				value = pane[row]
			}
			cells[column] = styledCell(value, widths[column])
		}
		result = append(result, strings.Join(cells, " │ "))
	}

	return result
}

func fitPaneLines(lines []string, width int, height int) []string {
	lines = fitLines(lines, paneContentWidth(width), height)
	return fitLines(lines, width, height)
}

func fitLines(lines []string, width int, height int) []string {
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	result := make([]string, 0, max(len(lines), height))
	for _, line := range lines {
		result = append(result, styledCell(line, width))
	}
	for height > 0 && len(result) < height {
		result = append(result, strings.Repeat(" ", width))
	}

	return result
}

func (m Model) renderFrame(lines []string) string {
	if m.height <= 0 || len(lines) <= m.height {
		return strings.Join(lines, "\n")
	}

	footerLine := lines[len(lines)-1]
	lines = append(lines[:m.height-1], footerLine)
	return strings.Join(lines, "\n")
}

func styledCell(value string, width int) string {
	value = truncate(value, width)
	padding := strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
	return value + padding
}

func rule(width int) string {
	return border.Render(strings.Repeat("─", max(width, 0)))
}

func joinSides(left string, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		leftWidth := max(width-ansi.StringWidth(right)-1, 0)
		if leftWidth == 0 {
			return truncate(right, width)
		}
		return truncate(left, leftWidth) + " " + right
	}

	return left + strings.Repeat(" ", gap) + right
}

func padText(value string, width int) string {
	return value + strings.Repeat(" ", max(width-ansi.StringWidth(value), 0))
}

func metricStyle(value int) lipgloss.Style {
	if value > 10 {
		return danger
	}
	if value > 5 {
		return amber
	}

	return green
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}

	return ansi.Truncate(value, width, "")
}

func displayPath(root string, path string) string {
	if root == "" || !filepath.IsAbs(path) {
		return path
	}

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}

	return relative
}

func sourceLocation(root string, path string, line int, column int, width int) string {
	path = displayPath(root, path)
	coordinates := fmt.Sprintf(":%d:%d", line, column)
	if ansi.StringWidth(path+coordinates) <= width {
		return path + coordinates
	}

	nameWidth := max(width-ansi.StringWidth(coordinates), 0)
	return truncate(filepath.Base(path), nameWidth) + coordinates
}
