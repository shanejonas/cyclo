package application

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclomatic-complexity-tui/domain"
)

func TestViewUsesAlternateScreen(t *testing.T) {
	view := (Model{width: 120}).View()
	if !view.AltScreen {
		t.Fatal("view does not use the alternate screen")
	}
}

func TestViewEnablesMouseWheelEvents(t *testing.T) {
	view := (Model{width: 120}).View()
	if view.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse mode = %d, want cell motion", view.MouseMode)
	}
}

func TestViewUsesCycloName(t *testing.T) {
	view := (Model{width: 120}).View()
	if view.WindowTitle != "cyclo" {
		t.Fatalf("window title = %q, want cyclo", view.WindowTitle)
	}
}

func TestHeaderKeepsTheControlPortVisible(t *testing.T) {
	model := Model{
		width:       80,
		controlPort: 54321,
		paths:       []string{"/home/shanejonas/code/github.com/shanejonas/a-very-long-repository-name"},
	}

	header := ansi.Strip(model.header(model.width))
	if !strings.Contains(header, "CYCLO") || !strings.Contains(header, "RPC :54321") {
		t.Fatalf("header hides the control port: %q", header)
	}
}

func TestWideViewUsesAnalyticsTablesAndSource(t *testing.T) {
	model := Model{
		width:         120,
		focus:         functionsPane,
		functionIndex: 1,
		report: domain.Report{
			Root:             "/workspace",
			Functions:        2,
			Total:            12,
			Average:          6,
			CognitiveTotal:   9,
			CognitiveAverage: 4.5,
			Files: []domain.File{{
				Path:           "/workspace/hot.go",
				Total:          12,
				Peak:           8,
				Average:        6,
				CognitiveTotal: 9,
				CognitivePeak:  7,
				Functions: []domain.Function{
					{Name: "Simple", Complexity: 4, CognitiveComplexity: 2, Line: 3, EndLine: 5, Source: "func Simple() {\n\treturn\n}"},
					{Name: "Branchy", Complexity: 8, CognitiveComplexity: 7, CognitiveDiagnostics: []domain.CognitiveDiagnostic{{Line: 13, Increment: 1, Kind: "if"}}, Line: 12, EndLine: 16, Source: "func Branchy(ready bool) string {\n\tif ready {\n\t\treturn \"branchy\"\n\t}\n\treturn \"plain\"\n}"},
				},
			}},
		},
	}

	view := ansi.Strip(model.View().Content)
	for _, text := range []string{
		"CC PEAK · PATHS",
		"COG PEAK · LOAD",
		"CC function distribution",
		"FILE",
		"SUM",
		"MAX COG",
		"FUNCTION",
		"CC COG LINES",
		"Source",
		"return \"branchy\"",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("analytics view is missing %q:\n%s", text, view)
		}
	}
	if strings.Contains(view, "return\n") {
		t.Fatalf("source pane shows the wrong function:\n%s", view)
	}
}

func TestWideViewPlacesFilesFunctionsAndSourceSideBySide(t *testing.T) {
	model := Model{
		width:  120,
		height: 24,
		report: domain.Report{
			Files: []domain.File{{
				Path: "hot.go",
				Functions: []domain.Function{{
					Name:   "Branchy",
					Source: "func Branchy() {}",
				}},
			}},
		},
	}

	for _, line := range strings.Split(ansi.Strip(model.View().Content), "\n") {
		if strings.Contains(line, "Files") && strings.Contains(line, "Functions") && strings.Contains(line, "Source") {
			return
		}
	}

	t.Fatal("wide view does not place Files, Functions, and Source side by side")
}

func TestShiftTabMovesPaneFocusBackward(t *testing.T) {
	model := Model{focus: filesPane}
	message := tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}

	updated, _ := model.Update(message)
	model = updated.(Model)
	if model.focus != detailsPane {
		t.Fatalf("focus = %d, want details pane", model.focus)
	}

	updated, _ = model.Update(message)
	model = updated.(Model)
	if model.focus != functionsPane {
		t.Fatalf("focus = %d, want functions pane", model.focus)
	}
}

func TestMouseWheelMovesTheFocusedPane(t *testing.T) {
	function := domain.Function{Name: "one", Line: 10, EndLine: 11, Source: "one\ntwo"}
	model := Model{focus: filesPane, report: domain.Report{Files: []domain.File{
		{Path: "one.go", Functions: []domain.Function{function}},
		{Path: "two.go", Functions: []domain.Function{function}},
	}}}
	model = updateSourceModel(t, model, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if model.fileIndex != 1 {
		t.Fatalf("wheel down file index = %d, want 1", model.fileIndex)
	}

	model = Model{focus: functionsPane, report: domain.Report{Files: []domain.File{{
		Path: "one.go", Functions: []domain.Function{function, {Name: "two"}},
	}}}}
	model = updateSourceModel(t, model, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if model.functionIndex != 1 {
		t.Fatalf("wheel down function index = %d, want 1", model.functionIndex)
	}

	model = sourceWorkspaceModel()
	model = updateSourceModel(t, model, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if model.sourceLine() != 11 {
		t.Fatalf("wheel down source line = %d, want 11", model.sourceLine())
	}
	model = updateSourceModel(t, model, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if model.sourceLine() != 10 {
		t.Fatalf("wheel up source line = %d, want 10", model.sourceLine())
	}
}

func TestScrollbarThumbSizeStaysFixedWhilePositionChanges(t *testing.T) {
	top, topHeight := scrollbarThumb(11, 0, 30, 7)
	bottom, bottomHeight := scrollbarThumb(11, 23, 30, 7)

	if top != 0 || bottom != 9 {
		t.Fatalf("thumb positions = %d and %d, want 0 and 9", top, bottom)
	}
	if topHeight != 2 || bottomHeight != 2 {
		t.Fatalf("thumb heights = %d and %d, want 2", topHeight, bottomHeight)
	}
}

func TestScrollablePanesShowAMutedThumb(t *testing.T) {
	files := make([]domain.File, 10)
	for index := range files {
		files[index] = domain.File{Path: fmt.Sprintf("file-%d.go", index)}
	}
	model := Model{report: domain.Report{Files: files}}
	assertMutedScrollbar(t, model.fileTable(30, 6), 2)

	model = sourceWorkspaceModel()
	assertMutedScrollbar(t, model.sourceLines(40, 8, "Source"), 4)
}

func assertMutedScrollbar(t *testing.T, lines []string, contentStart int) {
	t.Helper()
	if strings.Contains(ansi.Strip(strings.Join(lines[:contentStart], "\n")), "█") {
		t.Fatal("scrollbar overlaps pane headers")
	}
	rendered := strings.Join(lines[contentStart:], "\n")
	if !strings.Contains(ansi.Strip(rendered), "█") {
		t.Fatal("scrollable pane has no thumb")
	}
	if !strings.Contains(rendered, "38;2;118;130;134") {
		t.Fatal("scrollbar thumb is not muted")
	}
}

func TestSelectedFileAndFunctionStayVisibleWhileSourceIsFocused(t *testing.T) {
	model := Model{
		focus: detailsPane,
		report: domain.Report{Files: []domain.File{{
			Path:      "application/view.go",
			Functions: []domain.Function{{Name: "renderSourceCodeLines"}},
		}}},
	}

	file := model.fileRow(model.report.Files[0], 40, true)
	function := model.functionRow(model.report.Files[0].Functions[0], 40, true)
	if !strings.HasPrefix(ansi.Strip(file), "› application/view.go") {
		t.Fatalf("inactive file selection is hidden: %q", ansi.Strip(file))
	}
	if !strings.HasPrefix(ansi.Strip(function), "› renderSourceCodeLines") {
		t.Fatalf("inactive function selection is hidden: %q", ansi.Strip(function))
	}
	if !strings.Contains(file, "48;2;20;31;34") || !strings.Contains(function, "48;2;20;31;34") {
		t.Fatal("inactive selections do not retain a visible row highlight")
	}
}

func TestSourceSeparatesAndCombinesComplexityColors(t *testing.T) {
	lines := sourceCodeLines(domain.Function{
		Line:                  10,
		Complexity:            3,
		CyclomaticDiagnostics: []domain.CyclomaticDiagnostic{{Line: 10, Kind: "function"}, {Line: 12, Kind: "case"}, {Line: 13, Kind: "if"}},
		CognitiveDiagnostics:  []domain.CognitiveDiagnostic{{Line: 11, Increment: 1, Kind: "switch"}, {Line: 13, Increment: 2, Nesting: 1, Kind: "if"}},
		Source:                "func mixed(v int) {\n\tswitch v {\n\tcase 1:\n\t\tif ready {\n\t\t}\n\t}\n}",
	})

	cyclomaticColor := "38;2;221;194;125"
	if !strings.Contains(lines[2], cyclomaticColor) || !strings.Contains(lines[3], cyclomaticColor) {
		t.Fatalf("cyclomatic source is not amber:\n%s", strings.Join(lines, "\n"))
	}
	for index, number := range []string{"  12 │ ", "  13 │ "} {
		amberNumber := strings.TrimSuffix(amber.Render(number), "\x1b[m")
		if strings.Contains(lines[index+2], amberNumber) {
			t.Fatalf("cyclomatic line colors its line number: %q", lines[index+2])
		}
	}
	purpleForeground := "38;2;167;139;250"
	purpleBackground := "48;2;43;33;64"
	if !strings.Contains(lines[1], purpleBackground) {
		t.Fatalf("cognitive-only line has no purple background: %q", lines[1])
	}
	if strings.Contains(lines[2], purpleForeground) {
		t.Fatalf("cyclomatic-only line uses purple: %q", lines[2])
	}
	if strings.Contains(lines[2], purpleBackground) {
		t.Fatalf("cyclomatic-only line has a purple background: %q", lines[2])
	}
	if !strings.Contains(lines[3], purpleBackground) {
		t.Fatalf("shared line has no purple background: %q", lines[3])
	}
	if strings.Contains(lines[3], "58;2;167;139;250") {
		t.Fatalf("shared line still uses a purple underline: %q", lines[3])
	}
	if !strings.HasPrefix(ansi.Strip(lines[3]), "    13 │ ") {
		t.Fatalf("shared metric line has a gutter marker: %q", lines[3])
	}
}

func TestSourceDoesNotInventAPathFromIndentation(t *testing.T) {
	lines := sourceCodeLines(domain.Function{
		Line:   20,
		Source: "func branchy() {\n\tif ready {\n\t\treturn\n\t}\n}",
	})

	if strings.ContainsAny(ansi.Strip(strings.Join(lines, "\n")), "╭╮╰╯─") {
		t.Fatalf("source invents a path from indentation:\n%s", strings.Join(lines, "\n"))
	}
}

func TestSourceVisualizesErrorPath(t *testing.T) {
	lines := sourceCodeLines(domain.Function{
		Line:   30,
		Source: "func run() error {\n\terr := work()\n\tif err != nil {\n\t\treturn err\n\t}\n\treturn nil\n}",
	})

	errorPrefix := strings.TrimSuffix(danger.Render("╭"), "\x1b[m")
	if !strings.Contains(lines[2], errorPrefix) {
		t.Fatalf("error path does not start beside its condition: %q", lines[2])
	}
	if !strings.HasSuffix(ansi.Strip(lines[2]), "      ╭if err != nil {") {
		t.Fatalf("error path does not touch its condition: %q", lines[2])
	}
	if !strings.Contains(ansi.Strip(lines[3]), "│    return err") {
		t.Fatalf("error path does not follow its body: %q", lines[3])
	}
	errorPrefix = strings.TrimSuffix(danger.Render("╰"), "\x1b[m")
	if !strings.Contains(lines[4], errorPrefix) {
		t.Fatalf("error path does not return to the happy path: %q", lines[4])
	}
	errorNumber := strings.TrimSuffix(danger.Render("  33 │ "), "\x1b[m")
	if strings.Contains(lines[3], errorNumber) {
		t.Fatalf("error return colors its line number: %q", lines[3])
	}
	happyNumber := strings.TrimSuffix(green.Render("  35 │ "), "\x1b[m")
	if strings.Contains(lines[5], happyNumber) {
		t.Fatalf("happy-path return colors its line number: %q", lines[5])
	}
	greenNil := strings.TrimSuffix(green.Render("nil"), "\x1b[m")
	if strings.Contains(lines[5], greenNil) {
		t.Fatalf("nil error result uses the success color: %q", lines[5])
	}
}

func TestSourceShowsAddedAndDeletedDiffLines(t *testing.T) {
	function := domain.Function{
		Package: "sample", Name: "value", Line: 10, EndLine: 12,
		Source: "func value() string {\n\treturn \"after\"\n}",
		DiffLines: []domain.DiffLine{
			{Kind: "deleted", OldLine: 11, NewLine: 11, Text: "\treturn \"before\""},
			{Kind: "added", OldLine: 12, NewLine: 11, Text: "\treturn \"after\""},
		},
	}
	model := Model{
		width: 90, height: 20, focus: detailsPane,
		report: domain.Report{DiffBase: "main", Files: []domain.File{{Path: "sample.go", Functions: []domain.Function{function}}}},
	}

	rendered := model.View().Content
	plain := ansi.Strip(rendered)
	for _, want := range []string{"Source · diff main · +1 −1", "−  11 │", "return \"before\"", "+  11 │", "return \"after\""} {
		if !strings.Contains(plain, want) {
			t.Fatalf("diff source is missing %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(rendered, "38;2;102;166;120") || !strings.Contains(rendered, "38;2;209;105;105") {
		t.Fatalf("diff gutter does not use green and red:\n%q", rendered)
	}
}

func TestReturnColorsOnlyReturnedValues(t *testing.T) {
	errorLine := styledReturn(
		"        return nil, nil, fmt.Errorf(\"boom: %w\", err)",
		danger,
		true,
	)
	errorValue := strings.TrimSuffix(danger.Render("fmt.Errorf(\"boom: %w\", err)"), "\x1b[m")
	if !strings.Contains(errorLine, errorValue) {
		t.Fatalf("returned error is not colored: %q", errorLine)
	}
	if strings.Contains(errorLine, strings.TrimSuffix(danger.Render("return"), "\x1b[m")) {
		t.Fatalf("return keyword uses the error color: %q", errorLine)
	}
	if strings.Contains(errorLine, strings.TrimSuffix(danger.Render("nil"), "\x1b[m")) {
		t.Fatalf("nil result uses the error color: %q", errorLine)
	}

	happyLine := styledReturn("    return result, roots, nil", green, false)
	happyValue := strings.TrimSuffix(green.Render("result, roots"), "\x1b[m")
	if !strings.Contains(happyLine, happyValue) {
		t.Fatalf("returned result is not colored: %q", happyLine)
	}
	if strings.Contains(happyLine, strings.TrimSuffix(green.Render("return"), "\x1b[m")) {
		t.Fatalf("return keyword uses the success color: %q", happyLine)
	}
	if strings.Contains(happyLine, strings.TrimSuffix(green.Render("nil"), "\x1b[m")) {
		t.Fatalf("nil error result uses the success color: %q", happyLine)
	}
}

func TestSourceVisualizesAnyGuardReturningAnError(t *testing.T) {
	lines := sourceCodeLines(domain.Function{
		Line:   40,
		Source: "func run(paths []string) error {\n\tif len(paths) == 0 {\n\t\treturn fmt.Errorf(\"path required\")\n\t}\n\treturn nil\n}",
	})

	if !strings.HasSuffix(ansi.Strip(lines[1]), "      ╭if len(paths) == 0 {") {
		t.Fatalf("validation guard does not start an error path: %q", lines[1])
	}
	if !strings.Contains(ansi.Strip(lines[2]), "│    return fmt.Errorf") {
		t.Fatalf("validation error does not follow its guard: %q", lines[2])
	}
	if !strings.HasSuffix(ansi.Strip(lines[3]), "      ╰}") {
		t.Fatalf("validation guard does not close its error path: %q", lines[3])
	}
}

func TestSourceVisualizesTheNearestGuardReturningAnError(t *testing.T) {
	lines := sourceCodeLines(domain.Function{
		Line:   50,
		Source: "func run() error {\n\tif ready {\n\t\tif invalid {\n\t\t\treturn fmt.Errorf(\"invalid\")\n\t\t}\n\t}\n\treturn nil\n}",
	})

	if strings.Contains(ansi.Strip(lines[1]), "╭") {
		t.Fatalf("outer block incorrectly starts the error path: %q", lines[1])
	}
	if !strings.Contains(ansi.Strip(lines[2]), "╭if invalid {") {
		t.Fatalf("nearest error guard does not start the error path: %q", lines[2])
	}
}

func TestFunctionTableShowsFunctionLength(t *testing.T) {
	function := domain.Function{
		Name:                "fitLines",
		Complexity:          6,
		CognitiveComplexity: 9,
		Line:                337,
		EndLine:             352,
	}
	row := ansi.Strip((Model{}).functionRow(function, 40, false))
	fields := strings.Fields(row)

	if got := fields[len(fields)-1]; got != "16" {
		t.Fatalf("function length = %s, want 16: %q", got, row)
	}
	if !strings.HasSuffix(strings.TrimSpace(functionTableHeader(40)), "CC COG LINES") {
		t.Fatalf("function table header does not describe its length: %q", functionTableHeader(40))
	}
	if !strings.Contains((Model{}).functionRow(function, 40, false), "38;2;167;139;250") {
		t.Fatal("cognitive complexity does not use purple")
	}
}

func TestLargeNumber(t *testing.T) {
	want := []string{"█████", "█   █", "█████", "    █", "█████"}
	got := largeNumber(9)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("large number = %q, want %q", got, want)
	}
}

func TestLongReportRootKeepsFilenameAndCoordinates(t *testing.T) {
	root := filepath.Join("/", strings.Repeat("long-root-", 12))
	model := Model{
		width: 120,
		focus: filesPane,
		report: domain.Report{
			Root: root,
			Files: []domain.File{{
				Path:    filepath.Join(root, "hot.go"),
				Total:   8,
				Peak:    8,
				Average: 8,
				Functions: []domain.Function{{
					Package:    "sample",
					Name:       "Branchy",
					Complexity: 8,
					Line:       12,
					Column:     2,
				}},
			}},
			Functions: 1,
			Total:     8,
			Average:   8,
		},
	}

	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "› hot.go") {
		t.Fatalf("file row lost the filename:\n%s", view)
	}
	if !strings.Contains(view, "hot.go:12:2") {
		t.Fatalf("detail row lost the source location:\n%s", view)
	}
}

func TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow(t *testing.T) {
	root := filepath.Join("/", strings.Repeat("long-root-", 12))
	path := filepath.Join(root, "adapters", "outbound", "gocyclo", "analyzer.go")
	model := Model{
		width: 80,
		focus: filesPane,
		report: domain.Report{
			Root: root,
			Files: []domain.File{{
				Path:    path,
				Total:   8,
				Peak:    8,
				Average: 8,
				Functions: []domain.Function{{
					Package:    "sample",
					Name:       "Branchy",
					Complexity: 8,
					Line:       12,
					Column:     2,
				}},
			}},
		},
	}

	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "› analyzer.go") {
		t.Fatalf("file row lost the nested filename:\n%s", view)
	}
	if !strings.Contains(view, "analyzer.go:12:2") {
		t.Fatalf("detail row lost the nested source location:\n%s", view)
	}
}

func TestNarrowViewUsesTerminalCellWidth(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "CJK", path: "長い-long-path.go"},
		{name: "combining character", path: "e\u0301-long-path.go"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := Model{
				width: 10,
				focus: filesPane,
				report: domain.Report{
					Files: []domain.File{{Path: test.path, Total: 1, Peak: 1}},
				},
			}

			lines := strings.Split(model.View().Content, "\n")
			if len(lines) < 3 {
				t.Fatalf("narrow view has %d lines, want at least 3", len(lines))
			}
			for _, line := range lines {
				if width := ansi.StringWidth(line); width > model.width {
					t.Fatalf("line width = %d, want <= %d: %q", width, model.width, line)
				}
			}
			row := ""
			for _, line := range lines {
				if strings.Contains(line, "›") {
					row = line
					break
				}
			}
			if width := ansi.StringWidth(row); width != model.width {
				t.Fatalf("file row width = %d, want %d: %q", width, model.width, row)
			}
		})
	}
}
