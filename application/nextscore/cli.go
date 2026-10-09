package nextscore

import (
	"context"
	"fmt"
	"io"

	"github.com/shanejonas/cyclo/application/patterncheck"
	"github.com/shanejonas/cyclo/domain/patterns"
)

// Run implements `cyclo next`: show the single highest-impact pattern
// candidate with fix instructions. Agent-focused: one thing, not a firehose.
func Run(ctx context.Context, args []string, output io.Writer) error {
	report, err := patterncheck.GetReport(ctx, args)
	if err != nil {
		return err
	}
	if len(report.Candidates) == 0 {
		fmt.Fprintln(output, "No pattern candidates. Codebase is clean.")
		return nil
	}
	top := report.Candidates[0]
	fmt.Fprintf(output, "Next: %s (score %.2f)\n", top.Kind, float64(top.ScoreMilli)/1000)
	fmt.Fprintf(output, "  %s\n", top.Observation)
	fmt.Fprintf(output, "  %s\n", top.Inference)
	for _, s := range top.Sites {
		fmt.Fprintf(output, "  %s:%d %s\n", s.Path, s.Line, s.Name)
	}
	fmt.Fprintf(output, "\nFix with: cyclo fix --kind %s %s\n", top.Kind, top.Sites[0].Path)
	return nil
}

// Score implements `cyclo score`: a single 0-100 north-star number.
// 100 = clean, lower = more issues. Weighted by severity.
func Score(ctx context.Context, args []string, output io.Writer) error {
	report, err := patterncheck.GetReport(ctx, args)
	if err != nil {
		return err
	}
	score := computeScore(report)
	fmt.Fprintf(output, "Score: %d/100\n", score)
	fmt.Fprintf(output, "  %d active candidates", len(report.Candidates))
	if len(report.Suppressed) > 0 {
		fmt.Fprintf(output, ", %d suppressed", len(report.Suppressed))
	}
	fmt.Fprintln(output)
	return nil
}

// computeScore maps candidates to 0-100. Each candidate deducts points
// weighted by severity (score). Starts at 100, floors at 0.
func computeScore(report *patterns.PatternsReport) int {
	score := 100
	for _, c := range report.Candidates {
		// Severity weight: score 0.8 deducts 8 points, 0.4 deducts 4, etc.
		deduction := int(float64(c.ScoreMilli) / 100)
		score -= deduction
	}
	if score < 0 {
		score = 0
	}
	return score
}
