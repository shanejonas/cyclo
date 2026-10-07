package bugreducer

import (
	"fmt"
	"strings"
	"time"
)

func (m dashboard) sizeChart(width, height int) []string {
	columns, rows := max(width-6, 1), max(height-2, 2)
	duration := max(m.now.Sub(m.started), time.Millisecond)
	lastColumn := max(columns-1, 1)
	emptyRow := strings.Repeat(" ", columns)
	grid := make([][]rune, rows)
	for i := range grid {
		grid[i] = []rune(emptyRow)
	}
	point := 0
	for x := 0; x < columns; x++ {
		at := time.Duration(float64(duration) * float64(x) / float64(lastColumn))
		for point+1 < len(m.history) && m.history[point+1].elapsed <= at {
			point++
		}
		y := chartRow(m.history[point].bytes, m.original, rows)
		grid[y][x] = '●'
	}
	lines := make([]string, rows)
	for y, row := range grid {
		label := fmt.Sprintf("%3d%%│", 100-y*100/(rows-1))
		lines[y] = label + dashboardGreen.Render(string(row))
	}
	return append(lines, "    └"+strings.Repeat("─", columns), fmt.Sprintf("     0s → %s · %% of original", duration.Round(time.Second)))
}

func chartRow(size, original, rows int) int {
	if original == 0 {
		return rows - 1
	}
	return min(rows-1, max(0, (original-size)*(rows-1)/original))
}
