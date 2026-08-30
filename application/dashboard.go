package application

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/shanejonas/cyclomatic-complexity-tui/domain"
)

var digitRows = map[rune][5]string{
	'0': {"█████", "█   █", "█   █", "█   █", "█████"},
	'1': {"  █  ", " ██  ", "  █  ", "  █  ", "█████"},
	'2': {"█████", "    █", "█████", "█    ", "█████"},
	'3': {"█████", "    █", "█████", "    █", "█████"},
	'4': {"█   █", "█   █", "█████", "    █", "    █"},
	'5': {"█████", "█    ", "█████", "    █", "█████"},
	'6': {"█████", "█    ", "█████", "█   █", "█████"},
	'7': {"█████", "    █", "   █ ", "  █  ", "  █  "},
	'8': {"█████", "█   █", "█████", "█   █", "█████"},
	'9': {"█████", "█   █", "█████", "    █", "█████"},
}

func largeNumber(value int) []string {
	digits := strconv.Itoa(max(value, 0))
	lines := make([]string, 5)
	for _, digit := range digits {
		rows := digitRows[digit]
		for row := range lines {
			if lines[row] != "" {
				lines[row] += " "
			}
			lines[row] += rows[row]
		}
	}

	return lines
}

func reportPeak(report domain.Report) int {
	peak := 0
	for _, file := range report.Files {
		peak = max(peak, file.Peak)
	}

	return peak
}

func cognitivePeak(report domain.Report) int {
	peak := 0
	for _, file := range report.Files {
		peak = max(peak, file.CognitivePeak)
	}

	return peak
}

func complexityDistribution(report domain.Report) [3]int {
	counts := [3]int{}
	for _, file := range report.Files {
		for _, function := range file.Functions {
			switch {
			case function.Complexity <= 5:
				counts[0]++
			case function.Complexity <= 10:
				counts[1]++
			default:
				counts[2]++
			}
		}
	}

	return counts
}

func complexityBar(value int, maximum int, width int) string {
	if width <= 0 {
		return ""
	}
	if value <= 0 || maximum <= 0 {
		return strings.Repeat("░", width)
	}

	filled := min((value*width+maximum-1)/maximum, width)
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (m Model) analyticsLines(width int) []string {
	peak := reportPeak(m.report)
	load := cognitivePeak(m.report)
	scoreWidth := min(max(width/6, 18), 24)
	graphWidth := max(width-scoreWidth*2-6, 1)

	score := []string{blue.Render("CC PEAK · PATHS")}
	for _, line := range largeNumber(peak) {
		score = append(score, metricStyle(peak).Bold(true).Render(line))
	}
	score = append(score, metricStyle(peak).Bold(true).Render(complexityStatus(peak)))
	cognitiveScore := []string{cognitive.Render("COG PEAK · LOAD")}
	for _, line := range largeNumber(load) {
		cognitiveScore = append(cognitiveScore, cognitive.Bold(true).Render(line))
	}
	cognitiveScore = append(cognitiveScore, cognitive.Bold(true).Render("MENTAL LOAD"))

	counts := complexityDistribution(m.report)
	maximum := max(counts[0], counts[1], counts[2])
	graph := []string{
		blue.Render("CC function distribution"),
		fmt.Sprintf(
			"%s %s %s  %s %s  ·  %s %s %s  %s %s",
			blue.Render("CC"),
			muted.Render("TOTAL"),
			text.Render(strconv.Itoa(m.report.Total)),
			muted.Render("AVG"),
			text.Render(fmt.Sprintf("%.1f", m.report.Average)),
			cognitive.Render("COG"),
			muted.Render("TOTAL"),
			text.Render(strconv.Itoa(m.report.CognitiveTotal)),
			muted.Render("AVG"),
			text.Render(fmt.Sprintf("%.1f", m.report.CognitiveAverage)),
		),
		"",
		distributionLine("LOW", "1–5", counts[0], maximum, graphWidth, green),
		distributionLine("WATCH", "6–10", counts[1], maximum, graphWidth, amber),
		distributionLine("HIGH", "11+", counts[2], maximum, graphWidth, danger),
		muted.Render(fmt.Sprintf("%d files · %d functions", len(m.report.Files), m.report.Functions)),
	}

	return joinedRows(
		[][]string{score, cognitiveScore, graph},
		[]int{scoreWidth, scoreWidth, graphWidth},
	)
}

func distributionLine(
	label string,
	rangeLabel string,
	count int,
	maximum int,
	width int,
	style lipgloss.Style,
) string {
	barWidth := max(width-23, 4)
	bar := style.Render(complexityBar(count, maximum, barWidth))
	return fmt.Sprintf("%-6s %-5s %s %d", label, rangeLabel, bar, count)
}

func complexityStatus(value int) string {
	if value > 10 {
		return "HIGH"
	}
	if value > 5 {
		return "WATCH"
	}

	return "LOW"
}
