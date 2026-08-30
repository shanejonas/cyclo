# QA Report

## Verdict

PASS.

The immutable contract is intact and every approved scenario passes. Rework attempt 1 resolves all four original QA findings: the repository has a real executable with default and explicit path wiring, real long and nested paths retain useful filenames and source coordinates, CJK and combining-grapheme rendering obeys terminal display-cell width, and all production Go is `gofmt`-clean.

The workspace's empty read-only `.git` directory prevents default Go VCS stamping. Plain `go build ./...` and `go list` fail before source compilation with `error obtaining VCS status`; their `-buildvcs=false` forms pass. This is the supplied harness metadata defect, not a source defect. Plain `go test ./...`, race tests with VCS stamping disabled, vet, tidy, module verification, a built ELF executable, and actual PTY launches all pass.

## Contract Integrity

- Diffed 1 validation file.
- Approved SHA-256: `c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6`.
- Observed SHA-256: `c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6`.
- Normalized only the allowed package declaration in `features/01-complexity-tui_test.go`, from `package complexity_tui_test` to `package tui_test`, then diffed it against `adapters/inbound/tui/model_test.go`.
- Diff result: empty. Assertions, probe source, imports, scenario bodies, and failure messages are identical.
- Tampering detected: none.

```text
$ sha256sum features/01-complexity-tui_test.go
c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6  features/01-complexity-tui_test.go

$ sed '1s/^package complexity_tui_test$/package tui_test/' features/01-complexity-tui_test.go | diff -u - adapters/inbound/tui/model_test.go
(no output)
```

## Scenarios Validated

- Scenario `Start with the current repository` -> `TestStartWithTheCurrentRepository` -> PASS. The contract observes the default `.` path, a non-empty real self-scan, the complete summary, and production complexity at most 10. Independent gocyclo measurement found a maximum of 9. An actual default-path PTY launch rendered `10 files · 59 functions`.
- Scenario `Rank files and functions by complexity` -> `TestRankFilesAndFunctionsByComplexity` -> PASS. File and function ordering, aggregate and function scores, source locations, focus marker, and palette assertions pass.
- Scenario `Navigate files and functions` -> `TestNavigateFilesAndFunctions` -> PASS. File and function movement updates focus and detail.
- Scenario `Refresh an analysis` -> `TestRefreshAnAnalysis` -> PASS. The second analyzer call receives the original explicit path and replaces stale results.
- Scenario `Keep narrow terminals useful and easy to leave` -> `TestKeepNarrowTerminalsUsefulAndEasyToLeave` -> PASS. Pane cycling, width, footer, `q`, and `ctrl+c` assertions pass. Separate display-cell regressions pass for both CJK and combining graphemes.

## Original Rejections Rechecked

1. **Executable and path wiring: PASS.** `main.go` is a real `package main`. It constructs the gocyclo analyzer, forwards `os.Args[1:]` to `tui.NewModel`, and runs Bubble Tea. `go list -buildvcs=false` reports the root package as `main`; `go build -buildvcs=false -o /tmp/cyclomatic-complexity-tui-qa-rework-bin .` produced an ELF executable. An actual pseudo-terminal launch with no path rendered the repository, while an explicit `./domain` launch rendered exactly one file, `report.go`. Both accepted `q` and exited 0.
2. **Useful real paths and coordinates: PASS.** Default-path PTY output includes `view.go`, `analyzer.go`, and the full useful detail `adapters/inbound/tui/view.go:83:1`. Explicit-path output includes `report.go`. `TestLongReportRootKeepsFilenameAndCoordinates` and `TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow` exercise the real renderer and retain `hot.go:12:2` and `analyzer.go:12:2` respectively.
3. **Terminal display-cell width: PASS.** `styledCell`, `truncate`, `fileLabel`, and `sourceLocation` use `ansi.StringWidth`/`ansi.Truncate`. `TestNarrowViewUsesTerminalCellWidth` checks every rendered line with terminal-cell measurement and catches both the former CJK overflow and combining-character under-padding classes.
4. **Production formatting: PASS.** `gofmt -d` over every production Go file produced no output.

## Added Regression Tests

- `main_extra_test.go:13` launches the documented root command as a real subprocess, provides explicit input, sends `q`, enforces a timeout, and requires exit 0. It is a real executable smoke test rather than a mock. QA separately supplied the missing terminal dimension with an actual PTY and asserted behavior from rendered output.
- `adapters/inbound/tui/view_extra_test.go:11` exercises `Model.View` with an actual long root and asserts the useful filename and coordinates.
- `adapters/inbound/tui/view_extra_test.go:44` covers the 80-column nested-path regression that the first relative-path change exposed.
- `adapters/inbound/tui/view_extra_test.go:77` measures actual terminal cells for CJK and combining-grapheme paths, including exact row padding. It does not use rune count or a snapshot.

Focused execution:

```text
$ go test -buildvcs=false ./... -run 'TestGoRunLaunchesTUI|TestLongReportRootKeepsFilenameAndCoordinates|TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow|TestNarrowViewUsesTerminalCellWidth' -count=1 -v
=== RUN   TestGoRunLaunchesTUI
--- PASS: TestGoRunLaunchesTUI (0.16s)
PASS
ok  github.com/shanejonas/cyclomatic-complexity-tui 0.159s
=== RUN   TestLongReportRootKeepsFilenameAndCoordinates
--- PASS: TestLongReportRootKeepsFilenameAndCoordinates (0.00s)
=== RUN   TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow
--- PASS: TestNestedSourceLocationKeepsFilenameAndCoordinatesWhenNarrow (0.00s)
=== RUN   TestNarrowViewUsesTerminalCellWidth
=== RUN   TestNarrowViewUsesTerminalCellWidth/CJK
=== RUN   TestNarrowViewUsesTerminalCellWidth/combining_character
--- PASS: TestNarrowViewUsesTerminalCellWidth (0.00s)
    --- PASS: TestNarrowViewUsesTerminalCellWidth/CJK (0.00s)
    --- PASS: TestNarrowViewUsesTerminalCellWidth/combining_character (0.00s)
PASS
ok  github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 0.003s
```

## Commands Run

The harness's default Go cache is outside the writable workspace. The first broad run stopped at `open /home/shanejonas/.cache/go-build/...: read-only file system`; all authoritative Go commands below therefore use isolated `GOCACHE` directories under `/tmp`.

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-full-cache go test ./... -count=1
ok  github.com/shanejonas/cyclomatic-complexity-tui 0.254s
ok  github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 1.210s
?   github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?   github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok  github.com/shanejonas/cyclomatic-complexity-tui/features 1.189s
?   github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-race-cache go test -buildvcs=false -race ./... -count=1
ok  github.com/shanejonas/cyclomatic-complexity-tui 7.014s
ok  github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 7.677s
?   github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?   github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok  github.com/shanejonas/cyclomatic-complexity-tui/features 7.705s
?   github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-vet-cache go vet ./...
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-tidy-cache go mod tidy -diff
(no output)

$ go mod verify
all modules verified

$ gofmt -d main.go adapters/inbound/tui/model.go adapters/inbound/tui/view.go adapters/outbound/gocyclo/analyzer.go domain/report.go ports/analyzer.go
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-build-cache go build ./...
error obtaining VCS status: exit status 128
    Use -buildvcs=false to disable VCS stamping.

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-build-novcs-cache go build -buildvcs=false ./...
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-list-cache go list -buildvcs=false -f '{{.Name}} {{.ImportPath}}' . ./...
main github.com/shanejonas/cyclomatic-complexity-tui
tui github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
gocyclo github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo
domain github.com/shanejonas/cyclomatic-complexity-tui/domain
complexity_tui github.com/shanejonas/cyclomatic-complexity-tui/features
ports github.com/shanejonas/cyclomatic-complexity-tui/ports

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-complexity-cache go run -buildvcs=false github.com/fzipp/gocyclo/cmd/gocyclo -over 10 -ignore '_test.go' .
(no output; exit 0)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-complexity-top-cache go run -buildvcs=false github.com/fzipp/gocyclo/cmd/gocyclo -top 10 -ignore '_test.go' .
9 gocyclo sourceFiles adapters/outbound/gocyclo/analyzer.go:47:1
7 gocyclo analyzeFile adapters/outbound/gocyclo/analyzer.go:114:1
7 gocyclo collectGoFile adapters/outbound/gocyclo/analyzer.go:89:1
6 tui (Model).updateKey adapters/inbound/tui/model.go:82:1
5 gocyclo commonRoot adapters/outbound/gocyclo/analyzer.go:154:1
5 gocyclo (Analyzer).Analyze adapters/outbound/gocyclo/analyzer.go:23:1
```

## Executable and PTY Evidence

QA built the product to `/tmp`, verified that it is an executable, and ran it through `/usr/bin/script`, which allocates a pseudo-terminal rather than piping directly into Bubble Tea.

```text
$ go build -buildvcs=false -o /tmp/cyclomatic-complexity-tui-qa-rework-bin .
$ file /tmp/cyclomatic-complexity-tui-qa-rework-bin
/tmp/cyclomatic-complexity-tui-qa-rework-bin: ELF 64-bit LSB executable, x86-64, ...

$ (sleep 3; printf q) | script -qfec 'stty cols 120 rows 24; exec go run -buildvcs=false .' /dev/null
CYCLO  10 files · 59 functions · Total 170 · Average 2.9
Files                                  │ Functions                              │ Details
› view.go · Total 47 · Peak 4          │   (Model).fileLines · Complexity 4     │ adapters/inbound/tui/view.go:83:1
  analyzer.go · Total 44 · Peak 9      │   (Model).functionLines · Complexity 4 │ tui
  model.go · Total 39 · Peak 6         │   displayPath · Complexity 4           │ Complexity 4
...
tab panes · j/k move · r refresh · q quit
(exit 0)

$ (sleep 2; printf q) | script -qfec 'stty cols 100 rows 20; exec /tmp/cyclomatic-complexity-tui-qa-rework-bin ./domain' /dev/null
CYCLO  1 files · 0 functions · Total 0 · Average 0.0
Files                           │ Functions                       │ Details
› report.go · Total 0 · Peak 0  │ No functions                    │ No function selected
tab panes · j/k move · r refresh · q quit
(exit 0)
```

The initial cold-cache PTY attempt received its buffered `q` before the first analysis completed and displayed Bubble Tea's initial zero report. Repeating with the warmed cache and `q` delivered after startup produced the completed reports above. This is input timing in the probe, not a product failure.

## Analyzer Failure Probes

QA ran a temporary external probe against the real exported gocyclo adapter and removed the probe afterward.

```text
PASS empty paths: at least one path is required
PASS missing path: inspect "/tmp/.../missing": stat /tmp/.../missing: no such file or directory
PASS non-Go file: "/tmp/.../README.md" is not a Go source file
PASS malformed Go: parse "/tmp/.../broken.go": /tmp/.../broken.go:2:6: expected 'IDENT', found '{'
```

All failure paths returned contextual errors. None panicked.

## Red/Green Evidence

QA copied the workspace to `/tmp/cyclomatic-complexity-tui-qa-rework-red.JSTbFA/repo`, moved the production TUI and rework-only regression files aside inside that copy, and installed a compileable inert `tui.Model` stub. The shared product and both contracts were never edited. This proves the immutable tests fail by diagnostic assertions against an empty implementation rather than by compile error or panic.

RED:

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-red-cache go test ./... -count=1
--- FAIL: TestStartWithTheCurrentRepository (0.16s)
    model_test.go:12: self-scan public TUI contract failed: exit status 1
        default scan paths = [], want [.]
        exit status 1
--- FAIL: TestRankFilesAndFunctionsByComplexity (0.15s)
    model_test.go:16: ranking public TUI contract failed: exit status 1
        explicit scan paths = [], want [/workspace]
        exit status 1
--- FAIL: TestNavigateFilesAndFunctions (0.16s)
    model_test.go:20: navigation public TUI contract failed: exit status 1
        file navigation did not update focus and detail:

        exit status 1
--- FAIL: TestRefreshAnAnalysis (0.17s)
    model_test.go:24: refresh public TUI contract failed: exit status 1
        refresh scan paths = [], want the original path twice
        exit status 1
--- FAIL: TestKeepNarrowTerminalsUsefulAndEasyToLeave (0.15s)
    model_test.go:28: narrow public TUI contract failed: exit status 1
        narrow view does not show Files pane:

        exit status 1
FAIL
FAIL github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 0.794s
?    github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?    github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
--- FAIL: TestStartWithTheCurrentRepository (0.16s)
    01-complexity-tui_test.go:12: self-scan public TUI contract failed: exit status 1
        default scan paths = [], want [.]
        exit status 1
--- FAIL: TestRankFilesAndFunctionsByComplexity (0.17s)
    01-complexity-tui_test.go:16: ranking public TUI contract failed: exit status 1
        explicit scan paths = [], want [/workspace]
        exit status 1
--- FAIL: TestNavigateFilesAndFunctions (0.17s)
    01-complexity-tui_test.go:20: navigation public TUI contract failed: exit status 1
        file navigation did not update focus and detail:

        exit status 1
--- FAIL: TestRefreshAnAnalysis (0.15s)
    01-complexity-tui_test.go:24: refresh public TUI contract failed: exit status 1
        refresh scan paths = [], want the original path twice
        exit status 1
--- FAIL: TestKeepNarrowTerminalsUsefulAndEasyToLeave (0.17s)
    01-complexity-tui_test.go:28: narrow public TUI contract failed: exit status 1
        narrow view does not show Files pane:

        exit status 1
FAIL
FAIL github.com/shanejonas/cyclomatic-complexity-tui/features 0.819s
?    github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]
FAIL
```

GREEN after the isolated RED run:

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-rework-final-green-cache go test ./... -count=1
ok  github.com/shanejonas/cyclomatic-complexity-tui 0.197s
ok  github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 0.900s
?   github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?   github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok  github.com/shanejonas/cyclomatic-complexity-tui/features 0.900s
?   github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]
```

## Edge Cases Probed

- Real default executable launch and `q` through a pseudo-terminal: PASS.
- Real explicit directory launch and `q` through a built executable in a pseudo-terminal: PASS.
- Long absolute report root reduced to a useful filename and preserved `:line:column`: PASS.
- Nested relative path at the minimum wide width preserved the basename and `:line:column`: PASS.
- CJK double-cell graphemes did not overflow a 10-cell terminal: PASS.
- Combining graphemes padded to exactly 10 terminal cells: PASS.
- Empty analyzer paths, missing paths, non-Go files, and malformed Go returned contextual errors: PASS.
- Production cyclomatic complexity ceiling `<= 10`: PASS; observed maximum 9.
- Refresh retained the original explicit path and replaced stale data: PASS through the immutable contract.
- `q` and `ctrl+c` returned real Bubble Tea quit commands: PASS through the immutable contract; `q` also exited two real PTY processes with status 0.

## Workspace Audit

`git status --short` cannot run because `.git` exists as an empty read-only directory:

```text
$ git status --short
fatal: not a git repository (or any parent up to mount point /)
Stopping at filesystem boundary (GIT_DISCOVERY_ACROSS_FILESYSTEM not set).

$ ls -la .git
total 0
dr-xr-xr-x 2 shanejonas shanejonas  40 Aug 30 00:23 .
drwxr-xr-x 1 shanejonas shanejonas 188 Aug 30 00:23 ..
```

Because Git metadata is unavailable, QA audited the filesystem directly. There are no `*.bak`, `*.tmp`, `*.orig`, editor backups, temporary QA probes, skipped tests, scratch runners, debug prints, sample dumps, or commented-out code blocks. The pre-existing `temp/` directory is empty. The visible files match the feature, contracts/handoffs, implementation, README/module files, and real regression tests.

## Issues

- No blocking source or contract issues.
- Non-blocking harness defect: the empty read-only `.git` directory breaks default VCS stamping for commands that build/list the root main package. `-buildvcs=false` passes, and the resulting executable works. A normal Git checkout supplies valid metadata and does not require that flag.
