# Builder Rework Handoff

## Summary

Addressed every QA finding. The repository now has a root command that wires CLI paths into the gocyclo-backed model and runs Bubble Tea. Rendering makes absolute analyzer paths relative to `Report.Root`, falls back to the basename when a pane cannot fit a nested path, and keeps `:line:column` intact. Truncation and padding now use grapheme-aware terminal cell widths. All production Go files are formatted, and every production function remains below the complexity ceiling.

## Changed Files

- `main.go`
- `main_extra_test.go`
- `README.md`
- `adapters/inbound/tui/view.go`
- `adapters/inbound/tui/view_extra_test.go`
- `domain/report.go`
- `go.mod`
- `features/01-complexity-tui.builder-rework-1.md`

`features/01-complexity-tui_test.go` and `adapters/inbound/tui/model_test.go` remain unchanged.

## QA Findings Resolved

1. Added `main.go`. It creates `gocyclo.NewAnalyzer()`, passes `os.Args[1:]` to `tui.NewModel`, and runs `tea.NewProgram`. `NewModel` keeps the existing `.` default when no path is supplied. `TestGoRunLaunchesTUI` launches the documented `go run . <path>` command and quits it with `q`.
2. Paths now render relative to `Report.Root`. File rows and details fall back to the basename when a nested relative path does not fit. Details reserve space for `:line:column`. Long-root and 80-column nested-path regressions cover both cases.
3. Truncation, padding, and width assertions now use terminal display cells and grapheme clusters through `github.com/charmbracelet/x/ansi`. Narrow CJK and combining-character regressions cover overflow and under-padding.
4. Ran `gofmt` across every production Go file. `domain/report.go` now has canonical field alignment.

## RED Evidence

Before production changes:

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-red-cache go test ./... -count=1
--- FAIL: TestGoRunLaunchesTUI (0.02s)
    main_extra_test.go:32: go run . failed: exit status 1
        github.com/shanejonas/cyclomatic-complexity-tui: no non-test Go files in /home/shanejonas/code/github.com/shanejonas/cyclomatic-complexity-tui
FAIL
FAIL github.com/shanejonas/cyclomatic-complexity-tui
--- FAIL: TestLongReportRootKeepsFilenameAndCoordinates (0.00s)
    view_extra_test.go:40: file row lost the filename
--- FAIL: TestNarrowViewUsesTerminalCellWidth (0.00s)
    --- FAIL: TestNarrowViewUsesTerminalCellWidth/CJK (0.00s)
        view_extra_test.go:72: line width = 12, want <= 10
    --- FAIL: TestNarrowViewUsesTerminalCellWidth/combining_character (0.00s)
        view_extra_test.go:76: file row width = 9, want 10
FAIL
FAIL github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
FAIL
```

The initial relative-path fix exposed a nested-path edge at 80 columns:

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-nested-red-cache go test ./adapters/inbound/tui -run TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow -count=1
--- FAIL: TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow (0.00s)
    view_extra_test.go:73: file row lost the nested filename:
        › adapters/outbound/gocy │   Branchy · Complexity 8 │ adapters/outbound/gocycl
FAIL
```

## GREEN Evidence

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-build-no-vcs-cache go build -buildvcs=false ./...
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-test-cache go test ./... -count=1
ok   github.com/shanejonas/cyclomatic-complexity-tui
ok   github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
?    github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?    github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok   github.com/shanejonas/cyclomatic-complexity-tui/features
?    github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]

$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-vet-cache go vet ./...
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-race-cache go test -race ./... -count=1
ok   github.com/shanejonas/cyclomatic-complexity-tui
ok   github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
?    github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?    github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok   github.com/shanejonas/cyclomatic-complexity-tui/features
?    github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]

$ gofmt -d main.go main_extra_test.go adapters/inbound/tui/model.go adapters/inbound/tui/model_test.go adapters/inbound/tui/view.go adapters/inbound/tui/view_extra_test.go adapters/outbound/gocyclo/analyzer.go domain/report.go features/01-complexity-tui_test.go ports/analyzer.go
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-tidy-check-cache go mod tidy -diff
(no output)

$ go mod verify
all modules verified
```

## Executable Smoke Test

`TestGoRunLaunchesTUI` runs on every `go test ./...` invocation. A separate PTY smoke test launched the final renderer through the real default command:

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-final-smoke-cache go run .
CYCLO  10 files · 59 functions · Total 170 · Average 2.9
Files                     │ Functions                 │ Details
› view.go                 │   (Model).fileLines · Co  │ view.go:83:1
...
tab panes · j/k move · r refresh · q quit
```

Sending `q` exited with status 0.

## Complexity Evidence

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-complexity-cache go run github.com/fzipp/gocyclo/cmd/gocyclo -over 10 -ignore '_test.go' .
(no output; exit 0)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-rework-complexity-top-cache go run github.com/fzipp/gocyclo/cmd/gocyclo -top 10 -ignore '_test.go' .
9 gocyclo sourceFiles adapters/outbound/gocyclo/analyzer.go:47:1
7 gocyclo analyzeFile adapters/outbound/gocyclo/analyzer.go:114:1
7 gocyclo collectGoFile adapters/outbound/gocyclo/analyzer.go:89:1
6 tui (Model).updateKey adapters/inbound/tui/model.go:82:1
5 gocyclo commonRoot adapters/outbound/gocyclo/analyzer.go:154:1
5 gocyclo (Analyzer).Analyze adapters/outbound/gocyclo/analyzer.go:23:1
```

The highest production complexity is 9.

## Contract Integrity

```text
$ sha256sum features/01-complexity-tui_test.go
c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6  features/01-complexity-tui_test.go

$ diff -u <(sed '1s/^package complexity_tui_test$/package tui_test/' features/01-complexity-tui_test.go) adapters/inbound/tui/model_test.go
(no output)
```

## Refactors Applied

- Consolidated display-cell measurement and truncation on the ANSI grapheme implementation.
- Centralized root-relative path rendering.
- Added width-aware file labels and source locations so narrow panes keep the useful tail of a path.

Tests stayed green after each refactor.

## Risks / Open Questions

The workspace contains an empty, read-only `.git` directory. Now that the repository has a real executable, plain `go build ./...` asks Git for VCS stamping and fails with:

```text
error obtaining VCS status: exit status 128
    Use -buildvcs=false to disable VCS stamping.
```

`go build -buildvcs=false ./...` passes. A normal Git checkout does not need the flag. This is a workspace metadata limitation, not a source failure.
