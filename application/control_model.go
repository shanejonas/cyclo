package application

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/domain"
)

type ControlState struct {
	Revision              uint64         `json:"revision"`
	ControlPort           int            `json:"controlPort"`
	Paths                 []string       `json:"paths"`
	Focus                 string         `json:"focus"`
	Refreshing            bool           `json:"refreshing"`
	Error                 string         `json:"error,omitempty"`
	Report                ReportSummary  `json:"report"`
	Selection             Selection      `json:"selection"`
	Cursor                *SourceCursor  `json:"cursor,omitempty"`
	LineSelection         *LineSelection `json:"lineSelection,omitempty"`
	VisualSelectionActive bool           `json:"visualSelectionActive"`
	Annotations           []Annotation   `json:"annotations"`
	ActiveAnnotationID    string         `json:"activeAnnotationId,omitempty"`
}

type SourceCursor struct {
	SourceLine int `json:"sourceLine"`
}

type ReportSummary struct {
	Root             string  `json:"root"`
	FileCount        int     `json:"fileCount"`
	Functions        int     `json:"functions"`
	Total            int     `json:"total"`
	Average          float64 `json:"average"`
	CognitiveTotal   int     `json:"cognitiveTotal"`
	CognitiveAverage float64 `json:"cognitiveAverage"`
}

type Selection struct {
	FileIndex     *int             `json:"fileIndex,omitempty"`
	File          *FileSummary     `json:"file,omitempty"`
	FunctionIndex *int             `json:"functionIndex,omitempty"`
	Function      *FunctionDetails `json:"function,omitempty"`
	SourceOffset  int              `json:"sourceOffset"`
}

type FileSummary struct {
	Path             string  `json:"path"`
	FunctionCount    int     `json:"functionCount"`
	Total            int     `json:"total"`
	Peak             int     `json:"peak"`
	Average          float64 `json:"average"`
	CognitiveTotal   int     `json:"cognitiveTotal"`
	CognitivePeak    int     `json:"cognitivePeak"`
	CognitiveAverage float64 `json:"cognitiveAverage"`
}

type ControlReport struct {
	Root             string        `json:"root"`
	Files            []ControlFile `json:"files"`
	Functions        int           `json:"functions"`
	Total            int           `json:"total"`
	Average          float64       `json:"average"`
	CognitiveTotal   int           `json:"cognitiveTotal"`
	CognitiveAverage float64       `json:"cognitiveAverage"`
}

type ControlFile struct {
	Path             string            `json:"path"`
	Functions        []FunctionSummary `json:"functions"`
	Total            int               `json:"total"`
	Peak             int               `json:"peak"`
	Average          float64           `json:"average"`
	CognitiveTotal   int               `json:"cognitiveTotal"`
	CognitivePeak    int               `json:"cognitivePeak"`
	CognitiveAverage float64           `json:"cognitiveAverage"`
}

type FunctionSummary struct {
	Package               string                        `json:"package"`
	Name                  string                        `json:"name"`
	Complexity            int                           `json:"complexity"`
	CyclomaticDiagnostics []domain.CyclomaticDiagnostic `json:"cyclomaticDiagnostics,omitempty"`
	CognitiveComplexity   int                           `json:"cognitiveComplexity"`
	CognitiveDiagnostics  []domain.CognitiveDiagnostic  `json:"cognitiveDiagnostics,omitempty"`
	Line                  int                           `json:"line"`
	EndLine               int                           `json:"endLine"`
	Column                int                           `json:"column"`
}

type FunctionDetails struct {
	FunctionSummary
	Source string `json:"source"`
}

type AnnotationResult struct {
	Annotation Annotation   `json:"annotation"`
	State      ControlState `json:"state"`
}

func (m Model) WithControlPort(port int) Model {
	m.controlPort = port
	return m
}

func (m Model) WithAnnotationStore(store AnnotationStore) Model {
	m.annotationStore = store
	return m
}

func (m Model) updateControl(command controlCommand) (Model, tea.Cmd) {
	switch command.action {
	case getStateAction:
		command.answer(m.controlState(), nil)
	case waitForChangeAction:
		return m.waitForChange(command)
	case getReportAction:
		command.answer(m.controlReport(), nil)
	case setFocusAction:
		m = m.setFocus(command.pane)
		command.answer(m.controlState(), nil)
	default:
		return m.updateSourceControl(command)
	}
	return m, nil
}

func (m Model) updateSourceControl(command controlCommand) (Model, tea.Cmd) {
	switch command.action {
	case selectFileAction:
		return m.selectFile(command)
	case selectFunctionAction:
		return m.selectFunction(command)
	case scrollSourceAction:
		return m.scrollSource(command)
	case revealLinesAction:
		return m.revealLines(command)
	case clearLineSelectionAction:
		return m.clearControlLineSelection(command)
	case annotateLinesAction:
		return m.annotateLines(command)
	case removeAnnotationAction:
		return m.removeControlAnnotation(command)
	case refreshAction:
		return m.refresh(command)
	}
	return m, nil
}

func (m Model) selectFile(command controlCommand) (Model, tea.Cmd) {
	if command.index >= len(m.report.Files) {
		command.answer(nil, invalidParams("file index is out of range"))
		return m, nil
	}

	m.fileIndex = command.index
	m.functionIndex = 0
	m = m.resetSourceWorkspace()
	m.revision++
	command.answer(m.controlState(), nil)
	return m, nil
}

func (m Model) selectFunction(command controlCommand) (Model, tea.Cmd) {
	file, ok := m.selectedFile()
	if !ok || command.index >= len(file.Functions) {
		command.answer(nil, invalidParams("function index is out of range"))
		return m, nil
	}

	m.functionIndex = command.index
	m = m.resetSourceWorkspace()
	m.revision++
	command.answer(m.controlState(), nil)
	return m, nil
}

func (m Model) revealLines(command controlCommand) (Model, tea.Cmd) {
	startLine, endLine, ok := m.resolveSourceRange(command.startLine, command.endLine)
	if !ok {
		command.answer(nil, invalidParams("line range is outside the selected function"))
		return m, nil
	}
	function, _ := m.selectedFunction()
	m.focus = detailsPane
	m.sourceOffset = startLine - function.Line
	m.sourceCursor = endLine - function.Line
	m.lineSelection = &LineSelection{
		AnchorLine: startLine,
		StartLine:  startLine,
		EndLine:    endLine,
		Text:       m.sourceRangeText(startLine, endLine),
	}
	m.visualSelectionActive = false
	m.activeAnnotationID = ""
	m = m.keepSourceCursorVisible()
	m.revision++
	command.answer(m.controlState(), nil)
	return m, nil
}

func (m Model) clearControlLineSelection(command controlCommand) (Model, tea.Cmd) {
	m = m.clearLineSelection()
	command.answer(m.controlState(), nil)
	return m, nil
}

func (m Model) annotateLines(command controlCommand) (Model, tea.Cmd) {
	startLine, endLine, ok := m.resolveSourceRange(command.startLine, command.endLine)
	if !ok {
		command.answer(nil, invalidParams("line range is outside the selected function"))
		return m, nil
	}
	annotation, ok := m.newAnnotation(startLine, endLine, command.message)
	if !ok {
		command.answer(nil, invalidParams("line range is outside the selected function"))
		return m, nil
	}
	var err error
	m, err = m.saveAnnotation(annotation)
	if err != nil {
		m.annotationError = err
		command.answer(nil, runtimeError(err.Error()))
		return m, nil
	}
	m.revision++
	command.answer(AnnotationResult{Annotation: annotation, State: m.controlState()}, nil)
	return m, nil
}

func (m Model) resolveSourceRange(startLine int, endLine int) (int, int, bool) {
	if m.validSourceRange(startLine, endLine) {
		return startLine, endLine, true
	}
	function, ok := m.selectedFunction()
	if !ok {
		return 0, 0, false
	}
	absoluteStart := function.Line + startLine - 1
	absoluteEnd := function.Line + endLine - 1
	return absoluteStart, absoluteEnd, m.validSourceRange(absoluteStart, absoluteEnd)
}

func (m Model) removeControlAnnotation(command controlCommand) (Model, tea.Cmd) {
	before := len(m.annotations)
	var err error
	m, err = m.removeAnnotation(command.annotationID)
	if err != nil {
		m.annotationError = err
		command.answer(nil, runtimeError(err.Error()))
		return m, nil
	}
	if len(m.annotations) == before {
		command.answer(nil, invalidParams("annotation not found: "+command.annotationID))
		return m, nil
	}
	command.answer(m.controlState(), nil)
	return m, nil
}

func (m Model) scrollSource(command controlCommand) (Model, tea.Cmd) {
	function, ok := m.selectedFunction()
	if !ok {
		command.answer(nil, invalidParams("no function is selected"))
		return m, nil
	}

	m.sourceOffset = moveIndex(m.sourceOffset, command.lines, len(sourceCodeLines(function)))
	m.revision++
	command.answer(m.controlState(), nil)
	return m, nil
}

func (m Model) refresh(command controlCommand) (Model, tea.Cmd) {
	if m.refreshing {
		command.answer(nil, runtimeError("analysis is already refreshing"))
		return m, nil
	}

	m.refreshing = true
	m.refreshReply = command.reply
	m.revision++
	return m, m.analyze()
}

func (m Model) waitForChange(command controlCommand) (Model, tea.Cmd) {
	if m.revision > command.afterRevision || command.timeoutMs == 0 {
		command.answer(m.controlState(), nil)
		return m, nil
	}

	m.changeWaiters = append(m.changeWaiters, changeWaiter{
		afterRevision: command.afterRevision,
		reply:         command.reply,
	})
	duration := time.Duration(command.timeoutMs) * time.Millisecond
	return m, tea.Tick(duration, func(time.Time) tea.Msg {
		return changeWaiterTimeoutMsg{reply: command.reply}
	})
}

func (m Model) answerChangeWaiters() Model {
	waiters := make([]changeWaiter, 0, len(m.changeWaiters))
	for _, waiter := range m.changeWaiters {
		if m.revision <= waiter.afterRevision {
			waiters = append(waiters, waiter)
			continue
		}
		waiter.reply <- controlReply{result: m.controlState()}
	}
	m.changeWaiters = waiters
	return m
}

func (m Model) timeoutChangeWaiter(message changeWaiterTimeoutMsg) Model {
	waiters := make([]changeWaiter, 0, len(m.changeWaiters))
	found := false
	for _, waiter := range m.changeWaiters {
		if waiter.reply == message.reply {
			found = true
			continue
		}
		waiters = append(waiters, waiter)
	}
	if found {
		message.reply <- controlReply{result: m.controlState()}
	}
	m.changeWaiters = waiters
	return m
}

func (m Model) setFocus(focus pane) Model {
	if m.focus == focus {
		return m
	}
	m.focus = focus
	m.revision++
	return m
}

func (m Model) controlState() ControlState {
	state := ControlState{
		Revision:    m.revision,
		ControlPort: m.controlPort,
		Paths:       append([]string(nil), m.paths...),
		Focus:       paneName(m.focus),
		Refreshing:  m.refreshing,
		Report: ReportSummary{
			Root:             m.report.Root,
			FileCount:        len(m.report.Files),
			Functions:        m.report.Functions,
			Total:            m.report.Total,
			Average:          m.report.Average,
			CognitiveTotal:   m.report.CognitiveTotal,
			CognitiveAverage: m.report.CognitiveAverage,
		},
		Selection:             m.controlSelection(),
		LineSelection:         m.lineSelection,
		VisualSelectionActive: m.visualSelectionActive,
		Annotations:           append([]Annotation{}, m.annotations...),
		ActiveAnnotationID:    m.activeAnnotationID,
	}
	if m.sourceLine() > 0 {
		state.Cursor = &SourceCursor{SourceLine: m.sourceLine()}
	}
	if m.err != nil {
		state.Error = m.err.Error()
	}
	if m.annotationError != nil {
		state.Error = m.annotationError.Error()
	}
	return state
}

func (m Model) controlSelection() Selection {
	selection := Selection{SourceOffset: m.sourceOffset}
	file, ok := m.selectedFile()
	if !ok {
		return selection
	}

	fileIndex := m.fileIndex
	selection.FileIndex = &fileIndex
	selection.File = &FileSummary{
		Path:             file.Path,
		FunctionCount:    len(file.Functions),
		Total:            file.Total,
		Peak:             file.Peak,
		Average:          file.Average,
		CognitiveTotal:   file.CognitiveTotal,
		CognitivePeak:    file.CognitivePeak,
		CognitiveAverage: file.CognitiveAverage,
	}
	function, ok := m.selectedFunction()
	if !ok {
		return selection
	}

	functionIndex := m.functionIndex
	selection.FunctionIndex = &functionIndex
	details := functionDetails(function)
	selection.Function = &details
	return selection
}

func (m Model) controlReport() ControlReport {
	report := ControlReport{
		Root:             m.report.Root,
		Files:            make([]ControlFile, 0, len(m.report.Files)),
		Functions:        m.report.Functions,
		Total:            m.report.Total,
		Average:          m.report.Average,
		CognitiveTotal:   m.report.CognitiveTotal,
		CognitiveAverage: m.report.CognitiveAverage,
	}
	for _, file := range m.report.Files {
		functions := make([]FunctionSummary, 0, len(file.Functions))
		for _, function := range file.Functions {
			functions = append(functions, functionSummary(function))
		}
		report.Files = append(report.Files, ControlFile{
			Path:             file.Path,
			Functions:        functions,
			Total:            file.Total,
			Peak:             file.Peak,
			Average:          file.Average,
			CognitiveTotal:   file.CognitiveTotal,
			CognitivePeak:    file.CognitivePeak,
			CognitiveAverage: file.CognitiveAverage,
		})
	}
	return report
}

func functionDetails(function domain.Function) FunctionDetails {
	return FunctionDetails{FunctionSummary: functionSummary(function), Source: function.Source}
}

func functionSummary(function domain.Function) FunctionSummary {
	return FunctionSummary{
		Package:               function.Package,
		Name:                  function.Name,
		Complexity:            function.Complexity,
		CyclomaticDiagnostics: append([]domain.CyclomaticDiagnostic(nil), function.CyclomaticDiagnostics...),
		CognitiveComplexity:   function.CognitiveComplexity,
		CognitiveDiagnostics:  append([]domain.CognitiveDiagnostic(nil), function.CognitiveDiagnostics...),
		Line:                  function.Line,
		EndLine:               function.EndLine,
		Column:                function.Column,
	}
}

func (c controlCommand) answer(result any, err *controlError) {
	if c.reply != nil {
		c.reply <- controlReply{result: result, err: err}
	}
}

func parsePane(name string) (pane, bool) {
	switch name {
	case "files":
		return filesPane, true
	case "functions":
		return functionsPane, true
	case "source":
		return detailsPane, true
	default:
		return filesPane, false
	}
}

func paneName(focus pane) string {
	switch focus {
	case functionsPane:
		return "functions"
	case detailsPane:
		return "source"
	default:
		return "files"
	}
}
