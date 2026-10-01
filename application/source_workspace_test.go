package application

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/domain"
)

func TestSourcePaneMovesALineCursorAndKeepsItVisible(t *testing.T) {
	model := sourceWorkspaceModel()
	for range 7 {
		model = pressSourceKey(t, model, "j")
	}

	if model.sourceLine() != 17 {
		t.Fatalf("source line = %d, want 17", model.sourceLine())
	}
	if model.sourceOffset == 0 {
		t.Fatal("source cursor moved below the viewport without scrolling")
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "›   17 │") {
		t.Fatalf("source cursor is not visible:\n%s", ansi.Strip(model.View().Content))
	}
}

func TestSourcePaneSelectsAndAnnotatesLines(t *testing.T) {
	model := sourceWorkspaceModel()
	model = pressSourceKey(t, model, "v")
	model = pressSourceKey(t, model, "j")
	model = pressSourceKey(t, model, "j")

	if !model.visualSelectionActive || model.lineSelection == nil {
		t.Fatal("visual line selection did not start")
	}
	if model.lineSelection.StartLine != 10 || model.lineSelection.EndLine != 12 {
		t.Fatalf("line selection = %#v, want lines 10-12", model.lineSelection)
	}
	if !strings.Contains(model.View().Content, "48;2;45;52;54") {
		t.Fatal("visual selection does not highlight its source lines")
	}

	model = pressSourceKey(t, model, "a")
	model = updateSourceModel(t, model, tea.KeyPressMsg{Code: 'n', Text: "needs work"})
	model = updateSourceModel(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(model.annotations) != 1 || model.annotations[0].Message != "needs work" {
		t.Fatalf("annotations = %#v, want one note", model.annotations)
	}
	if model.visualSelectionActive || model.annotating {
		t.Fatal("saving a note left an input mode active")
	}

	rendered := model.View().Content
	if !strings.Contains(ansi.Strip(rendered), "◆ needs work") {
		t.Fatalf("annotation is not shown beside its source:\n%s", ansi.Strip(rendered))
	}
	if !strings.Contains(rendered, "38;2;221;194;125") {
		t.Fatalf("annotation is not amber:\n%s", rendered)
	}
	if model.lineSelection != nil {
		t.Fatalf("saving an annotation left its source selected: %#v", model.lineSelection)
	}
	if !strings.Contains(model.View().Content, "48;2;44;34;14") {
		t.Fatalf("saved annotation does not highlight its source range:\n%q", model.View().Content)
	}
}

func TestSourcePaneStacksDistinctAnnotationRows(t *testing.T) {
	model := sourceWorkspaceModel()
	first, _ := model.newAnnotation(11, 16, "agent note")
	second, _ := model.newAnnotation(15, 16, "my note")
	model.annotations = []Annotation{second, first}
	model.activeAnnotationID = second.ID

	rendered := strings.Join(model.sourceLines(90, 30, "Source"), "\n")
	plain := ansi.Strip(rendered)
	for _, want := range []string{
		"Source · ◆ 2 NOTES",
		"╰─◆ 1/2 agent note",
		"╰─◆ 2/2 my note",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("source does not show %q:\n%s", want, plain)
		}
	}
	if strings.Count(plain, "agent note") != 1 || strings.Count(plain, "my note") != 1 {
		t.Fatalf("annotation messages are duplicated or hidden:\n%s", plain)
	}
	if strings.Contains(rendered, "48;2;20;40;30") {
		t.Fatalf("active annotation row uses a background:\n%q", rendered)
	}
	if !strings.Contains(rendered, "48;2;44;34;14") {
		t.Fatalf("annotation range does not use an amber background:\n%q", rendered)
	}
}

func TestAnnotationsUseBackgroundInsteadOfAGutterRail(t *testing.T) {
	model := sourceWorkspaceModel()
	annotation, _ := model.newAnnotation(11, 16, "note")
	model.annotations = []Annotation{annotation}

	gutter, _ := model.sourceGutter(12, muted)
	if ansi.Strip(gutter) != "  " {
		t.Fatalf("annotation gutter = %q, want two blank cells", ansi.Strip(gutter))
	}
	if !strings.Contains(model.View().Content, "48;2;44;34;14") {
		t.Fatal("annotation range does not have an amber background")
	}
}

func TestAnnotationRowsWrapWithoutLosingText(t *testing.T) {
	model := sourceWorkspaceModel()
	message := "carry bundle ids and align metadata to the selected attempt before merging"
	annotation, _ := model.newAnnotation(11, 16, message)
	model.annotations = []Annotation{annotation}

	rows := sourceAnnotationRows(model.visibleAnnotations(), 16, 32)
	if len(rows) < 2 {
		t.Fatalf("annotation stayed on one row: %q", ansi.Strip(strings.Join(rows, "\n")))
	}
	plain := ansi.Strip(strings.Join(rows, " "))
	for _, word := range strings.Fields(message) {
		if !strings.Contains(plain, word) {
			t.Fatalf("wrapped annotation lost %q: %q", word, plain)
		}
	}
	for _, row := range rows {
		if width := ansi.StringWidth(row); width > 32 {
			t.Fatalf("wrapped row width = %d, want <= 32: %q", width, ansi.Strip(row))
		}
	}
}

func TestSourceScrollExtentMatchesRenderedNotesAndDeletions(t *testing.T) {
	for _, width := range []int{14, 30, 32, 50, 90, 140} {
		model := sourceWorkspaceModel()
		model.width = width
		function := &model.report.Files[0].Functions[0]
		function.EndLine = 22
		function.DiffLines = []domain.DiffLine{
			{Kind: domain.DiffDeleted, NewLine: 11, OldLine: 11, Text: "deleted"},
			{Kind: domain.DiffAdded, NewLine: 12, Text: "added"},
			{Kind: domain.DiffDeleted, NewLine: 23, OldLine: 24, Text: "trailing deletion"},
		}
		first, _ := model.newAnnotation(11, 11, strings.Repeat("語 ", 30))
		second, _ := model.newAnnotation(11, 11, "second note\nwith another line")
		last, _ := model.newAnnotation(22, 22, "last note")
		other := first
		other.Path = "other.go"
		model.annotations = []Annotation{last, first, other, second}
		code := model.selectedSourceCodeLines(*function)
		for start := range code {
			model.sourceOffset = start
			rows := model.sourceDisplayWindow(code, *function, paneContentWidth(model.sourcePaneWidth()), 1000)
			if count := model.sourceDisplayRowCount(start, len(code)-1); count != len(rows) {
				t.Fatalf("width %d, start %d: scroll extent %d, rendered rows %d", width, start, count, len(rows))
			}
		}
	}
}

func TestCursorScrollingFitsRenderedRows(t *testing.T) {
	for _, width := range []int{32, 90} {
		for _, height := range []int{2, 12, 24} {
			model := sourceWorkspaceModel()
			model.width, model.height = width, height
			function := &model.report.Files[0].Functions[0]
			function.EndLine = 22
			function.DiffLines = []domain.DiffLine{
				{Kind: domain.DiffDeleted, NewLine: 14, OldLine: 14, Text: "deleted"},
				{Kind: domain.DiffDeleted, NewLine: 23, OldLine: 24, Text: "trailing deletion"},
			}
			note, _ := model.newAnnotation(11, 11, strings.Repeat("wrapped note ", 10))
			last, _ := model.newAnnotation(22, 22, "last note\nsecond row")
			model.annotations = []Annotation{last, note}
			code := model.selectedSourceCodeLines(*function)
			annotations := model.visibleAnnotations()
			visible := model.sourceViewportHeight()
			for cursor := range code {
				for _, offset := range []int{0, 8} {
					model.sourceCursor, model.sourceOffset = cursor, offset
					want := offset
					if visible <= 0 || cursor < offset {
						want = cursor
					}
					for want < cursor {
						rows := 0
						for index := want; index <= cursor; index++ {
							rows += len(sourceRowsAtLine(*function, function.Line+index, code[index], paneContentWidth(model.sourcePaneWidth()), index == len(code)-1, annotations))
						}
						if rows <= visible {
							break
						}
						want++
					}
					if next := model.keepSourceCursorVisible(); next.sourceOffset != want || next.sourceCursor != cursor {
						t.Fatalf("%dx%d cursor %d offset %d: got %d/%d, want %d/%d", width, height, cursor, offset, next.sourceOffset, next.sourceCursor, want, cursor)
					}
				}
			}
		}
	}
}

func TestSourcePaneNavigatesAndRemovesAnnotations(t *testing.T) {
	model := sourceWorkspaceModel()
	first, _ := model.newAnnotation(11, 11, "first")
	second, _ := model.newAnnotation(15, 16, "second")
	model.annotations = []Annotation{first, second}

	model = pressSourceKey(t, model, "]")
	if model.activeAnnotationID != first.ID || model.sourceLine() != 11 {
		t.Fatalf("next annotation = %q at %d, want %q at 11", model.activeAnnotationID, model.sourceLine(), first.ID)
	}
	if model.lineSelection != nil {
		t.Fatalf("annotation navigation selected its source: %#v", model.lineSelection)
	}
	model = pressSourceKey(t, model, "]")
	if model.activeAnnotationID != second.ID || model.sourceLine() != 16 {
		t.Fatalf("next annotation = %q at %d, want %q at 16", model.activeAnnotationID, model.sourceLine(), second.ID)
	}
	model = pressSourceKey(t, model, "d")
	if len(model.annotations) != 1 || model.annotations[0].ID != first.ID {
		t.Fatalf("annotations after delete = %#v, want only %s", model.annotations, first.ID)
	}
}

func TestAnnotationNavigationWorksFromEveryPane(t *testing.T) {
	model := sourceWorkspaceModel()
	annotation, _ := model.newAnnotation(15, 16, "nested branch")
	model.annotations = []Annotation{annotation}
	model.focus = filesPane

	model = pressSourceKey(t, model, "]")

	if model.focus != detailsPane {
		t.Fatalf("focus = %v, want source", model.focus)
	}
	if model.activeAnnotationID != annotation.ID || model.sourceLine() != 16 {
		t.Fatalf("annotation = %q at %d, want %q at 16", model.activeAnnotationID, model.sourceLine(), annotation.ID)
	}
}

func TestAnnotationNavigationCrossesFunctionsAndFiles(t *testing.T) {
	model := sourceWorkspaceModel()
	model.report.Files[0].Functions = append(model.report.Files[0].Functions, domain.Function{
		Name: "second", Line: 30, EndLine: 32, Source: "func second() {\n    return\n}",
	})
	model.report.Files = append(model.report.Files, domain.File{
		Path: "other.go",
		Functions: []domain.Function{{
			Name: "third", Line: 50, EndLine: 52, Source: "func third() {\n    return\n}",
		}},
	})
	first := Annotation{
		ID: "annotation-1", Path: "inspect.go", Function: "second", FunctionLine: 30,
		StartLine: 31, EndLine: 31, Message: "second note", Text: "    return",
	}
	last := Annotation{
		ID: "annotation-2", Path: "other.go", Function: "third", FunctionLine: 50,
		StartLine: 51, EndLine: 51, Message: "third note", Text: "    return",
	}
	model.annotations = []Annotation{last, first}
	model.focus = filesPane

	model = pressSourceKey(t, model, "]")
	if model.fileIndex != 0 || model.functionIndex != 1 || model.activeAnnotationID != first.ID {
		t.Fatalf("next annotation = file %d function %d note %q, want 0/1/%q", model.fileIndex, model.functionIndex, model.activeAnnotationID, first.ID)
	}
	model = pressSourceKey(t, model, "]")
	if model.fileIndex != 1 || model.functionIndex != 0 || model.activeAnnotationID != last.ID {
		t.Fatalf("next annotation = file %d function %d note %q, want 1/0/%q", model.fileIndex, model.functionIndex, model.activeAnnotationID, last.ID)
	}

	model.activeAnnotationID = ""
	model.fileIndex = 0
	model.functionIndex = 0
	model = pressSourceKey(t, model, "[")
	if model.fileIndex != 1 || model.functionIndex != 0 || model.activeAnnotationID != last.ID {
		t.Fatalf("previous annotation = file %d function %d note %q, want 1/0/%q", model.fileIndex, model.functionIndex, model.activeAnnotationID, last.ID)
	}
}

func TestFileNavigationWorksFromEveryPane(t *testing.T) {
	model := sourceWorkspaceModel()
	model.report.Files = append(model.report.Files,
		domain.File{Path: "second.go", Functions: []domain.Function{{Name: "second", Line: 1}}},
		domain.File{Path: "third.go", Functions: []domain.Function{{Name: "third", Line: 1}}},
	)
	model.focus = detailsPane

	model = pressSourceKey(t, model, ".")
	if model.fileIndex != 1 || model.focus != detailsPane {
		t.Fatalf("next file = %d with focus %v, want file 1 with source focus", model.fileIndex, model.focus)
	}
	model = pressSourceKey(t, model, ",")
	if model.fileIndex != 0 || model.focus != detailsPane {
		t.Fatalf("previous file = %d with focus %v, want file 0 with source focus", model.fileIndex, model.focus)
	}
}

func TestTablesMarkFilesAndFunctionsWithAnnotations(t *testing.T) {
	model := sourceWorkspaceModel()
	annotated, _ := model.newAnnotation(11, 11, "note")
	model.annotations = []Annotation{annotated}
	model.report.Files = append(model.report.Files,
		domain.File{Path: "plain.go", Functions: []domain.Function{{Name: "plain", Line: 1}}},
	)

	annotatedFile := model.fileRow(model.report.Files[0], 40, false)
	plainFile := model.fileRow(model.report.Files[1], 40, false)
	annotatedFunction := model.functionRow(model.report.Files[0].Functions[0], 40, false)
	plainFunction := model.functionRow(model.report.Files[1].Functions[0], 40, false)

	for name, row := range map[string]string{
		"annotated file":     annotatedFile,
		"annotated function": annotatedFunction,
	} {
		if !strings.Contains(ansi.Strip(row), "◆") {
			t.Fatalf("%s has no annotation marker: %q", name, ansi.Strip(row))
		}
		if !strings.Contains(row, "38;2;221;194;125") {
			t.Fatalf("%s marker is not amber: %q", name, row)
		}
	}
	for name, row := range map[string]string{
		"plain file":     plainFile,
		"plain function": plainFunction,
	} {
		if strings.Contains(ansi.Strip(row), "◆") {
			t.Fatalf("%s has an annotation marker: %q", name, ansi.Strip(row))
		}
	}
}

func TestFooterShowsGlobalAnnotationNavigation(t *testing.T) {
	model := sourceWorkspaceModel()
	model.focus = filesPane

	footer := ansi.Strip(model.footer(120))
	if !strings.Contains(footer, ",/. files") || !strings.Contains(footer, "[/] notes") {
		t.Fatalf("footer does not show global annotation navigation: %q", footer)
	}
}

func sourceWorkspaceModel() Model {
	lines := []string{
		"func inspect() error {", "    value := 1", "    if value > 0 {", "        value++", "    }",
		"    if value > 1 {", "        value++", "    }", "    if value > 2 {", "        value++", "    }",
		"    return nil", "}",
	}
	return Model{
		width: 90, height: 12, focus: detailsPane,
		report: domain.Report{Files: []domain.File{{
			Path: "inspect.go",
			Functions: []domain.Function{{
				Package: "sample", Name: "inspect", Line: 10, EndLine: 21, Source: strings.Join(lines, "\n"),
			}},
		}}},
	}
}

func pressSourceKey(t *testing.T, model Model, key string) Model {
	t.Helper()
	return updateSourceModel(t, model, tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}

func updateSourceModel(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	updated, _ := model.Update(message)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model = %T, want Model", updated)
	}
	return next
}
