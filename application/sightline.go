package application

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	sourceLinePrefixWidth = 9
	sourceOverlayPadding  = 3
)

type sightlinePoint struct {
	x int
	y int
}

func overlayErrorPaths(base []string, source []string, prefixWidth int) []string {
	positions := sourceSightlinePositions(source)
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(base, "\n"))}
	layers = append(layers, sightlineLayers(errorSightline(source, positions), danger, prefixWidth)...)

	return strings.Split(lipgloss.NewCompositor(layers...).Render(), "\n")
}

func sourceSightlinePositions(lines []string) []int {
	positions := make([]int, len(lines))
	for index, line := range lines {
		indentation, body := splitIndent(line)
		if body == "" && index > 0 {
			positions[index] = positions[index-1]
			continue
		}
		positions[index] = sourceOverlayPadding - 1 + len(indentation)
	}

	return positions
}

func errorSightline(lines []string, positions []int) map[sightlinePoint]rune {
	result := map[sightlinePoint]rune{}
	for row, line := range lines {
		if !errorReturn(line) {
			continue
		}
		start, end, found := enclosingErrorGuard(lines, positions, row)
		if !found {
			continue
		}

		column := positions[start]
		result[sightlinePoint{x: column, y: start}] = '╭'
		for row := start + 1; row < end; row++ {
			result[sightlinePoint{x: column, y: row}] = '│'
		}
		result[sightlinePoint{x: column, y: end}] = '╰'
	}

	return result
}

func enclosingErrorGuard(lines []string, positions []int, errorRow int) (int, int, bool) {
	for start := errorRow - 1; start >= 0; start-- {
		if positions[start] >= positions[errorRow] || !guardCondition(lines[start]) {
			continue
		}
		end := errorBlockEnd(lines, positions, start)
		if end >= errorRow {
			return start, end, true
		}
	}

	return 0, 0, false
}

func guardCondition(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasSuffix(line, "{") {
		return false
	}

	return strings.HasPrefix(line, "if ") ||
		strings.HasPrefix(line, "if(") ||
		strings.Contains(line, " else ")
}

func errorBlockEnd(lines []string, positions []int, start int) int {
	for index := start + 1; index < len(lines); index++ {
		if positions[index] == positions[start] && strings.HasPrefix(strings.TrimSpace(lines[index]), "}") {
			return index
		}

	}

	return start
}

func sightlineLayers(
	cells map[sightlinePoint]rune,
	style lipgloss.Style,
	prefixWidth int,
) []*lipgloss.Layer {
	layers := make([]*lipgloss.Layer, 0, len(cells))
	for point, glyph := range cells {
		layer := lipgloss.NewLayer(style.Render(string(glyph))).
			X(prefixWidth + point.x).
			Y(point.y).
			Z(1)
		layers = append(layers, layer)
	}

	return layers
}
