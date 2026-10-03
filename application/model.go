package application

import (
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/domain"
)

type Analyzer interface {
	Analyze(paths []string) (domain.Report, error)
}

type pane int

const (
	filesPane pane = iota
	functionsPane
	detailsPane
)

type reportMsg struct {
	report domain.Report
	err    error
}

type changeWaiter struct {
	afterRevision uint64
	reply         chan controlReply
}

type changeWaiterTimeoutMsg struct {
	reply chan controlReply
}

type Model struct {
	analyzer      Analyzer
	paths         []string
	report        domain.Report
	err           error
	controlPort   int
	revision      uint64
	refreshing    bool
	refreshReply  chan controlReply
	changeWaiters []changeWaiter
	width         int
	height        int
	focus         pane
	sourceWorkspace
	fileIndex        int
	functionIndex    int
	qualityView      bool
	annotations      []Annotation
	nextAnnotationID int
	annotationStore  AnnotationStore
	annotationError  error
}

func NewModel(analyzer Analyzer, paths []string) Model {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	return Model{
		analyzer:   analyzer,
		paths:      append([]string(nil), paths...),
		refreshing: true,
	}
}

func (m Model) Init() tea.Cmd {
	return m.analyze()
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	previousRevision := m.revision
	var command tea.Cmd
	switch message := message.(type) {
	case reportMsg:
		m = m.withReport(message)
	case controlCommand:
		m, command = m.updateControl(message)
	case changeWaiterTimeoutMsg:
		m = m.timeoutChangeWaiter(message)
	default:
		m, command = m.updateInput(message)
	}

	if m.revision > previousRevision {
		m = m.answerChangeWaiters()
	}
	return m, command
}

func (m Model) updateInput(message tea.Msg) (Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case tea.KeyPressMsg:
		return m.updateKey(message)
	case tea.MouseWheelMsg:
		m = m.updateMouseWheel(message)
	case tea.MouseClickMsg:
		m = m.selectTreemap(message)
	}
	return m, nil
}

func (m Model) analyze() tea.Cmd {
	return func() tea.Msg {
		report, err := m.analyzer.Analyze(m.paths)
		return reportMsg{report: report, err: err}
	}
}

func (m Model) withReport(message reportMsg) Model {
	m.err = message.err
	m.refreshing = false
	m.revision++
	if message.err != nil {
		return m.drainRefreshReply()
	}
	return m.applyReport(message.report).drainRefreshReply()
}

// applyReport swaps in a fresh analysis report and reloads annotations.
func (m Model) applyReport(report domain.Report) Model {
	m.report = rankReport(report)
	m.fileIndex, m.functionIndex = 0, 0
	return m.reloadWorkspace()
}

// reloadWorkspace resets the source workspace and reloads annotations
// for the current report.
func (m Model) reloadWorkspace() Model {
	m = m.resetSourceWorkspace()
	m = m.loadAnnotations()
	return m.relocateAnnotations()
}

// drainRefreshReply answers a pending control-plane refresh, if any.
func (m Model) drainRefreshReply() Model {
	if m.refreshReply == nil {
		return m
	}
	m.refreshReply <- controlReply{result: m.controlState()}
	m.refreshReply = nil
	return m
}

func (m Model) updateKey(message tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.annotating {
		return m.updateAnnotationInput(message), nil
	}
	key := message.Keystroke()
	if next, command, handled := m.updateGlobalKey(key); handled {
		return next, command
	}
	if next, handled := m.updateSourceKey(key); handled {
		return next, nil
	}
	return m.updateMovementKey(key), nil
}

func (m Model) updateGlobalKey(key string) (Model, tea.Cmd, bool) {
	next, handled := m.updateReviewNavigationKey(key)
	if handled {
		return next, nil, true
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit, true
	case "r":
		if m.refreshing {
			return m, nil, true
		}
		m.refreshing = true
		m.revision++
		return m, m.analyze(), true
	default:
		next, handled = m.updateFocusKey(key)
		return next, nil, handled
	}
}

func (m Model) updateFocusKey(key string) (Model, bool) {
	switch key {
	case "e":
		return m.toggleQualityView(), true
	case "tab":
		m = m.setFocus(nextPane(m.focus, 1))
	case "shift+tab":
		m = m.setFocus(nextPane(m.focus, -1))
	case "esc":
		m = m.clearLineSelection()
	default:
		return m, false
	}
	return m, true
}

func (m Model) updateReviewNavigationKey(key string) (Model, bool) {
	switch key {
	case "[":
		return m.focusAdjacentAnnotation(-1), true
	case "]":
		return m.focusAdjacentAnnotation(1), true
	case ",":
		before := m
		m = m.moveFile(-1)
		return m.advanceNavigationRevision(before), true
	case ".":
		before := m
		m = m.moveFile(1)
		return m.advanceNavigationRevision(before), true
	default:
		return m, false
	}
}

func (m Model) updateSourceKey(key string) (Model, bool) {
	if m.focus != detailsPane || m.qualityView {
		return m, false
	}
	switch key {
	case "v":
		m = m.toggleVisualSelection()
	case "a":
		m = m.startAnnotating()
	case "d":
		m = m.removeAnnotationAtCursor()
	default:
		return m, false
	}
	return m, true
}

func (m Model) updateMovementKey(key string) Model {
	var delta int
	switch key {
	case "j":
		delta = 1
	case "k":
		delta = -1
	default:
		return m
	}
	next := m.move(delta)
	return next.advanceNavigationRevision(m)
}

func (m Model) updateMouseWheel(message tea.MouseWheelMsg) Model {
	before := m
	switch message.Button {
	case tea.MouseWheelUp:
		m = m.move(-1)
	case tea.MouseWheelDown:
		m = m.move(1)
	}
	return m.advanceNavigationRevision(before)
}

func (m Model) advanceNavigationRevision(before Model) Model {
	changed := m.fileIndex != before.fileIndex ||
		m.functionIndex != before.functionIndex ||
		m.sourceOffset != before.sourceOffset ||
		m.sourceCursor != before.sourceCursor
	if changed {
		m.revision++
	}
	return m
}

func nextPane(current pane, delta int) pane {
	return pane((int(current) + delta + 3) % 3)
}

func (m Model) move(delta int) Model {
	if m.focus == filesPane {
		return m.moveFile(delta)
	}
	if m.focus == functionsPane {
		file, ok := m.selectedFile()
		if ok {
			m.functionIndex = moveIndex(m.functionIndex, delta, len(file.Functions))
			m = m.resetSourceWorkspace()
		}
		return m
	}

	return m.moveDetails(delta)
}

func (m Model) moveFile(delta int) Model {
	next := moveIndex(m.fileIndex, delta, len(m.report.Files))
	if next == m.fileIndex {
		return m
	}

	m.fileIndex = next
	m.functionIndex = 0
	return m.resetSourceWorkspace()
}

func moveIndex(current int, delta int, length int) int {
	if length == 0 {
		return 0
	}

	next := current + delta
	// Overflow moves the sum in the opposite direction to the requested delta.
	if delta > 0 && next < current {
		return length - 1
	}
	if delta < 0 && next > current {
		return 0
	}
	return min(max(next, 0), length-1)
}

func rankReport(report domain.Report) domain.Report {
	report.Files = append([]domain.File(nil), report.Files...)
	for index := range report.Files {
		report.Files[index].Functions = rankedFunctions(report.Files[index].Functions)
	}
	sort.SliceStable(report.Files, func(left int, right int) bool {
		return fileRanksBefore(report.Files[left], report.Files[right])
	})

	return report
}

func rankedFunctions(functions []domain.Function) []domain.Function {
	functions = append([]domain.Function(nil), functions...)
	sort.SliceStable(functions, func(left int, right int) bool {
		return functionRanksBefore(functions[left], functions[right])
	})
	return functions
}

func fileRanksBefore(left domain.File, right domain.File) bool {
	if left.Total != right.Total {
		return left.Total > right.Total
	}

	return left.Path < right.Path
}

func functionRanksBefore(left domain.Function, right domain.Function) bool {
	if left.Complexity != right.Complexity {
		return left.Complexity > right.Complexity
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}

	return left.Name < right.Name
}

func (m Model) selectedFile() (domain.File, bool) {
	if m.fileIndex < 0 || m.fileIndex >= len(m.report.Files) {
		return domain.File{}, false
	}

	return m.report.Files[m.fileIndex], true
}

func (m Model) selectedFunction() (domain.Function, bool) {
	file, ok := m.selectedFile()
	if !ok || m.functionIndex < 0 || m.functionIndex >= len(file.Functions) {
		return domain.Function{}, false
	}

	return file.Functions[m.functionIndex], true
}
