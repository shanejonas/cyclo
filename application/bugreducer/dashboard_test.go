package bugreducer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shanejonas/cyclo/adapters/reducer"
)

// testDashboard builds a dashboard model the way reduction.newDashboard does,
// without needing a full reduction run.
func testDashboard(ctx context.Context, cancel context.CancelFunc, opts options, source []byte, outputs ...*checkerOutput) dashboard {
	output := &checkerOutput{}
	if len(outputs) > 0 {
		output = outputs[0]
	}
	return reduction{ctx: ctx, options: opts, source: source, check: &checker{output: output}}.newDashboard(ctx, cancel)
}

func TestDashboardReductionShowsChecksAndSaves(t *testing.T) {
	command := checkerCommand(t, "marker")
	t.Setenv("BUG_REDUCER_TEST_OUTPUT", "stdout marker")
	options := options{input: "input.txt", command: command, timeout: 5 * time.Second}
	check, cleanup, err := newChecker(options)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	check.output = &checkerOutput{}
	destination, err := os.Create(filepath.Join(t.TempDir(), "reduced"))
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("noise\nspecific failure\nmore noise\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := reduction{ctx: ctx, options: options, source: source, check: check, destination: destination, output: &bytes.Buffer{}}
	model := run.newDashboard(ctx, cancel)
	result := run.reduceForDashboard(func(msg tea.Msg) {
		model = model.withProgress(msg.(progressMsg))
	})
	if result.err != nil {
		t.Fatal(result.err)
	}
	saved, err := os.ReadFile(destination.Name())
	if err != nil || string(saved) != "specific failure\n" {
		t.Fatalf("saved = %q, %v", saved, err)
	}
	if !model.seedOK || model.accepted == 0 || model.rejected == 0 || model.checks != check.checks {
		t.Fatalf("counts = checks %d, accepted %d, rejected %d, seed %v", model.checks, model.accepted, model.rejected, model.seedOK)
	}
	if !bytes.Equal(model.best, saved) || !strings.Contains(result.summary, "saved ") {
		t.Fatalf("best = %q, summary = %q", model.best, result.summary)
	}
	for _, want := range []string{"stdout marker", "stderr marker"} {
		if !strings.Contains(check.output.snapshot(), want) {
			t.Fatalf("missing %q: %q", want, check.output.snapshot())
		}
	}
	if !strings.Contains(strings.Join(model.recent, "\n"), "- noise") {
		t.Fatalf("missing removed lines: %v", model.recent)
	}
}

func TestDashboardCancellationSavesAcceptedCandidate(t *testing.T) {
	command := checkerCommand(t, "marker")
	check, cleanup, err := newChecker(options{input: "input", command: command, timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	check.output = &checkerOutput{}
	destination, err := os.Create(filepath.Join(t.TempDir(), "reduced"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var best []byte
	run := reduction{
		ctx:         ctx,
		options:     options{},
		source:      []byte("noise\nspecific failure\nmore noise\n"),
		check:       check,
		destination: destination,
		output:      &bytes.Buffer{},
	}
	result := run.reduceForDashboard(func(msg tea.Msg) {
		p := msg.(progressMsg).progress
		if p.Accepted && !p.Seed {
			best = bytes.Clone(p.Candidate)
			cancel()
		}
	})
	if !errors.Is(result.err, context.Canceled) || len(best) == 0 {
		t.Fatalf("result = %+v, best = %q", result, best)
	}
	saved, err := os.ReadFile(destination.Name())
	if err != nil || !bytes.Equal(saved, best) {
		t.Fatalf("saved = %q, best = %q, error = %v", saved, best, err)
	}
}

func TestDashboardStopWaitsForSaveAndCompletedRunStaysOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := testDashboard(ctx, cancel, options{}, []byte("input"))
	next, cmd := model.updateKey("q")
	if cmd != nil || ctx.Err() == nil || !next.(dashboard).stopping {
		t.Fatal("quit must cancel the checker and wait for the saved result")
	}
	_, cmd = next.(dashboard).Update(reductionResult{err: context.Canceled, at: time.Now()})
	if cmd == nil {
		t.Fatal("stopped run did not exit after saving")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	model = testDashboard(ctx, cancel, options{}, []byte("input"))
	next, cmd = model.Update(reductionResult{at: time.Now()})
	if cmd != nil || next.(dashboard).result == nil {
		t.Fatal("completed run must remain open for inspection")
	}
	cancel()
	_, cmd = next.(dashboard).Update(dashboardTick(time.Now()))
	if cmd == nil {
		t.Fatal("external cancellation must close an already completed dashboard")
	}
}

func TestDashboardFitsTerminalAndShowsAllPanes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := testDashboard(ctx, cancel, options{input: "demo.txt", output: "demo.reduced", command: []string{"checker"}}, []byte("input"))
	view := ansi.Strip(model.View().Content)
	for _, title := range paneTitles {
		if !strings.Contains(view, title) {
			t.Fatalf("missing pane %q:\n%s", title, view)
		}
	}
	for _, size := range [][2]int{{120, 36}, {96, 24}, {80, 24}, {40, 12}, {10, 5}, {1, 1}} {
		model.width, model.height = size[0], size[1]
		for focus := range paneTitles {
			model.focus = focus
			lines := strings.Split(model.View().Content, "\n")
			if len(lines) != model.height {
				t.Fatalf("%v pane %d: %d lines", size, focus, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > model.width {
					t.Fatalf("%v pane %d: line exceeds width: %q", size, focus, line)
				}
			}
		}
	}
}

func TestDashboardOutputScrollsBackFromLiveTail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := &checkerOutput{}
	_, _ = output.Write([]byte(strings.Repeat("old line\n", 30) + "newest line"))
	model := testDashboard(ctx, cancel, options{}, nil, output)
	model.focus = 3
	if !strings.Contains(model.panel(3, 60, 10), "newest line") {
		t.Fatal("output does not follow the tail")
	}
	model = model.scroll("k")
	if strings.Contains(model.panel(3, 60, 10), "newest line") {
		t.Fatal("scrolling up does not leave the live tail")
	}
	model = model.scroll("G")
	if !strings.Contains(model.panel(3, 60, 10), "newest line") {
		t.Fatal("end does not return to live output")
	}
}

func TestCheckerOutputIsBoundedAndClearedBetweenChecks(t *testing.T) {
	output := &checkerOutput{}
	large := []byte(strings.Repeat("x", previewLimit*2) + "end")
	if n, err := output.Write(large); n != len(large) || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if snapshot := output.snapshot(); len(snapshot) != previewLimit || !strings.HasSuffix(snapshot, "end") {
		t.Fatalf("incorrect tail: %d bytes", len(snapshot))
	}
	_, _ = output.Write([]byte("more"))
	if snapshot := output.snapshot(); len(snapshot) != previewLimit || !strings.HasSuffix(snapshot, "endmore") {
		t.Fatal("small writes do not preserve the bounded tail")
	}
	output.reset()
	if output.snapshot() != "" {
		t.Fatal("previous checker output remains")
	}
}

func TestCheckerOutputIsVisibleBeforeCheckCompletes(t *testing.T) {
	command := checkerCommand(t, "timeout")
	t.Setenv("BUG_REDUCER_TEST_OUTPUT", "running check")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	check := checker{command: command, path: filepath.Join(t.TempDir(), "candidate"), timeout: time.Second, output: &checkerOutput{}}
	done := make(chan error, 1)
	go func() {
		_, err := check.check(ctx, []byte("input"))
		done <- err
	}()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !strings.Contains(check.output.snapshot(), "running check") {
		select {
		case err := <-done:
			t.Fatalf("checker finishes before output is visible: %v", err)
		case <-deadline:
			t.Fatal("checker output never arrives")
		case <-ticker.C:
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("checker cancellation = %v", err)
	}
}

func TestDashboardSanitizesTerminalText(t *testing.T) {
	input := "\x1b[2J\x1b]52;c;clipboard\ahello\x00\b\r\x1b[31m world\x1b[0m\n\t你好"
	if got := safeText(input); got != "hello world\n    你好" {
		t.Fatalf("unsafe output: %q", got)
	}
}

func TestDeletedLinePreviewPreservesExactBytes(t *testing.T) {
	for _, tc := range []struct {
		source, want string
		start, count int
	}{
		{"alpha\nbeta\ngamma", "beta\n", 1, 1},
		{"alpha\nbeta", "beta", 1, 1},
		{"alpha\n", "", 1, 1},
		{"", "", 0, 1},
		{"a\r\nb\r\nc\r\n", "b\r\nc\r\n", 1, 2},
		{"alpha\n\nbeta", "\n", 1, 1},
		{"alpha\nbeta\n", "beta\n", 1, 2},
		{"prefix\n語語\nsuffix", "語語\n", 1, 1},
		{"alpha\nbeta", "", 1, 0},
	} {
		progress := reducer.Progress{StartLine: tc.start, RemovedLines: tc.count}
		if got := string(removedBytes([]byte(tc.source), progress)); got != tc.want {
			t.Fatalf("source %q, start %d, count %d: removed %q, want %q", tc.source, tc.start, tc.count, got, tc.want)
		}
	}
}

func TestDashboardTracksTimeAndBestSize(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	model := testDashboard(ctx, cancel, options{}, []byte("noise\nbug\n"))
	model = model.withProgress(progressMsg{progress: reducer.Progress{Candidate: []byte("bug\n"), Accepted: true, RemovedLines: 1}, at: model.started.Add(time.Second)})
	if model.history[1].elapsed != time.Second || model.history[1].bytes != 4 {
		t.Fatalf("history = %+v", model.history)
	}
	model.now = model.started.Add(2 * time.Second)
	chart := ansi.Strip(strings.Join(model.sizeChart(40, 8), "\n"))
	if !strings.Contains(chart, "100%") || !strings.Contains(chart, "2s") || !strings.Contains(chart, "●") {
		t.Fatalf("chart = %s", chart)
	}
}

func TestTUIFlagsKeepNonterminalRunsPlain(t *testing.T) {
	for _, flag := range []string{"", "--tui", "--tui=false"} {
		args := []string{"input", "--", "checker"}
		if flag != "" {
			args = append([]string{flag}, args...)
		}
		options, err := parseOptions(args, &bytes.Buffer{})
		if err != nil || options.tui != (flag == "--tui") {
			t.Fatalf("%q = %+v, %v", flag, options, err)
		}
	}
}

func TestDashboardSyntaxDeletionUsesByteRange(t *testing.T) {
	source := []byte("package p\nfunc keep() { println(1); println(2) }\n")
	start := bytes.Index(source, []byte("println(1)"))
	end := start + len("println(1)")
	model := dashboard{best: source, checks: 2}
	progress := reducer.Progress{Unit: "expression_statement", StartByte: start, EndByte: end,
		Candidate: append(bytes.Clone(source[:start]), source[end:]...), StartLine: 1}
	preview := strings.Join(model.deletion(progress), "\n")
	if !strings.Contains(preview, "- println(1)") || strings.Contains(preview, "println(2)") {
		t.Fatalf("preview = %s", preview)
	}
	model.current = progress
	if !strings.Contains(model.attemptLabel(), "expression_statement") {
		t.Fatal(model.attemptLabel())
	}
}
