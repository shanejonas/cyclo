package application

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/domain"
)

const maximumAnnotationLength = 160

type LineSelection struct {
	AnchorLine int    `json:"anchorLine"`
	StartLine  int    `json:"startLine"`
	EndLine    int    `json:"endLine"`
	Text       string `json:"text"`
}

func (s *LineSelection) contains(line int) bool {
	return s != nil && s.StartLine <= line && line <= s.EndLine
}

type Annotation = domain.Annotation

type AnnotationStore interface {
	ListAnnotations(repository string) ([]domain.Annotation, error)
	SaveAnnotation(repository string, annotation domain.Annotation) error
	DeleteAnnotation(repository string, id string) error
}

func (m Model) resetSourceWorkspace() Model {
	m.sourceWorkspace = sourceWorkspace{}
	return m
}

// showDetails focuses the details pane.
func (m Model) showDetails() Model {
	m.focus = detailsPane
	m.qualityView = false
	return m
}

func (m Model) moveSourceCursor(delta int) Model {
	if annotation, ok := m.unmatchedAnnotation(); ok {
		return m.scrollSavedAnnotation(annotation, delta)
	}
	lineCount := m.selectedSourceLineCount()
	if lineCount == 0 {
		return m
	}

	m.sourceCursor = moveIndex(m.sourceCursor, delta, lineCount)
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
	counter := m.sourceRowCounter()
	rows := counter.count(m.sourceOffset, m.sourceCursor)
	for m.sourceOffset < m.sourceCursor && rows > visible {
		rows -= counter.count(m.sourceOffset, m.sourceOffset)
		m.sourceOffset++
	}
	return m
}

func (m Model) sourceDisplayRowCount(start int, end int) int {
	return m.sourceRowCounter().count(start, end)
}

// sourceRowCounter snapshots the selected source and wrapping width for one
// measurement operation. It does not persist derived state on the model.
type sourceRowCounter struct {
	function        domain.Function
	annotations     []Annotation
	width           int
	lastSourceIndex int
}

func (m Model) sourceRowCounter() sourceRowCounter {
	function, _ := m.selectedFunction()
	return sourceRowCounter{
		function:        function,
		annotations:     m.visibleAnnotations(),
		width:           paneContentWidth(m.sourcePaneWidth()),
		lastSourceIndex: sourceLineCount(function.Source) - 1,
	}
}

func (c sourceRowCounter) count(start int, end int) int {
	if end < start {
		return 0
	}

	count := end - start + 1
	firstLine, lastLine := c.function.Line+start, c.function.Line+end
	count += sourceDeletedRowCount(c.function.DiffLines, firstLine, lastLine)
	count += sourceAnnotationRowCount(c.annotations, firstLine, lastLine, c.width)
	if start <= c.lastSourceIndex && c.lastSourceIndex <= end {
		count += sourceDeletedRowCount(c.function.DiffLines, c.function.EndLine+1, c.function.EndLine+1)
	}
	return count
}

func sourceDeletedRowCount(lines []domain.DiffLine, start int, end int) int {
	count := 0
	for _, line := range lines {
		if line.Kind != domain.DiffDeleted || line.NewLine < start || line.NewLine > end {
			continue
		}
		count++
	}
	return count
}

func sourceAnnotationRowCount(annotations []Annotation, start int, end int, width int) int {
	count := 0
	for index, annotation := range annotations {
		if annotation.EndLine < start || annotation.EndLine > end {
			continue
		}
		prefix := sourceAnnotationPrefix(index, len(annotations))
		messageWidth := max(width-ansi.StringWidth(prefix), 1)
		count += strings.Count(ansi.Wrap(annotation.Message, messageWidth, ""), "\n") + 1
	}
	return count
}

func (m Model) toggleVisualSelection() Model {
	if m.selectedSourceLineCount() == 0 {
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
	m.sourceWorkspace = m.sourceWorkspace.clearSelection()
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
		m.sourceWorkspace = m.sourceWorkspace.cancelDraft()
	case "enter":
		m = m.saveDraftAnnotation()
	case "backspace":
		m.sourceWorkspace = m.sourceWorkspace.trimDraft()
	default:
		m.sourceWorkspace = m.sourceWorkspace.typeDraft(message.Text)
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
	next, err := m.saveAnnotation(annotation)
	next.sourceWorkspace = next.sourceWorkspace.stopAnnotating()
	if err != nil {
		next.annotationError = err
		return next
	}
	next.sourceWorkspace = next.sourceWorkspace.finishDraft(annotation.ID)
	return next.keepSourceCursorVisible()
}

func (m Model) saveAnnotation(annotation Annotation) (Model, error) {
	if m.annotationStore != nil {
		err := m.annotationStore.SaveAnnotation(m.report.Root, annotation)
		if err != nil {
			return m, fmt.Errorf("save annotation: %w", err)
		}
	}
	m.annotations = slices.Concat(m.annotations, []Annotation{annotation})
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
	if !ok || function.Source == "" || startLine > endLine {
		return false
	}
	lastLine := function.Line + strings.Count(function.Source, "\n")
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

func (m Model) selectedSourceLineCount() int {
	function, _ := m.selectedFunction()
	return sourceLineCount(function.Source)
}

func sourceLineCount(source string) int {
	if source == "" {
		return 0
	}
	return strings.Count(source, "\n") + 1
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
		if annotation.Path == file.Path && annotationMatchesFunction(annotation, function) {
			result = append(result, annotation)
		}
	}
	sort.SliceStable(result, func(left int, right int) bool {
		return annotationBefore(result[left], result[right])
	})
	return result
}

func annotationBefore(left Annotation, right Annotation) bool {
	if left.StartLine != right.StartLine {
		return left.StartLine < right.StartLine
	}
	return left.EndLine < right.EndLine
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

func (m Model) functionAnnotationCount(path string, function domain.Function) int {
	count := 0
	for _, annotation := range m.annotations {
		if annotation.Path == path && annotationMatchesFunction(annotation, function) {
			count++
		}
	}
	return count
}

func (m Model) annotationAtCursor() (Annotation, bool) {
	annotations := m.visibleAnnotations()
	line := m.sourceLine()
	for index := len(annotations) - 1; index >= 0; index-- {
		annotation := annotations[index]
		if annotation.StartLine <= line && line <= annotation.EndLine {
			return annotation, true
		}
	}
	return Annotation{}, false
}

func (m Model) removeAnnotationAtCursor() Model {
	annotation, ok := m.unmatchedAnnotation()
	if !ok {
		annotation, ok = m.annotationAtCursor()
	}
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
		m.annotations = slices.Concat(m.annotations[:index], m.annotations[index+1:])
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
	m = m.resetSourceWorkspace().showDetails()
	m.sourceWorkspace = m.sourceWorkspace.focusAnnotation(annotation, target.functionIndex >= 0)
	m.revision++
	if target.functionIndex < 0 {
		return m
	}
	return m.keepSourceCursorVisible()
}

type annotationTarget struct {
	Annotation
	fileIndex     int
	functionIndex int
}

func (m Model) annotationTargets() []annotationTarget {
	targets := make([]annotationTarget, 0, len(m.annotations))
	for _, annotation := range m.annotations {
		targets = append(targets, m.annotationTarget(annotation))
	}
	sort.SliceStable(targets, func(left int, right int) bool {
		return annotationTargetBefore(targets[left], targets[right])
	})
	return targets
}

func (m Model) annotationTarget(annotation Annotation) annotationTarget {
	target := annotationTarget{Annotation: annotation, fileIndex: len(m.report.Files), functionIndex: -1}
	for fileIndex, file := range m.report.Files {
		if file.Path != annotation.Path {
			continue
		}
		target.fileIndex = fileIndex
		for functionIndex, function := range file.Functions {
			if annotationMatchesFunction(annotation, function) {
				target.functionIndex = functionIndex
				return target
			}
		}
	}
	return target
}

func annotationTargetBefore(left annotationTarget, right annotationTarget) bool {
	if left.fileIndex != right.fileIndex {
		return left.fileIndex < right.fileIndex
	}
	if left.functionIndex != right.functionIndex {
		return left.functionIndex < right.functionIndex
	}
	return annotationBefore(left.Annotation, right.Annotation)
}

func (m Model) adjacentAnnotationTargetIndex(targets []annotationTarget, delta int) int {
	for index, target := range targets {
		if target.ID == m.activeAnnotationID {
			return (index + delta + len(targets)) % len(targets)
		}
	}
	if delta > 0 {
		return m.nextAnnotationTargetIndex(targets)
	}
	return m.previousAnnotationTargetIndex(targets)
}

func (m Model) nextAnnotationTargetIndex(targets []annotationTarget) int {
	for index, target := range targets {
		if m.annotationTargetAfterCursor(target) {
			return index
		}
	}
	return 0
}

func (m Model) previousAnnotationTargetIndex(targets []annotationTarget) int {
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
	if target.functionIndex < 0 {
		return true
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
	if target.functionIndex < 0 {
		return true
	}
	if target.functionIndex != m.functionIndex {
		return target.functionIndex < m.functionIndex
	}
	return target.StartLine <= m.sourceLine()
}
