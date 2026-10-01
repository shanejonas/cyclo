package bugreducer

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	dashboardAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("#66b9cf"))
	dashboardMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#89969d"))
	dashboardRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#df8585"))
	dashboardGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("#88c99a"))
)

var paneTitles = [4]string{"Statistics", "Size over time", "Recent reductions", "Checker output"}

func (m dashboard) View() tea.View {
	header := dashboardAccent.Bold(true).Render("CYCLO / BUG REDUCER") + "  " + safeText(filepath.Base(m.options.input))
	footer := "tab panes · j/k scroll · enter expand · q stop & save"
	if m.result != nil {
		footer = "tab panes · j/k scroll · enter expand · q close"
	}
	content := header + "\n" + m.workspace() + "\n" + dashboardMuted.Render(footer)
	view := tea.NewView(strings.Join(fitLines(strings.Split(content, "\n"), m.width, m.height), "\n"))
	view.AltScreen = true
	view.WindowTitle = "cyclo · bug reducer"
	return view
}

func (m dashboard) workspace() string {
	height := max(m.height-2, 1)
	if m.expanded || m.width < 96 || m.height < 24 {
		return m.panel(m.focus, m.width, height)
	}
	left, right := (m.width-1)/2, m.width/2
	top := min(14, height/2)
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, m.panel(0, left, top), " ", m.panel(1, right, top)),
		lipgloss.JoinHorizontal(lipgloss.Top, m.panel(2, left, height-top), " ", m.panel(3, right, height-top)),
	)
}

func (m dashboard) panel(index, width, height int) string {
	innerWidth, innerHeight := max(width-2, 1), max(height-2, 1)
	visible := max(innerHeight-2, 1)
	lines := m.paneLines(index, innerWidth, visible)
	offset := min(m.offsets[index], max(len(lines)-visible, 0))
	if index == 3 {
		offset = max(len(lines)-visible-m.offsets[index], 0)
	}
	title := fmt.Sprintf(" %d %s", index+1, paneTitles[index])
	if len(lines) > visible {
		title += fmt.Sprintf(" · %d/%d", offset+1, len(lines))
	}
	content := []string{dashboardAccent.Bold(true).Render(title), ""}
	content = append(content, colorPaneLines(lines[offset:min(offset+visible, len(lines))])...)
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#3d4a52"))
	if index == m.focus {
		style = style.BorderForeground(lipgloss.Color("#66b9cf"))
	}
	return style.Render(strings.Join(fitLines(content, innerWidth, innerHeight), "\n"))
}

func (m dashboard) paneLines(index, width, height int) []string {
	switch index {
	case 0:
		return m.statistics()
	case 1:
		return m.sizeChart(width, height)
	case 2:
		if len(m.recent) == 0 {
			return []string{"Accepted deletions appear here.", "Only candidates that preserve the bug are kept."}
		}
		return m.recent
	default:
		return m.checkerLines()
	}
}

func (m dashboard) statistics() []string {
	elapsed := m.now.Sub(m.started)
	percent := 100 * float64(m.original-len(m.best)) / float64(max(m.original, 1))
	lines := []string{
		m.status(),
		fmt.Sprintf("Best: %d → %d bytes  (%.1f%% removed)", m.original, len(m.best), percent),
		fmt.Sprintf("Elapsed: %s · %.1f checks/s", elapsed.Round(time.Second), float64(m.checks)/max(elapsed.Seconds(), 0.1)),
		fmt.Sprintf("Checks: %d · reductions: %d", m.checks, m.accepted),
		fmt.Sprintf("Rejected: %d · errors: %d", m.rejected, m.failed),
		m.attemptLabel(),
		"Output: " + safeText(m.options.output),
	}
	if m.result != nil && m.result.err != nil {
		lines = append(lines, "Error: "+safeText(m.result.err.Error()))
	}
	return lines
}

func (m dashboard) status() string {
	if m.result != nil {
		return m.finishedStatus()
	}
	if m.stopping {
		return "Stopping · waiting for cleanup and saving…"
	}
	if !m.seedOK {
		return "Validating original input…"
	}
	if m.options.language == "go" {
		return "Reducing · deleting Go syntax units"
	}
	return "Reducing · deleting whole-line chunks"
}

func (m dashboard) finishedStatus() string {
	if m.result.err == nil {
		if m.options.language == "go" {
			return "Complete · no further syntax-unit deletions"
		}
		return "Complete · no further single-line deletions"
	}
	if m.result.summary != "" {
		return "Stopped · best accepted input saved"
	}
	return "Failed · no reduced input saved"
}

func (m dashboard) attemptLabel() string {
	if m.current.Seed {
		return "Pass: validate original input"
	}
	if m.current.Unit != "" {
		return fmt.Sprintf("Pass: remove %s · trying %d bytes", m.current.Unit, len(m.current.Candidate))
	}
	if m.current.ChunkSize == 0 {
		return "Waiting for checker…"
	}
	return fmt.Sprintf("Pass: %d-line chunks · trying %d bytes", m.current.ChunkSize, len(m.current.Candidate))
}

func (m dashboard) checkerLines() []string {
	outcome := "running"
	if !m.current.Checking {
		outcome = m.checkOutcome()
	}
	label := fmt.Sprintf("Check #%d · %s", m.checks, outcome)
	lines := []string{label, "$ " + safeText(strings.Join(m.options.command, " ")) + " <candidate>", ""}
	output := safeText(m.output.snapshot())
	if output == "" {
		return append(lines, "(no checker output)")
	}
	return append(lines, strings.Split(output, "\n")...)
}

func (m dashboard) checkOutcome() string {
	if m.current.Err != nil {
		return "error: " + safeText(m.current.Err.Error())
	}
	if m.current.Accepted {
		return "accepted · bug preserved (exit 0)"
	}
	return "rejected · bug not confirmed"
}

func fitLines(lines []string, width, height int) []string {
	result := make([]string, max(height, 1))
	for i := range result {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], max(width, 1), "…")
		}
		result[i] = line + strings.Repeat(" ", max(width-ansi.StringWidth(line), 0))
	}
	return result
}

func colorPaneLines(lines []string) []string {
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = line
		if strings.HasPrefix(line, "- ") {
			result[i] = dashboardRed.Render(line)
		}
	}
	return result
}

// Checker output and input previews are untrusted terminal text.
func safeText(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ReplaceAll(ansi.Strip(value), "\t", "    "))
}

func splitPreview(value string) []string {
	lines := strings.Split(strings.TrimSuffix(value, "\n"), "\n")
	if len(lines) > 80 {
		return append(lines[:80], "… (preview truncated)")
	}
	return lines
}
