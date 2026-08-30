package application

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclomatic-complexity-tui/domain"
)

const maximumAnnotationLength = 160

type LineSelection struct {
	AnchorLine int    `json:"anchorLine"`
	StartLine  int    `json:"startLine"`
	EndLine    int    `json:"endLine"`
	Text       string `json:"text"`
}

type Annotation = domain.Annotation

type AnnotationStore interface {
	ListAnnotations(repository string) ([]domain.Annotation, error)
	SaveAnnotation(repository string, annotation domain.Annotation) error
	DeleteAnnotation(repository string, id string) error
}

func (m Model) resetSourceWorkspace() Model {
	m.sourceOffset = 0
	m.sourceCursor = 0
	m.lineSelection = nil
	m.visualSelectionActive = false
	m.activeAnnotationID = ""
	m.annotating = false
	m.annotationDraft = ""
	return m
}

func (m Model) moveSourceCursor(delta int) Model {
	lines := m.selectedSourceLines()
	if len(lines) == 0 {
		return m
	}

	m.sourceCursor = moveIndex(m.sourceCursor, delta, len(lines))
	m = m.keepSourceCursorVisible()
	if m.visualSelectionActive {
		m = m.selectFromAnchor(m.lineSelection.AnchorLine)
	}
	m.activeAnnotationID = ""
	return m
}

func (m Model) keepSourceCursorVisible() Model {
	visible := m.sourceViewportHeight()
	if visible <= 0 || m.sourceCursor < m.sourceOffset {
		m.sourceOffset = m.sourceCursor
		return m
	}
	for m.sourceOffset < m.sourceCursor && m.sourceDisplayRowCount(m.sourceOffset, m.sourceCursor) > visible {
		m.sourceOffset++
	}
	return m
}

func (m Model) sourceDisplayRowCount(start int, end int) int {
	if end < start {
		return 0
	}

	count := end - start + 1
	function, ok := m.selectedFunction()
	if !ok {
		return count
	}
	width := m.sourcePaneWidth()
	for index := start; index <= end; index++ {
		lineNumber := function.Line + index
		count += len(sourceDeletedRows(function.DiffLines, lineNumber))
		count += len(m.sourceAnnotationRows(lineNumber, width))
		if index == len(m.selectedSourceLines())-1 {
			count += len(sourceDeletedRows(function.DiffLines, function.EndLine+1))
		}
	}
	return count
}

func (m Model) toggleVisualSelection() Model {
	if len(m.selectedSourceLines()) == 0 {
		return m
	}
	if m.visualSelectionActive {
		return m.clearLineSelection()
	}

	m.visualSelectionActive = true
	m = m.selectFromAnchor(m.sourceLine())
	m.revision++
	return m
}

func (m Model) selectFromAnchor(anchor int) Model {
	start, end := min(anchor, m.sourceLine()), max(anchor, m.sourceLine())
	m.lineSelection = &LineSelection{
		AnchorLine: anchor,
		StartLine:  start,
		EndLine:    end,
		Text:       m.sourceRangeText(start, end),
	}
	return m
}

func (m Model) clearLineSelection() Model {
	if m.lineSelection == nil && !m.visualSelectionActive && m.activeAnnotationID == "" {
		return m
	}
	m.lineSelection = nil
	m.visualSelectionActive = false
	m.activeAnnotationID = ""
	m.revision++
	return m
}

func (m Model) startAnnotating() Model {
	if !m.visualSelectionActive || m.lineSelection == nil {
		return m
	}
	m.annotating = true
	m.annotationDraft = ""
	m.revision++
	return m
}

func (m Model) updateAnnotationInput(message tea.KeyPressMsg) Model {
	switch message.Keystroke() {
	case "esc":
		m.annotating = false
		m.annotationDraft = ""
	case "enter":
		m = m.saveDraftAnnotation()
	case "backspace":
		m.annotationDraft = trimLastRune(m.annotationDraft)
	default:
		if message.Text != "" && len([]rune(m.annotationDraft+message.Text)) <= maximumAnnotationLength {
			m.annotationDraft += message.Text
		}
	}
	m.revision++
	return m
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func (m Model) saveDraftAnnotation() Model {
	message := strings.TrimSpace(m.annotationDraft)
	if message == "" || m.lineSelection == nil {
		return m
	}
	annotation, ok := m.newAnnotation(m.lineSelection.StartLine, m.lineSelection.EndLine, message)
	if !ok {
		return m
	}
	var err error
	m, err = m.saveAnnotation(annotation)
	if err != nil {
		m.annotationError = err
		m.annotating = false
		return m
	}
	m.activeAnnotationID = annotation.ID
	m.visualSelectionActive = false
	m.lineSelection = nil
	m.annotating = false
	m.annotationDraft = ""
	return m.keepSourceCursorVisible()
}

func (m Model) saveAnnotation(annotation Annotation) (Model, error) {
	if m.annotationStore != nil {
		err := m.annotationStore.SaveAnnotation(m.report.Root, annotation)
		if err != nil {
			return m, fmt.Errorf("save annotation: %w", err)
		}
	}
	m.annotations = append(m.annotations, annotation)
	m.annotationError = nil
	return m, nil
}

func (m Model) loadAnnotations() Model {
	if m.annotationStore == nil {
		return m
	}

	annotations, err := m.annotationStore.ListAnnotations(m.report.Root)
	if err != nil {
		m.annotationError = fmt.Errorf("load annotations: %w", err)
		return m
	}
	m.annotations = annotations
	m.nextAnnotationID = nextAnnotationSequence(annotations)
	m.annotationError = nil
	return m
}

func nextAnnotationSequence(annotations []Annotation) int {
	next := 0
	for _, annotation := range annotations {
		value := strings.TrimPrefix(annotation.ID, "annotation-")
		sequence, err := strconv.Atoi(value)
		if err == nil {
			next = max(next, sequence)
		}
	}
	return next
}

func (m *Model) newAnnotation(startLine int, endLine int, message string) (Annotation, bool) {
	file, fileOK := m.selectedFile()
	function, functionOK := m.selectedFunction()
	if !fileOK || !functionOK || !m.validSourceRange(startLine, endLine) {
		return Annotation{}, false
	}
	m.nextAnnotationID++
	return Annotation{
		ID:           fmt.Sprintf("annotation-%d", m.nextAnnotationID),
		Path:         file.Path,
		Function:     function.Name,
		FunctionLine: function.Line,
		StartLine:    startLine,
		EndLine:      endLine,
		Message:      message,
		Text:         m.sourceRangeText(startLine, endLine),
	}, true
}

func (m Model) validSourceRange(startLine int, endLine int) bool {
	function, ok := m.selectedFunction()
	if !ok || startLine > endLine {
		return false
	}
	lastLine := function.Line + len(m.selectedSourceLines()) - 1
	return startLine >= function.Line && endLine <= lastLine
}

func (m Model) sourceRangeText(startLine int, endLine int) string {
	function, ok := m.selectedFunction()
	if !ok || !m.validSourceRange(startLine, endLine) {
		return ""
	}
	lines := m.selectedSourceLines()
	return strings.Join(lines[startLine-function.Line:endLine-function.Line+1], "\n")
}

func (m Model) selectedSourceLines() []string {
	function, ok := m.selectedFunction()
	if !ok || function.Source == "" {
		return nil
	}
	return normalizedSourceLines(function.Source)
}

func (m Model) sourceLine() int {
	function, ok := m.selectedFunction()
	if !ok {
		return 0
	}
	return function.Line + m.sourceCursor
}

func (m Model) visibleAnnotations() []Annotation {
	file, fileOK := m.selectedFile()
	function, functionOK := m.selectedFunction()
	if !fileOK || !functionOK {
		return nil
	}

	result := make([]Annotation, 0)
	for _, annotation := range m.annotations {
		if annotation.Path == file.Path && annotation.FunctionLine == function.Line {
			result = append(result, annotation)
		}
	}
	sort.SliceStable(result, func(left int, right int) bool {
		if result[left].StartLine != result[right].StartLine {
			return result[left].StartLine < result[right].StartLine
		}
		return result[left].EndLine < result[right].EndLine
	})
	return result
}

func (m Model) fileAnnotationCount(path string) int {
	count := 0
	for _, annotation := range m.annotations {
		if annotation.Path == path {
			count++
		}
	}
	return count
}

func (m Model) functionAnnotationCount(path string, line int) int {
	count := 0
	for _, annotation := range m.annotations {
		if annotation.Path == path && annotation.FunctionLine == line {
			count++
		}
	}
	return count
}

func (m Model) annotationAtCursor() (Annotation, bool) {
	annotations := m.visibleAnnotations()
	for index := len(annotations) - 1; index >= 0; index-- {
		annotation := annotations[index]
		if annotation.StartLine <= m.sourceLine() && m.sourceLine() <= annotation.EndLine {
			return annotation, true
		}
	}
	return Annotation{}, false
}

func (m Model) removeAnnotationAtCursor() Model {
	annotation, ok := m.annotationAtCursor()
	if !ok {
		return m
	}

	next, err := m.removeAnnotation(annotation.ID)
	if err != nil {
		m.annotationError = err
		return m
	}
	return next
}

func (m Model) removeAnnotation(id string) (Model, error) {
	for index, annotation := range m.annotations {
		if annotation.ID != id {
			continue
		}
		if m.annotationStore != nil {
			err := m.annotationStore.DeleteAnnotation(m.report.Root, id)
			if err != nil {
				return m, fmt.Errorf("delete annotation: %w", err)
			}
		}
		m.annotations = append(m.annotations[:index], m.annotations[index+1:]...)
		if m.activeAnnotationID == id {
			m.activeAnnotationID = ""
		}
		m.annotationError = nil
		m.revision++
		return m, nil
	}
	return m, nil
}

func (m Model) focusAdjacentAnnotation(delta int) Model {
	targets := m.annotationTargets()
	if len(targets) == 0 {
		return m
	}

	index := m.adjacentAnnotationTargetIndex(targets, delta)
	target := targets[index]
	annotation := target.Annotation
	m.fileIndex = target.fileIndex
	m.functionIndex = target.functionIndex
	m = m.resetSourceWorkspace()
	m.focus = detailsPane
	m.sourceCursor = annotation.EndLine - annotation.FunctionLine
	m.activeAnnotationID = annotation.ID
	m = m.keepSourceCursorVisible()
	m.revision++
	return m
}

type annotationTarget struct {
	Annotation
	fileIndex     int
	functionIndex int
}

func (m Model) annotationTargets() []annotationTarget {
	targets := make([]annotationTarget, 0, len(m.annotations))
	for fileIndex, file := range m.report.Files {
		for functionIndex, function := range file.Functions {
			for _, annotation := range m.annotations {
				if annotation.Path != file.Path || annotation.FunctionLine != function.Line {
					continue
				}
				targets = append(targets, annotationTarget{
					Annotation: annotation, fileIndex: fileIndex, functionIndex: functionIndex,
				})
			}
		}
	}
	sort.SliceStable(targets, func(left int, right int) bool {
		return annotationTargetBefore(targets[left], targets[right])
	})
	return targets
}

func annotationTargetBefore(left annotationTarget, right annotationTarget) bool {
	if left.fileIndex != right.fileIndex {
		return left.fileIndex < right.fileIndex
	}
	if left.functionIndex != right.functionIndex {
		return left.functionIndex < right.functionIndex
	}
	if left.StartLine != right.StartLine {
		return left.StartLine < right.StartLine
	}
	return left.EndLine < right.EndLine
}

func (m Model) adjacentAnnotationTargetIndex(targets []annotationTarget, delta int) int {
	for index, target := range targets {
		if target.ID == m.activeAnnotationID {
			return (index + delta + len(targets)) % len(targets)
		}
	}
	if delta > 0 {
		for index, target := range targets {
			if m.annotationTargetAfterCursor(target) {
				return index
			}
		}
		return 0
	}
	for index := len(targets) - 1; index >= 0; index-- {
		if m.annotationTargetBeforeCursor(targets[index]) {
			return index
		}
	}
	return len(targets) - 1
}

func (m Model) annotationTargetAfterCursor(target annotationTarget) bool {
	if target.fileIndex != m.fileIndex {
		return target.fileIndex > m.fileIndex
	}
	if target.functionIndex != m.functionIndex {
		return target.functionIndex > m.functionIndex
	}
	return target.EndLine >= m.sourceLine()
}

func (m Model) annotationTargetBeforeCursor(target annotationTarget) bool {
	if target.fileIndex != m.fileIndex {
		return target.fileIndex < m.fileIndex
	}
	if target.functionIndex != m.functionIndex {
		return target.functionIndex < m.functionIndex
	}
	return target.StartLine <= m.sourceLine()
}
