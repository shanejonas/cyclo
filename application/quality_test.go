package application

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/domain"
	"github.com/shanejonas/cyclo/domain/quality"
)

func qualityModel() Model {
	loc := quality.Location{Path: "sample.go", Line: 10, Column: 1, Name: "sample.Change"}
	d := quality.Diagnostic{Location: loc, RuleID: "mutation_per_target", Actual: 4, Limit: 3, Message: "mutation_per_target: 4 exceeds 3"}
	q := &domain.FunctionQuality{FunctionResult: quality.FunctionResult{Location: loc, DensityMilli: 1000, Mutations: 4, MutatedTargets: 1, Complete: false, UnclassifiedCalls: 1, Effects: []quality.Effect{{Kind: quality.UnknownEffect, Detail: "callback", Line: 12}}}, Diagnostics: []quality.Diagnostic{d}, MutationEvents: []quality.Mutation{{Root: "state", FieldPath: "count", Line: 11, Provenance: quality.External}}}
	return Model{width: 100, height: 30, report: domain.Report{Root: "/workspace", Quality: &domain.QualityAnalysis{Status: "ready", Report: &quality.Report{SchemaVersion: 1, Functions: []quality.FunctionResult{q.FunctionResult}, Diagnostics: q.Diagnostics}}, Files: []domain.File{{Path: "sample.go", Functions: []domain.Function{{Name: "Change", Line: 10, EndLine: 14, Column: 1, Source: "func Change() {\n one\n two\n three\n}", Quality: q}}}}}}
}

func TestQualityToggleShowsEvidenceAndPreservesSourceCursor(t *testing.T) {
	m := qualityModel()
	m.focus, m.sourceCursor = detailsPane, 2
	m, _ = m.updateFocusKey("e")
	if !m.qualityView || m.sourceCursor != 2 {
		t.Fatal("toggle loses source position")
	}
	view := ansi.Strip(strings.Join(m.qualityLines(100, 30), "\n"))
	for _, want := range []string{"Density 1000", "mutation_per_target: 4 exceeds 3", "Incomplete", "L12 unknown: callback", "L11 state.count (external)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("quality view missing %q: %s", want, view)
		}
	}
	m.height = 8
	revision := m.revision
	m = m.moveDetails(1)
	if m.qualityOffset != 1 || m.revision <= revision || m.sourceCursor != 2 {
		t.Fatal("quality scrolling changes source or fails to advance revision")
	}
	for range 100 {
		m = m.moveDetails(1)
	}
	revision = m.revision
	m = m.moveDetails(1)
	if m.revision != revision {
		t.Fatal("quality scroll advances beyond final page")
	}
	m, _ = m.updateFocusKey("e")
	if m.qualityView || m.sourceCursor != 2 {
		t.Fatal("source cursor not restored")
	}
}

func TestRPCReportAndStateExposeQuality(t *testing.T) {
	m := qualityModel()
	for _, value := range []any{m.controlReport(), m.controlState()} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"density_milli", "mutation_events", "provenance", "rule_id", "unclassified_calls"} {
			if !strings.Contains(string(encoded), key) {
				t.Fatalf("RPC omits %s: %s", key, encoded)
			}
		}
	}
	command, err := parseSetDetailsView(json.RawMessage(`{"view":"quality"}`))
	if err != nil {
		t.Fatal(err)
	}
	command.reply = make(chan controlReply, 1)
	m, _ = m.updateControl(command)
	state := (<-command.reply).result.(ControlState)
	if state.DetailsView != "quality" || state.Focus != "source" {
		t.Fatalf("state: %+v", state)
	}
	for _, input := range []string{`{}`, `{"view":"bad"}`, `{"view":"source","extra":1}`} {
		if _, err := parseSetDetailsView(json.RawMessage(input)); err == nil {
			t.Fatal("invalid details view accepted")
		}
	}
	command = controlCommand{action: revealLinesAction, startLine: 11, endLine: 11, reply: make(chan controlReply, 1)}
	m, _ = m.updateControl(command)
	if m.qualityView || m.sourceLine() != 11 {
		t.Fatal("revealLines does not return to source")
	}
}

func TestQualityFailuresAndMissingFactsRemainVisible(t *testing.T) {
	m := qualityModel()
	m.report.Quality = &domain.QualityAnalysis{Status: "error", Error: "sample.go: undefined name"}
	if !strings.Contains(strings.Join(m.qualityBody(), "\n"), "analysis failed") || m.qualityHeader() != " · quality failed" {
		t.Fatal("quality failure hidden")
	}
	m.report.Quality = &domain.QualityAnalysis{Status: "ready", Report: &quality.Report{}}
	m.report.Files[0].Functions[0].Quality = nil
	if !strings.Contains(strings.Join(m.qualityBody(), "\n"), "unavailable") {
		t.Fatal("missing facts look clean")
	}
	m.qualityView = true
	m.focus = detailsPane
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = next.(Model)
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatal("quality view exceeds narrow terminal")
		}
	}
}

func TestQualityScrollExtentMatchesDisplayedStates(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Model)
	}{
		{"evidence", func(*Model) {}},
		{"empty evidence", func(m *Model) { m.report.Files[0].Functions[0].Quality = &domain.FunctionQuality{} }},
		{"refreshing", func(m *Model) { m.refreshing = true }},
		{"not analyzed", func(m *Model) { m.report.Quality = nil }},
		{"multiline failure", func(m *Model) {
			m.report.Quality = &domain.QualityAnalysis{Status: "error", Error: "first\nsecond\n"}
		}},
		{"no selection", func(m *Model) { m.report.Files = nil }},
		{"missing facts", func(m *Model) { m.report.Files[0].Functions[0].Quality = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, height := range []int{0, 2, 8, 30} {
				m := qualityModel()
				m.height, m.qualityView = height, true
				tc.change(&m)
				rows := len(m.qualityBody())
				if got := m.qualityBodyRowCount(); got != rows {
					t.Fatalf("height %d: counted %d rows, displayed %d", height, got, rows)
				}
				visible := rows
				if height != 0 {
					visible = max(m.sourceViewportHeight()+m.sourceHeaderHeight()-2, 1)
				}
				m.qualityOffset = rows + 5
				next := m.moveDetails(0)
				if want := max(rows-visible, 0); next.qualityOffset != want {
					t.Fatalf("height %d: last page %d, want %d", height, next.qualityOffset, want)
				}
			}
		})
	}
}
