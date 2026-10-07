package qualitycheck

import (
	"strings"

	"github.com/shanejonas/cyclo/domain/quality"
	"github.com/shanejonas/cyclo/internal/gitchanged"
)

// touchedFunctions returns the set of fact keys (path + name) for functions
// whose line range intersects a diff touch range.
func touchedFunctions(facts []quality.Function, ranges map[string][]gitchanged.LineRange, root, cwd string) map[string]bool {
	result := map[string]bool{}
	for _, fact := range facts {
		rel := gitchanged.RootRelative(fact.Location.Path, root, cwd)
		fileRanges, ok := ranges[rel]
		if !ok {
			continue
		}
		start := fact.Location.Line
		end := start + strings.Count(fact.Source, "\n")
		for _, r := range fileRanges {
			if gitchanged.Overlaps(start, end, r) {
				result[fact.Location.Path+"\x00"+fact.Location.Name] = true
				break
			}
		}
	}
	return result
}

// filterChanged keeps only diagnostics whose function the diff touches,
// then rebuilds the work plan over the remaining findings.
func filterChanged(report quality.Report, facts []quality.Function, touched map[string]bool, config quality.Config) (quality.Report, error) {
	diagnostics := make([]quality.Diagnostic, 0, len(report.Diagnostics))
	for _, diagnostic := range report.Diagnostics {
		if touched[diagnostic.Path+"\x00"+diagnostic.Name] {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	report.Diagnostics = diagnostics
	groups, err := quality.BuildFixGroups(facts, diagnostics, config)
	if err != nil {
		return report, err
	}
	report.FixGroups = groups
	return report, nil
}
