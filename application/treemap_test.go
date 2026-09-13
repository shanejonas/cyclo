package application

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/domain"
)

func TestTreemapAreaTracksCyclomaticAndKeepsFunctionsInsideFiles(t *testing.T) {
	model := treemapModel()
	chart := model.complexityTreemap(40)
	if chart.total != 16 || chart.peak != 8 || chart.count != 3 {
		t.Fatalf("total/peak/count = %d/%d/%d", chart.total, chart.peak, chart.count)
	}
	if chart.cognitiveTotal != 10 || chart.cognitivePeak != 6 {
		t.Fatalf("cognitive total/peak = %d/%d", chart.cognitiveTotal, chart.cognitivePeak)
	}
	covered := map[image.Point]bool{}
	functions := 0
	for _, file := range chart.files {
		area := file.bounds.Dx() * file.bounds.Dy()
		want := float64(40*treemapHeight*2*file.weight) / float64(chart.total)
		if math.Abs(float64(area)-want) > float64(treemapHeight*2) {
			t.Fatalf("file area = %d, want near %.1f", area, want)
		}
		for y := file.bounds.Min.Y; y < file.bounds.Max.Y; y++ {
			for x := file.bounds.Min.X; x < file.bounds.Max.X; x++ {
				point := image.Pt(x, y)
				if covered[point] {
					t.Fatalf("files overlap at %v", point)
				}
				covered[point] = true
			}
		}
		content := file.bounds.Inset(1)
		childArea := 0
		for i, function := range file.children {
			if function.weight <= 0 || !function.bounds.In(content) {
				t.Fatalf("invalid function tile: %+v", function)
			}
			area := function.bounds.Dx() * function.bounds.Dy()
			want := float64(content.Dx()*content.Dy()*function.weight) / float64(file.weight)
			if math.Abs(float64(area)-want) > float64(max(content.Dx(), content.Dy())) {
				t.Fatalf("function area = %d, want near %.1f", area, want)
			}
			for _, other := range file.children[:i] {
				if function.bounds.Overlaps(other.bounds) {
					t.Fatal("function tiles overlap")
				}
			}
			childArea += area
			functions++
		}
		if childArea != content.Dx()*content.Dy() {
			t.Fatal("function tiles leave a hole in their file")
		}
	}
	if len(covered) != 40*treemapHeight*2 || functions != 3 {
		t.Fatalf("coverage/functions = %d/%d", len(covered), functions)
	}
}

func TestTreemapClicksSelectFilesAndFunctionsAfterResizing(t *testing.T) {
	for _, width := range []int{100, 120, 181} {
		chartWidth := analyticsWidths(width)[0]
		for _, target := range []struct {
			y     int
			focus pane
		}{{3, filesPane}, {8, functionsPane}} {
			model := treemapModel()
			model.width = width
			model.sourceOffset, model.sourceCursor = 4, 5
			model.lineSelection = &LineSelection{StartLine: 1, EndLine: 2}
			model = updateSourceModel(t, model, tea.MouseClickMsg{X: chartWidth - 3, Y: target.y, Button: tea.MouseLeft})
			if model.fileIndex != 1 || model.functionIndex != 0 || model.focus != target.focus {
				t.Fatalf("width=%d y=%d: selected %d/%d/%d", width, target.y, model.fileIndex, model.functionIndex, model.focus)
			}
			if model.revision != 1 || model.sourceOffset != 0 || model.sourceCursor != 0 || model.lineSelection != nil {
				t.Fatal("tile selection did not reset source navigation and advance the revision")
			}
			if !strings.Contains(ansi.Strip(model.View().Content), "func second()") {
				t.Fatal("tile selection did not reveal the selected source")
			}
		}
	}
}

func TestTreemapIgnoresClicksOutsideMapsAndWhileAnnotating(t *testing.T) {
	for _, point := range [][2]int{{0, 0}, {0, 2}, {85, 8}, {86, 8}, {87, 8}, {88, 8}, {90, 8}, {1, 14}, {1, 20}} {
		model := treemapModel()
		model = updateSourceModel(t, model, tea.MouseClickMsg{X: point[0], Y: point[1], Button: tea.MouseLeft})
		if model.revision != 0 {
			t.Fatalf("click %v outside the map changed selection", point)
		}
	}
	for _, size := range [][2]int{{60, 30}, {120, 24}, {120, 30}} {
		model := treemapModel()
		model.width, model.height = size[0], size[1]
		model.annotating = size[0] == 120 && size[1] == 30
		model.annotationDraft = "keep this note"
		model = updateSourceModel(t, model, tea.MouseClickMsg{X: 38, Y: 8, Button: tea.MouseLeft})
		if model.revision != 0 || model.annotationDraft != "keep this note" {
			t.Fatal("hidden chart or annotation input accepted a chart click")
		}
	}
	model := treemapModel()
	model = updateSourceModel(t, model, tea.MouseClickMsg{X: 38, Y: 8, Button: tea.MouseRight})
	if model.revision != 0 {
		t.Fatal("right click changed selection")
	}
}

func TestTreemapOutlineFollowsKeyboardSelectionWithoutChangingLayout(t *testing.T) {
	model := treemapModel()
	moved := pressSourceKey(t, model, "j")
	chart := model.complexityTreemap(41)
	before := chart.raster(model.fileIndex, model.functionIndex)
	after := chart.raster(moved.fileIndex, moved.functionIndex)
	first := chart.files[0].children[0].bounds.Min
	second := chart.files[1].children[0].bounds.Min
	if before.RGBAAt(first.X, first.Y) != treemapSelection || after.RGBAAt(first.X, first.Y) == treemapSelection {
		t.Fatal("previous function outline did not move")
	}
	if after.RGBAAt(second.X, second.Y) != treemapSelection {
		t.Fatal("selected function has no white outline")
	}
	for _, point := range []image.Point{first, second} {
		old, _ := chart.tileAt(point)
		next, _ := moved.complexityTreemap(41).tileAt(point)
		if old.bounds != next.bounds || old.file != next.file || old.function != next.function {
			t.Fatal("selection rearranged the map")
		}
	}
}

func TestTreemapZeroCognitiveStaysSelectableAndGreen(t *testing.T) {
	model := treemapModel()
	chart := model.complexityTreemap(41)
	var zero *treemapTile
	for _, file := range chart.files {
		for i := range file.children {
			if file.children[i].cognitive == 0 {
				zero = &file.children[i]
			}
		}
	}
	if zero == nil {
		t.Fatal("no zero-cognitive function found")
	}
	if zero.weight <= 0 {
		t.Fatal("zero-cognitive function has no area")
	}
	if _, ok := chart.tileAt(zero.bounds.Min); !ok {
		t.Fatal("zero-cognitive function is not selectable")
	}
	canvas := chart.raster(0, 0)
	got := canvas.RGBAAt(zero.bounds.Min.X+1, zero.bounds.Min.Y+1)
	if got.G <= got.R || got.G <= got.B {
		t.Fatalf("zero-cognitive tile is not green: %v", got)
	}
}

func TestTreemapEmptyReportHasNoTargets(t *testing.T) {
	for _, model := range []Model{{}, {width: 41}} {
		chart := model.complexityTreemap(41)
		if len(chart.files) != 0 || chart.total != 0 {
			t.Fatal("empty report produced nonzero tiles")
		}
		if !strings.Contains(ansi.Strip(strings.Join(chart.render(0, 0), "\n")), "No complexity to map") {
			t.Fatal("empty map has no empty state")
		}
		if _, ok := chart.tileAt(image.Pt(1, 1)); ok {
			t.Fatal("empty map has a click target")
		}
	}
}

func TestTreemapCognitiveChangesColorNotArea(t *testing.T) {
	model := treemapModel()
	base := model.complexityTreemap(41)
	model.report.Files[0].Functions[1].CognitiveComplexity = 25
	changed := model.complexityTreemap(41)
	if base.total != changed.total || base.peak != changed.peak {
		t.Fatal("cognitive change altered area")
	}
	for i := range base.files {
		if base.files[i].bounds != changed.files[i].bounds {
			t.Fatal("cognitive change altered geometry")
		}
		for j := range base.files[i].children {
			if base.files[i].children[j].bounds != changed.files[i].children[j].bounds {
				t.Fatal("cognitive change altered function geometry")
			}
		}
	}
	point := base.files[0].children[1].bounds.Min.Add(image.Pt(1, 1))
	if base.raster(0, 0).RGBAAt(point.X, point.Y) == changed.raster(0, 0).RGBAAt(point.X, point.Y) {
		t.Fatal("cognitive change did not change the rendered color")
	}
}

func TestTreemapSameCognitiveAcrossFilesSameColor(t *testing.T) {
	model := treemapModel()
	model.report.Files[0].Functions = model.report.Files[0].Functions[:1]
	model.report.Files[0].Functions[0].Complexity = 4
	model.report.Files[0].Functions[0].CognitiveComplexity = 15
	model.report.Files[1].Functions[0].CognitiveComplexity = 15
	chart := model.complexityTreemap(40)
	canvas := chart.raster(0, 0)
	first := chart.files[0].children[0].bounds.Min
	second := chart.files[1].children[0].bounds.Min
	if canvas.RGBAAt(first.X+1, first.Y+1) != canvas.RGBAAt(second.X+1, second.Y+1) {
		t.Fatal("same cognitive across files produced different colors")
	}
}

func TestTreemapSelectionOnlyChangesOutline(t *testing.T) {
	model := treemapModel()
	chart := model.complexityTreemap(41)
	before := chart.raster(0, 0)
	after := chart.raster(1, 0)
	selected := chart.files[1].children[0].bounds.Min
	if before.RGBAAt(selected.X, selected.Y) == treemapSelection {
		t.Fatal("unselected tile already outlined")
	}
	if after.RGBAAt(selected.X, selected.Y) != treemapSelection {
		t.Fatal("selected tile not outlined")
	}
	unselected := chart.files[0].children[0].bounds.Min
	if before.RGBAAt(unselected.X+1, unselected.Y+1) != after.RGBAAt(unselected.X+1, unselected.Y+1) {
		t.Fatal("selection recolored another tile")
	}
}

func TestTreemapColorScaleMatchesLegendAndClampsExtremes(t *testing.T) {
	for _, test := range []struct {
		score int
		want  color.RGBA
	}{
		{-1, color.RGBA{102, 166, 120, 255}},
		{0, color.RGBA{102, 166, 120, 255}},
		{5, color.RGBA{161, 180, 122, 255}},
		{10, color.RGBA{221, 194, 125, 255}},
		{20, color.RGBA{212, 112, 60, 255}},
		{30, color.RGBA{209, 105, 105, 255}},
		{1000, color.RGBA{209, 105, 105, 255}},
	} {
		if got := treemapColor(test.score); got != test.want {
			t.Fatalf("COG %d: color = %v, want %v", test.score, got, test.want)
		}
	}
}

func TestTreemapTinyTilesStayInsideCanvas(t *testing.T) {
	model := treemapModel()
	for range 200 {
		model.report.Files[0].Functions = append(model.report.Files[0].Functions, domain.Function{Complexity: 1})
	}
	for _, width := range []int{1, 2, 5, 31} {
		chart := model.complexityTreemap(width)
		canvas := chart.raster(0, 0)
		for y := range treemapHeight * 2 {
			for x := range width {
				tile, ok := chart.tileAt(image.Pt(x, y))
				if !ok || tile.weight <= 0 || canvas.RGBAAt(x, y).A != 255 {
					t.Fatalf("width=%d: missing tile at %d/%d", width, x, y)
				}
			}
		}
	}
}

func TestTreemapLayoutFitsAndSourceCursorRemainsVisible(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {120, 40}, {80, 24}, {60, 14}} {
		model := treemapModel()
		model.width, model.height = size[0], size[1]
		model.focus = detailsPane
		lines := make([]string, 60)
		for i := range lines {
			lines[i] = fmt.Sprintf("line %d", i+1)
		}
		model.report.Files[0].Functions[0].Source = strings.Join(lines, "\n")
		model.report.Files[0].Functions[0].EndLine = 60
		for range 50 {
			model = pressSourceKey(t, model, "j")
		}
		view := ansi.Strip(model.View().Content)
		if !strings.Contains(view, "line 51") {
			t.Fatalf("size %v hides source cursor:\n%s", size, view)
		}
		rows := strings.Split(view, "\n")
		if len(rows) != model.height {
			t.Fatalf("view height = %d, want %d", len(rows), model.height)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > model.width {
				t.Fatalf("view overflows %d columns: %q", model.width, row)
			}
		}
	}
}

func treemapModel() Model {
	return Model{
		width: 120, height: 30,
		report: domain.Report{
			Root: "/workspace", Functions: 3, Total: 16, CognitiveTotal: 10,
			Files: []domain.File{
				{Path: "first.go", Total: 12, CognitiveTotal: 6, Functions: []domain.Function{
					{Name: "nested", Complexity: 8, CognitiveComplexity: 6, Line: 1},
					{Name: "flat", Complexity: 4, CognitiveComplexity: 0, Line: 4},
				}},
				{Path: "second.go", Total: 4, CognitiveTotal: 4, Functions: []domain.Function{
					{Name: "second", Complexity: 4, CognitiveComplexity: 4, Line: 1, Source: "func second() {}"},
				}},
			},
		},
	}
}
