package application

import (
	"image"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

type treemapTile struct {
	bounds            image.Rectangle
	file, function    int
	weight, cognitive int
	children          []treemapTile
}

type treemap struct {
	files                         []treemapTile
	total, peak                   int
	cognitiveTotal, cognitivePeak int
	count                         int
	width                         int
}

var treemapSelection = color.RGBA{R: 237, G: 242, B: 243, A: 255}

var treemapFileBorder = color.RGBA{R: 58, G: 70, B: 74, A: 255}

// treemapColor interpolates green→yellow→orange→red across cognitive complexity.
func treemapColor(score int) color.RGBA {
	colors := [...]color.RGBA{
		{R: 102, G: 166, B: 120, A: 255},
		{R: 221, G: 194, B: 125, A: 255},
		{R: 212, G: 112, B: 60, A: 255},
		{R: 209, G: 105, B: 105, A: 255},
	}
	score = min(max(score, 0), 30)
	index := min(score/10, len(colors)-2)
	low, high := colors[index], colors[index+1]
	fraction := float64(score-index*10) / 10
	return color.RGBA{
		R: uint8(float64(low.R) + fraction*float64(int(high.R)-int(low.R))),
		G: uint8(float64(low.G) + fraction*float64(int(high.G)-int(low.G))),
		B: uint8(float64(low.B) + fraction*float64(int(high.B)-int(low.B))), A: 255,
	}
}

func (m Model) complexityTreemap(width int) treemap {
	chart := treemap{width: width}
	for fileIndex, file := range m.report.Files {
		tile := treemapTile{file: fileIndex, function: -1}
		for functionIndex, function := range file.Functions {
			value := function.Complexity
			chart.count++
			chart.peak = max(chart.peak, value)
			chart.cognitiveTotal += function.CognitiveComplexity
			chart.cognitivePeak = max(chart.cognitivePeak, function.CognitiveComplexity)
			if value <= 0 {
				continue
			}
			tile.children = append(tile.children, treemapTile{
				file: fileIndex, function: functionIndex,
				weight: value, cognitive: function.CognitiveComplexity,
			})
			tile.weight += value
		}
		if tile.weight > 0 {
			chart.files = append(chart.files, tile)
			chart.total += tile.weight
		}
	}
	layoutTreemap(chart.files, image.Rect(0, 0, width, treemapHeight*2), chart.total)
	return chart
}

// Bisect the longest side near half the weight to keep tiles compact.
func layoutTreemap(tiles []treemapTile, bounds image.Rectangle, total int) {
	if len(tiles) == 0 || bounds.Empty() {
		return
	}
	if len(tiles) == 1 {
		tiles[0].bounds = bounds
		if min(bounds.Dx(), bounds.Dy()) >= 5 {
			bounds = bounds.Inset(1)
		}
		layoutTreemap(tiles[0].children, bounds, total)
		return
	}
	index, weight := treemapPartition(tiles, total)
	first, second := splitTreemapBounds(bounds, float64(weight)/float64(total))
	layoutTreemap(tiles[:index], first, weight)
	layoutTreemap(tiles[index:], second, total-weight)
}

func treemapPartition(tiles []treemapTile, total int) (int, int) {
	index, weight := 1, tiles[0].weight
	for index < len(tiles)-1 && math.Abs(float64(total-2*(weight+tiles[index].weight))) < math.Abs(float64(total-2*weight)) {
		weight += tiles[index].weight
		index++
	}
	return index, weight
}

func splitTreemapBounds(bounds image.Rectangle, fraction float64) (image.Rectangle, image.Rectangle) {
	first, second := bounds, bounds
	if bounds.Dx() >= bounds.Dy() {
		split := bounds.Min.X + int(math.Round(float64(bounds.Dx())*fraction))
		first.Max.X, second.Min.X = split, split
		return first, second
	}
	split := bounds.Min.Y + int(math.Round(float64(bounds.Dy())*fraction))
	first.Max.Y, second.Min.Y = split, split
	return first, second
}

func (s treemap) tileAt(point image.Point) (treemapTile, bool) {
	for _, file := range s.files {
		if !point.In(file.bounds) {
			continue
		}
		for _, function := range file.children {
			if point.In(function.bounds) {
				return function, true
			}
		}
		return file, true
	}
	return treemapTile{}, false
}

func (s treemap) raster(file int, function int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, s.width, treemapHeight*2))
	for _, tile := range s.files {
		paintTreemap(canvas, tile, file, function)
	}
	return canvas
}

func (s treemap) render(file int, function int) []string {
	if s.total == 0 {
		lines := make([]string, treemapHeight)
		lines[treemapHeight/2] = muted.Render("No complexity to map")
		return lines
	}
	canvas := s.raster(file, function)
	lines := make([]string, treemapHeight)
	for row := range lines {
		var line strings.Builder
		for column := range s.width {
			top := canvas.RGBAAt(column, row*2)
			bottom := canvas.RGBAAt(column, row*2+1)
			line.WriteString(lipgloss.NewStyle().Foreground(top).Background(bottom).Render("▀"))
		}
		lines[row] = line.String()
	}
	return lines
}

func paintTreemap(canvas *image.RGBA, tile treemapTile, file int, function int) {
	base := treemapFileBorder
	if tile.function >= 0 {
		base = treemapColor(tile.cognitive)
	}
	selected := tile.file == file && tile.function == function
	for y := tile.bounds.Min.Y; y < tile.bounds.Max.Y; y++ {
		for x := tile.bounds.Min.X; x < tile.bounds.Max.X; x++ {
			canvas.SetRGBA(x, y, tile.pixel(image.Pt(x, y), base, selected))
		}
	}
	for _, child := range tile.children {
		paintTreemap(canvas, child, file, function)
	}
}

func (t treemapTile) pixel(point image.Point, base color.RGBA, selected bool) color.RGBA {
	x, y := point.X-t.bounds.Min.X, point.Y-t.bounds.Min.Y
	w, h := t.bounds.Dx(), t.bounds.Dy()
	if selected && min(x, y, w-1-x, h-1-y) == 0 {
		return treemapSelection
	}
	if min(w, h) > 2 && min(x, y, w-1-x, h-1-y) == 0 {
		return shadeTreemap(base, 0.35)
	}
	dx := (float64(x) + 0.5) / float64(w)
	dy := (float64(y) + 0.5) / float64(h)
	cushion := math.Sqrt(16 * dx * (1 - dx) * dy * (1 - dy))
	return shadeTreemap(base, 0.45+0.55*cushion+0.2*(1-dx-dy))
}

func shadeTreemap(c color.RGBA, shade float64) color.RGBA {
	return color.RGBA{
		R: uint8(min(float64(c.R)*shade, 255)),
		G: uint8(min(float64(c.G)*shade, 255)),
		B: uint8(min(float64(c.B)*shade, 255)), A: 255,
	}
}
