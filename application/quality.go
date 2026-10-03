package application

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/domain"
	"github.com/shanejonas/cyclo/domain/quality"
)

func (m Model) detailsLines(width, height int, title string) []string {
	if m.qualityView {
		return m.qualityLines(width, height)
	}
	return m.sourceLines(width, height, title)
}

func (m Model) qualityLines(width, height int) []string {
	lines := []string{m.paneTitle("Quality · e source", detailsPane)}
	function, ok := m.selectedFunction()
	if ok {
		lines = append(lines, text.Bold(true).Render(function.Name))
	}
	body := m.qualityBody()
	available := len(body)
	if height > 0 {
		available = max(height-len(lines), 0)
	}
	start := min(m.qualityOffset, max(len(body)-available, 0))
	contentWidth := paneContentWidth(width)
	for _, line := range body[start:min(start+available, len(body))] {
		lines = append(lines, truncate(line, contentWidth))
	}
	return fitPaneLines(lines, width, height)
}

func (m Model) qualityBody() []string {
	if m.refreshing {
		return []string{muted.Render("Scanning quality…")}
	}
	if m.report.Quality == nil {
		return []string{muted.Render("Quality not analyzed")}
	}
	if m.report.Quality.Status == "error" {
		return append([]string{danger.Render("Quality analysis failed")}, strings.Split(m.report.Quality.Error, "\n")...)
	}
	function, ok := m.selectedFunction()
	if !ok {
		return []string{muted.Render("No function selected")}
	}
	if function.Quality == nil {
		return []string{muted.Render("Quality unavailable for this function"), muted.Render("Inactive build, generated, or nested literal")}
	}
	return functionQualityLines(function.Quality)
}

func (m Model) qualityBodyRowCount() int {
	if m.refreshing || m.report.Quality == nil {
		return 1
	}
	if m.report.Quality.Status == "error" {
		return 2 + strings.Count(m.report.Quality.Error, "\n")
	}
	function, ok := m.selectedFunction()
	if !ok {
		return 1
	}
	q := function.Quality
	if q == nil {
		return 2
	}
	// Six summary rows and two heading rows for each evidence section.
	return 10 + len(q.Diagnostics) + len(q.Effects) + len(q.MutationEvents)
}

func functionQualityLines(q *domain.FunctionQuality) []string {
	coverage := "No unknown effects under policy"
	if !q.Complete {
		coverage = "Incomplete: unknown effects remain"
	}
	return slices.Concat(
		[]string{
			fmt.Sprintf("Density %d milli · %d mutations", q.DensityMilli, q.Mutations),
			fmt.Sprintf("%d targets · %d unclassified calls", q.MutatedTargets, q.UnclassifiedCalls),
			muted.Render(coverage),
			muted.Render("Modeled effects; zero is not proof of purity"),
			"",
			amber.Render(fmt.Sprintf("Findings · %d", len(q.Diagnostics))),
		},
		diagnosticMessages(q.Diagnostics),
		[]string{"", blue.Render(fmt.Sprintf("Effects · %d", len(q.Effects)))},
		effectLines(q.Effects),
		mutationQualityLines(q.MutationEvents),
	)
}

func diagnosticMessages(diagnostics []quality.Diagnostic) []string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	return messages
}

func effectLines(effects []quality.Effect) []string {
	lines := make([]string, 0, len(effects))
	for _, effect := range effects {
		lines = append(lines, fmt.Sprintf("L%d %s: %s", effect.Line, effect.Kind, effect.Detail))
	}
	return lines
}

func mutationQualityLines(mutations []quality.Mutation) []string {
	lines := []string{"", blue.Render(fmt.Sprintf("Mutations · %d", len(mutations)))}
	for _, mutation := range mutations {
		target := mutation.Root
		if mutation.FieldPath != "" {
			target += "." + mutation.FieldPath
		}
		lines = append(lines, fmt.Sprintf("L%d %s (%s)", mutation.Line, target, mutation.Provenance))
	}
	return lines
}

func (m Model) qualityHeader() string {
	if m.report.Quality == nil {
		return ""
	}
	if m.report.Quality.Status == "error" {
		return " · quality failed"
	}
	if m.report.Quality.Report == nil {
		return ""
	}
	return fmt.Sprintf(" · Q %d", len(m.report.Quality.Report.Diagnostics))
}

func qualityBadge(function domain.Function) string {
	if function.Quality == nil {
		return ""
	}
	return amber.Render(fmt.Sprintf(" · Q %d · e quality", len(function.Quality.Diagnostics)))
}

func (m Model) detailsViewName() string {
	if m.qualityView {
		return "quality"
	}
	return "source"
}

func (m Model) setDetailsView(view string) Model {
	showQuality := view == "quality"
	if m.qualityView == showQuality {
		return m
	}
	m.qualityView = showQuality
	m.revision++
	return m
}

func (m Model) toggleQualityView() Model {
	view := "quality"
	if m.qualityView {
		view = "source"
	}
	m = m.setDetailsView(view)
	return m.setFocus(detailsPane)
}

func (m Model) moveDetails(delta int) Model {
	if !m.qualityView {
		return m.moveSourceCursor(delta)
	}
	rows := m.qualityBodyRowCount()
	maximum := max(rows-m.qualityViewportHeight(rows), 0)
	next := moveIndex(min(m.qualityOffset, maximum), delta, maximum+1)
	if next == m.qualityOffset {
		return m
	}
	m.qualityOffset = next
	m.revision++
	return m
}

func (m Model) qualityViewportHeight(rows int) int {
	if m.height == 0 {
		return rows
	}
	return max(m.sourceViewportHeight()+m.sourceHeaderHeight()-2, 1)
}

func (m Model) updateDetailsControl(command controlCommand) (Model, tea.Cmd) {
	if command.action != setDetailsViewAction {
		return m.updateSourceControl(command)
	}
	m = m.setDetailsView(command.view)
	m = m.setFocus(detailsPane)
	command.answer(m.controlState(), nil)
	return m, nil
}
