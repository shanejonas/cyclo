package bugreducer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/internal/reducer"
)

type progressMsg struct {
	progress reducer.Progress
	at       time.Time
}

type reductionResult struct {
	err     error
	summary string
	at      time.Time
}

func runDashboard(ctx context.Context, options options, source []byte, check *checker, destination *os.File, output io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	check.output = &checkerOutput{}
	model := newDashboard(ctx, cancel, options, source, check.output)
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(output), tea.WithoutSignalHandler())
	done := make(chan reductionResult, 1)
	go func() {
		result := reduceForDashboard(ctx, source, check, destination, program.Send, options)
		done <- result
		program.Send(result)
	}()
	_, programErr := program.Run()
	// A renderer/input failure must also stop the checker and save its best input.
	cancel()
	result := <-done
	_, reportErr := io.WriteString(output, result.summary)
	return errors.Join(programErr, result.err, reportErr)
}

func reduceForDashboard(ctx context.Context, source []byte, check *checker, destination *os.File, send func(tea.Msg), settings options) reductionResult {
	observe := func(progress reducer.Progress) {
		if progress.Checking {
			check.output.reset()
		}
		send(progressMsg{progress: progress, at: time.Now()})
	}
	reduced, err := reduceInput(ctx, settings, source, check, observe)
	var summary bytes.Buffer
	err = finish(destination, reduced, err, check.checks, len(source), &summary)
	return reductionResult{err: err, summary: summary.String(), at: time.Now()}
}

type dashboardTick time.Time

type sizePoint struct {
	elapsed time.Duration
	bytes   int
}

type dashboard struct {
	ctx      context.Context
	cancel   context.CancelFunc
	options  options
	output   *checkerOutput
	started  time.Time
	now      time.Time
	original int
	best     []byte
	seedOK   bool
	current  reducer.Progress
	checks   int
	accepted int
	rejected int
	failed   int
	history  []sizePoint
	recent   []string
	result   *reductionResult
	stopping bool
	width    int
	height   int
	focus    int
	offsets  [4]int
	expanded bool
}

func newDashboard(ctx context.Context, cancel context.CancelFunc, options options, source []byte, output *checkerOutput) dashboard {
	now := time.Now()
	return dashboard{
		ctx: ctx, cancel: cancel, options: options, output: output,
		started: now, now: now, original: len(source), best: source,
		width: 120, height: 36, history: []sizePoint{{bytes: len(source)}},
	}
}

func (m dashboard) Init() tea.Cmd { return dashboardClock() }

func dashboardClock() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(now time.Time) tea.Msg { return dashboardTick(now) })
}

func (m dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1)
	case tea.KeyPressMsg:
		return m.updateKey(msg.Keystroke())
	case progressMsg:
		m = m.withProgress(msg)
	case reductionResult:
		return m.completed(msg)
	case dashboardTick:
		return m.tick(time.Time(msg))
	}
	return m, nil
}

func (m dashboard) completed(result reductionResult) (tea.Model, tea.Cmd) {
	m.result, m.now = &result, result.at
	if m.stopping || m.ctx.Err() != nil {
		return m, tea.Quit
	}
	return m, nil
}

func (m dashboard) tick(now time.Time) (tea.Model, tea.Cmd) {
	if m.ctx.Err() != nil {
		return m.stop()
	}
	if m.result == nil {
		m.now = now
	}
	return m, dashboardClock()
}

func (m dashboard) stop() (tea.Model, tea.Cmd) {
	if m.result != nil {
		return m, tea.Quit
	}
	m.stopping = true
	m.cancel()
	return m, nil // Wait for checker cleanup and saving before leaving the screen.
}

func (m dashboard) updateKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m.stop()
	case "tab":
		m.focus = (m.focus + 1) % 4
	case "shift+tab":
		m.focus = (m.focus + 3) % 4
	case "enter":
		m.expanded = !m.expanded
	default:
		m = m.scroll(key)
	}
	return m, nil
}

func (m dashboard) scroll(key string) dashboard {
	direction := 1
	if m.focus == 3 {
		direction = -1 // Output scrolls relative to its live tail.
	}
	offset := m.offsets[m.focus]
	switch key {
	case "j", "down":
		offset += direction
	case "k", "up":
		offset -= direction
	case "home", "g":
		offset = m.scrollEdge(false)
	case "end", "G":
		offset = m.scrollEdge(true)
	}
	m.offsets[m.focus] = max(0, min(offset, len(m.paneLines(m.focus, m.width, m.height))-1))
	return m
}

func (m dashboard) scrollEdge(end bool) int {
	if end == (m.focus == 3) {
		return 0
	}
	return len(m.paneLines(m.focus, m.width, m.height))
}

func (m dashboard) withProgress(msg progressMsg) dashboard {
	p := msg.progress
	m.current, m.now = p, msg.at
	if p.Checking {
		m.checks++
		return m
	}
	if p.Err != nil {
		m.failed++
		return m
	}
	if !p.Accepted {
		m.rejected++
		return m
	}
	if p.Seed {
		m.seedOK = true
		return m
	}
	if len(p.Candidate) >= len(m.best) {
		return m
	}
	recent := append(m.deletion(p), m.recent...)
	history := append(m.history, sizePoint{elapsed: m.now.Sub(m.started), bytes: len(p.Candidate)})
	m.accepted++
	m.recent = recent[:min(len(recent), 300)]
	m.best = p.Candidate
	m.history = compactHistory(history)
	return m
}

func (m dashboard) deletion(p reducer.Progress) []string {
	header := fmt.Sprintf("Check #%d · %d → %d bytes · line %d", m.checks, len(m.best), len(p.Candidate), p.StartLine+1)
	removed := removedBytes(m.best, p)
	preview := safeText(string(removed[:min(len(removed), previewLimit)]))
	result := []string{header}
	for _, line := range splitPreview(preview) {
		result = append(result, "- "+line)
	}
	return append(result, "")
}

func removedBytes(source []byte, p reducer.Progress) []byte {
	if p.Unit != "" {
		return source[p.StartByte:p.EndByte]
	}

	start := lineByteOffset(source, p.StartLine)
	end := start + lineByteOffset(source[start:], p.RemovedLines)
	return source[start:end]
}

func lineByteOffset(source []byte, line int) int {
	offset := 0
	for range line {
		next := bytes.IndexByte(source[offset:], '\n')
		if next < 0 {
			return len(source)
		}
		offset += next + 1
	}
	return offset
}

// Keep the full time span without retaining every accepted candidate forever.
func compactHistory(points []sizePoint) []sizePoint {
	if len(points) <= 256 {
		return points
	}
	result := make([]sizePoint, 0, 130)
	for i := 0; i < len(points)-1; i += 2 {
		result = append(result, points[i])
	}
	return append(result, points[len(points)-1])
}
