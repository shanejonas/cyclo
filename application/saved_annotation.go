package application

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m Model) unmatchedAnnotation() (Annotation, bool) {
	if m.functionIndex >= 0 {
		return Annotation{}, false
	}
	for _, annotation := range m.annotations {
		if annotation.ID == m.activeAnnotationID {
			return annotation, true
		}
	}
	return Annotation{}, false
}

func savedAnnotationRows(annotation Annotation, width int) []string {
	rows := savedAnnotationMessageRows(annotation.Message, width)
	for index, line := range normalizedSourceLines(annotation.Text) {
		rows = append(rows, muted.Render(fmt.Sprintf("%4d │ ", annotation.StartLine+index))+text.Render(line))
	}
	return rows
}

func savedAnnotationMessageRows(message string, width int) []string {
	wrapped := ansi.Wrap("◆ "+message, max(width, 1), "")
	return strings.Split(amber.Render(wrapped)+"\n", "\n")
}

func savedAnnotationRowCount(annotation Annotation, width int) int {
	return len(savedAnnotationMessageRows(annotation.Message, width)) + strings.Count(annotation.Text, "\n") + 1
}

func (m Model) savedAnnotationLines(annotation Annotation, width int, height int, title string) []string {
	contentWidth := paneContentWidth(width)
	lines := []string{m.paneTitle(title, detailsPane) + amber.Render(" · unmatched")}
	if m.annotationError != nil {
		lines = append(lines, danger.Render(m.annotationError.Error()))
	}
	lines = append(lines,
		blue.Render(m.sourceLocation(annotation.Path, annotation.StartLine, 1, contentWidth)),
		text.Render(annotation.Function+" · saved code"),
		rule(contentWidth),
	)
	rows := savedAnnotationRows(annotation, contentWidth)
	available := max(height-len(lines), 0)
	if height == 0 {
		available = len(rows)
	}
	start := min(m.sourceOffset, max(len(rows)-available, 0))
	contentStart := len(lines)
	lines = append(lines, rows[start:min(start+available, len(rows))]...)
	lines = fitPaneLines(lines, width, height)
	return verticalScrollbar(lines, contentStart, start, len(rows), available, width)
}

func (m Model) scrollSavedAnnotation(annotation Annotation, delta int) Model {
	rowCount := savedAnnotationRowCount(annotation, paneContentWidth(m.sourcePaneWidth()))
	visible := m.sourceViewportHeight()
	if m.height == 0 {
		visible = rowCount
	}
	maximum := max(rowCount-visible, 0)
	m.sourceOffset = moveIndex(m.sourceOffset, delta, maximum+1)
	return m
}
