package application_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStartWithTheCurrentRepository(t *testing.T) {
	runProbe(t, "self-scan")
}

func TestRankFilesAndFunctionsByComplexity(t *testing.T) {
	runProbe(t, "ranking")
}

func TestNavigateFilesAndFunctions(t *testing.T) {
	runProbe(t, "navigation")
}

func TestRefreshAnAnalysis(t *testing.T) {
	runProbe(t, "refresh")
}

func TestKeepNarrowTerminalsUsefulAndEasyToLeave(t *testing.T) {
	runProbe(t, "narrow")
}

func runProbe(t *testing.T, scenario string) {
	t.Helper()

	root := moduleRoot(t)
	directory, err := os.MkdirTemp(root, ".complexity-tui-contract-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })

	probe := filepath.Join(directory, "main.go")
	err = os.WriteFile(probe, []byte(probeSource), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "run", probe, scenario)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s public TUI contract failed: %v\n%s", scenario, err, output)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate validation contract")
	}

	return filepath.Dir(filepath.Dir(filename))
}

const probeSource = `package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/adapters/gocyclo"
	"github.com/shanejonas/cyclo/application"
	"github.com/shanejonas/cyclo/domain"
)

type sequenceAnalyzer struct {
	reports []domain.Report
	calls   [][]string
}

func (a *sequenceAnalyzer) Analyze(paths []string) (domain.Report, error) {
	a.calls = append(a.calls, append([]string(nil), paths...))
	if len(a.reports) == 0 {
		return domain.Report{}, errors.New("no report configured")
	}

	index := min(len(a.calls)-1, len(a.reports)-1)
	return a.reports[index], nil
}

type recordingAnalyzer struct {
	delegate gocyclo.Analyzer
	calls    [][]string
	report   domain.Report
}

func (a *recordingAnalyzer) Analyze(paths []string) (domain.Report, error) {
	a.calls = append(a.calls, append([]string(nil), paths...))
	report, err := a.delegate.Analyze(paths)
	a.report = report
	return report, err
}

func main() {
	if len(os.Args) != 2 {
		failf("scenario argument is required")
	}

	var err error
	switch os.Args[1] {
	case "self-scan":
		err = selfScan()
	case "ranking":
		err = ranking()
	case "navigation":
		err = navigation()
	case "refresh":
		err = refresh()
	case "narrow":
		err = narrow()
	default:
		err = fmt.Errorf("unknown scenario %q", os.Args[1])
	}
	if err != nil {
		failf("%v", err)
	}
}

func selfScan() error {
	analyzer := &recordingAnalyzer{delegate: gocyclo.NewAnalyzer()}
	model := application.NewModel(analyzer, nil)
	model, err := load(model, 120, 24)
	if err != nil {
		return err
	}
	if len(analyzer.calls) != 1 || len(analyzer.calls[0]) != 1 || analyzer.calls[0][0] != "." {
		return fmt.Errorf("default scan paths = %v, want [.]", analyzer.calls)
	}
	if len(analyzer.report.Files) == 0 {
		return errors.New("self scan returned no Go files")
	}

	path, function, maximum := maximumProductionComplexity(analyzer.report)
	if maximum > 10 {
		return fmt.Errorf("production complexity ceiling exceeded: %s %s = %d, want <= 10", path, function, maximum)
	}

	view := plain(model.View().Content)
	if !containsAll(view, "CYCLO", "Files", "Functions", "Source") {
		return fmt.Errorf("scan summary is incomplete:\n%s", view)
	}

	return nil
}

func maximumProductionComplexity(report domain.Report) (string, string, int) {
	path := ""
	name := ""
	maximum := 0
	for _, file := range report.Files {
		if strings.HasSuffix(file.Path, "_test.go") {
			continue
		}
		for _, function := range file.Functions {
			if function.Complexity <= maximum {
				continue
			}
			path = file.Path
			name = function.Name
			maximum = function.Complexity
		}
	}

	return path, name, maximum
}

func ranking() error {
	analyzer := &sequenceAnalyzer{reports: []domain.Report{rankedReport()}}
	model := application.NewModel(analyzer, []string{"/workspace"})
	model, err := load(model, 120, 24)
	if err != nil {
		return err
	}
	if len(analyzer.calls) != 1 || len(analyzer.calls[0]) != 1 || analyzer.calls[0][0] != "/workspace" {
		return fmt.Errorf("explicit scan paths = %v, want [/workspace]", analyzer.calls)
	}

	raw := model.View().Content
	view := plain(raw)
	if strings.Index(view, "hot.go") < 0 || strings.Index(view, "hot.go") >= strings.Index(view, "low.go") {
		return fmt.Errorf("files are not ranked by aggregate complexity:\n%s", view)
	}
	if strings.Index(view, "Branchy") < 0 || strings.Index(view, "Branchy") >= strings.Index(view, "Simple") {
		return fmt.Errorf("functions are not ranked by complexity:\n%s", view)
	}
	if !containsAll(view, "hot.go:12:2", "sample · Branchy", "FILE", "SUM", "MAX", "COG", "FUNCTION", "CC", "LINE") {
		return fmt.Errorf("selected detail is incomplete:\n%s", view)
	}
	if !strings.Contains(view, "› hot.go") || !strings.Contains(raw, "38;2;102;166;120") || !strings.Contains(raw, "38;2;105;153;176") {
		return fmt.Errorf("gherky-inspired header, focus gutter, or palette is missing:\n%s", raw)
	}

	return nil
}

func navigation() error {
	analyzer := &sequenceAnalyzer{reports: []domain.Report{rankedReport()}}
	model := application.NewModel(analyzer, []string{"/workspace"})
	model, err := load(model, 120, 24)
	if err != nil {
		return err
	}

	model, err = press(model, "j")
	if err != nil {
		return err
	}
	view := plain(model.View().Content)
	if !containsAll(view, "› low.go", "low.go:4:1", "Calm") {
		return fmt.Errorf("file navigation did not update focus and detail:\n%s", view)
	}

	model, err = press(model, "k")
	if err != nil {
		return err
	}
	model, err = press(model, "tab")
	if err != nil {
		return err
	}
	model, err = press(model, "j")
	if err != nil {
		return err
	}
	view = plain(model.View().Content)
	if !containsAll(view, "› Simple", "hot.go:3:1", "sample · Simple") {
		return fmt.Errorf("function navigation did not update focus and detail:\n%s", view)
	}

	return nil
}

func refresh() error {
	stale := domain.Report{
		Root:      "/workspace",
		Files:     []domain.File{{Path: "stale.go", Total: 2, Peak: 2, Average: 2, Functions: []domain.Function{{Package: "sample", Name: "Stale", Complexity: 2, Line: 2, Column: 1}}}},
		Functions: 1,
		Total:     2,
		Average:   2,
	}
	fresh := domain.Report{
		Root:      "/workspace",
		Files:     []domain.File{{Path: "fresh.go", Total: 7, Peak: 7, Average: 7, Functions: []domain.Function{{Package: "sample", Name: "Fresh", Complexity: 7, Line: 8, Column: 1}}}},
		Functions: 1,
		Total:     7,
		Average:   7,
	}
	analyzer := &sequenceAnalyzer{reports: []domain.Report{stale, fresh}}
	model := application.NewModel(analyzer, []string{"/workspace"})
	model, err := load(model, 120, 24)
	if err != nil {
		return err
	}
	model, err = press(model, "r")
	if err != nil {
		return err
	}
	if len(analyzer.calls) != 2 || analyzer.calls[1][0] != "/workspace" {
		return fmt.Errorf("refresh scan paths = %v, want the original path twice", analyzer.calls)
	}

	view := plain(model.View().Content)
	if !strings.Contains(view, "fresh.go") || strings.Contains(view, "stale.go") {
		return fmt.Errorf("refresh did not replace stale results:\n%s", view)
	}

	return nil
}

func narrow() error {
	analyzer := &sequenceAnalyzer{reports: []domain.Report{rankedReport()}}
	model := application.NewModel(analyzer, []string{"/workspace"})
	model, err := load(model, 60, 14)
	if err != nil {
		return err
	}
	if err := assertNarrowPane(model, "Files", "Functions", "Details"); err != nil {
		return err
	}

	model, err = press(model, "tab")
	if err != nil {
		return err
	}
	if err := assertNarrowPane(model, "Functions", "Files", "Details"); err != nil {
		return err
	}

	model, err = press(model, "tab")
	if err != nil {
		return err
	}
	if err := assertNarrowPane(model, "Details", "Files", "Functions"); err != nil {
		return err
	}
	view := plain(model.View().Content)
	if !containsAll(view, "tab", "j/k", "r", "q") {
		return fmt.Errorf("compact key footer is incomplete:\n%s", view)
	}

	quitKeys := []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	}
	for _, key := range quitKeys {
		_, command := model.Update(key)
		if command == nil {
			return fmt.Errorf("%q did not return a quit command", key.Keystroke())
		}
		if _, ok := command().(tea.QuitMsg); !ok {
			return fmt.Errorf("%q command does not emit tea.QuitMsg", key.Keystroke())
		}
	}

	return nil
}

func rankedReport() domain.Report {
	return domain.Report{
		Root: "/workspace",
		Files: []domain.File{
			{
				Path:    "low.go",
				Total:   3,
				Peak:    3,
				Average: 3,
				Functions: []domain.Function{
					{Package: "sample", Name: "Calm", Complexity: 3, Line: 4, Column: 1},
				},
			},
			{
				Path:    "hot.go",
				Total:   12,
				Peak:    8,
				Average: 6,
				Functions: []domain.Function{
					{Package: "sample", Name: "Simple", Complexity: 4, Line: 3, Column: 1},
					{Package: "sample", Name: "Branchy", Complexity: 8, Line: 12, Column: 2},
				},
			},
		},
		Functions: 3,
		Total:     15,
		Average:   5,
	}
}

func load(model application.Model, width int, height int) (application.Model, error) {
	model, err := update(model, tea.WindowSizeMsg{Width: width, Height: height})
	if err != nil {
		return model, err
	}

	command := model.Init()
	return settle(model, command)
}

func press(model application.Model, key string) (application.Model, error) {
	message := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	if key == "tab" {
		message = tea.KeyPressMsg{Code: tea.KeyTab}
	}

	model, command, err := updateWithCommand(model, message)
	if err != nil {
		return model, err
	}

	return settle(model, command)
}

func update(model application.Model, message tea.Msg) (application.Model, error) {
	model, _, err := updateWithCommand(model, message)
	return model, err
}

func updateWithCommand(model application.Model, message tea.Msg) (application.Model, tea.Cmd, error) {
	updated, command := model.Update(message)
	next, ok := updated.(application.Model)
	if !ok {
		return model, nil, fmt.Errorf("updated model = %T, want application.Model", updated)
	}

	return next, command, nil
}

func settle(model application.Model, command tea.Cmd) (application.Model, error) {
	for attempts := 0; command != nil && attempts < 16; attempts++ {
		message := command()
		next, followup, err := updateWithCommand(model, message)
		if err != nil {
			return model, err
		}
		model = next
		command = followup
	}
	if command != nil {
		return model, errors.New("model did not settle after 16 commands")
	}

	return model, nil
}

func assertNarrowPane(model application.Model, visible string, hidden ...string) error {
	view := plain(model.View().Content)
	if !strings.Contains(view, visible) {
		return fmt.Errorf("narrow view does not show %s pane:\n%s", visible, view)
	}
	for _, title := range hidden {
		if strings.Contains(view, title) {
			return fmt.Errorf("narrow view shows inactive %s pane:\n%s", title, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if utf8.RuneCountInString(line) > 60 {
			return fmt.Errorf("narrow line width = %d, want <= 60: %q", utf8.RuneCountInString(line), line)
		}
	}

	return nil
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}

	return true
}

var ansi = regexp.MustCompile("\\x1b\\[[0-?]*[ -/]*[@-~]")

func plain(value string) string {
	return ansi.ReplaceAllString(value, "")
}

func failf(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", arguments...)
	os.Exit(1)
}
`
