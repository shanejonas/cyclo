# QA Report

## Verdict

REJECT.

The package contract passes, but the product does not. The repository has no executable entrypoint, real self-scan paths lose every filename and source coordinate during rendering, and Unicode paths can exceed the terminal width. `domain/report.go` also fails `gofmt`.

## Contract Integrity

- Diffed 1 validation file.
- Approved SHA-256: `c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6`.
- Observed SHA-256: `c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6`.
- Normalized `features/01-complexity-tui_test.go` from `package complexity_tui_test` to the allowed `package tui_test`, then diffed it against `adapters/inbound/tui/model_test.go`.
- Diff result: empty. Assertions, probes, imports, and test bodies match.
- Tampering detected: none.

## Scenarios Validated

- `Start with the current repository` -> `TestStartWithTheCurrentRepository` -> contract PASS. Independent self-scan found `sourceFiles` at complexity 9, below the ceiling of 10.
- `Rank files and functions by complexity` -> `TestRankFilesAndFunctionsByComplexity` -> contract PASS. Real repository rendering FAILS because all absolute paths truncate before the filename.
- `Navigate files and functions` -> `TestNavigateFilesAndFunctions` -> PASS.
- `Refresh an analysis` -> `TestRefreshAnAnalysis` -> PASS.
- `Keep narrow terminals useful and easy to leave` -> `TestKeepNarrowTerminalsUsefulAndEasyToLeave` -> contract PASS for ASCII data. A Unicode path renders 12 terminal cells at width 10, so the real width invariant FAILS.

## Builder Claims

- Default paths, ranking, navigation, refresh, palette, narrow pane cycling, and quit commands match the synthetic contract.
- RED evidence matches a direct isolated removal of the implementation.
- GREEN, race, vet, and build evidence matches fresh runs.
- The claim that this implements a usable TUI does not match reality. The workspace has no `package main`, and no command can start a Bubble Tea program.
- The claim that widths 80 through 119 render correctly only holds for single-cell text. The renderer counts runes instead of terminal cells.
- Git metadata is unusable, so QA could not independently reconstruct the changed-file set from Git. The filesystem inventory matches the handoff plus pre-existing domain, port, adapter, and feature files.

## Commands Run

```text
$ sha256sum features/01-complexity-tui_test.go
c892f85d537b29647951436463daa5af6e9b8f064c4b424f35cf4b16b3b14eb6  features/01-complexity-tui_test.go

$ diff -u <(sed '1s/^package complexity_tui_test$/package tui_test/' features/01-complexity-tui_test.go) adapters/inbound/tui/model_test.go
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-test-cache go test ./... -count=1
ok  github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
?   github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?   github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok  github.com/shanejonas/cyclomatic-complexity-tui/features
?   github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-race-cache go test -race ./... -count=1
ok  github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
?   github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?   github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok  github.com/shanejonas/cyclomatic-complexity-tui/features
?   github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-vet-cache go vet ./...
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-build-cache go build ./...
(no output)

$ go mod verify
all modules verified

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-tidy-cache go mod tidy -diff
(no output)

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-entry-cache go run .
no Go files in /home/shanejonas/code/github.com/shanejonas/cyclomatic-complexity-tui

$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-list-cache go list -f '{{.Name}} {{.ImportPath}}' ./...
tui github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui
gocyclo github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo
domain github.com/shanejonas/cyclomatic-complexity-tui/domain
complexity_tui github.com/shanejonas/cyclomatic-complexity-tui/features
ports github.com/shanejonas/cyclomatic-complexity-tui/ports

$ rg -n '^package main$' -g '*.go' .
(no matches)

$ gofmt -d domain/report.go
diff domain/report.go.orig domain/report.go
--- domain/report.go.orig
+++ domain/report.go
@@ -1,11 +1,11 @@
 package domain
 
 type Report struct {
-       Root       string
-       Files      []File
+       Root      string
+       Files     []File
        Functions int
-       Total      int
-       Average    float64
+       Total     int
+       Average   float64
 }
```

`go test -cover ./... -count=1` passes but reports 0.0% in the TUI and analyzer packages. The committed tests launch probes in child `go run` processes, so Go's parent-process coverage does not observe the exercised code. The tests still call the real model, and the self-scan scenario calls the real gocyclo adapter.

`git status --short` returns `fatal: not a git repository`. A direct filesystem scan found no `*.bak`, `*.tmp`, `*.orig`, editor backups, debug prints, skipped tests, scratch runners, or data dumps. The workspace contains an empty `temp/` directory, which Git cannot track and which predates the implementation files.

## Red/Green Evidence

QA copied the workspace to `/tmp`, removed the production TUI files there, and installed a compileable inert `Model` stub. This avoids changing the shared implementation and proves that every scenario fails through its diagnostic assertion instead of a compile error or panic.

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-red-stub-cache go test ./... -count=1
--- FAIL: TestStartWithTheCurrentRepository (0.17s)
    model_test.go:12: self-scan public TUI contract failed: exit status 1
        default scan paths = [], want [.]
        exit status 1
--- FAIL: TestRankFilesAndFunctionsByComplexity (0.16s)
    model_test.go:16: ranking public TUI contract failed: exit status 1
        explicit scan paths = [], want [/workspace]
        exit status 1
--- FAIL: TestNavigateFilesAndFunctions (0.16s)
    model_test.go:20: navigation public TUI contract failed: exit status 1
        file navigation did not update focus and detail:

        exit status 1
--- FAIL: TestRefreshAnAnalysis (0.16s)
    model_test.go:24: refresh public TUI contract failed: exit status 1
        refresh scan paths = [], want the original path twice
        exit status 1
--- FAIL: TestKeepNarrowTerminalsUsefulAndEasyToLeave (0.17s)
    model_test.go:28: narrow public TUI contract failed: exit status 1
        narrow view does not show Files pane:

        exit status 1
FAIL
FAIL github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 0.817s
?    github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?    github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
--- FAIL: TestStartWithTheCurrentRepository (0.16s)
    01-complexity-tui_test.go:12: self-scan public TUI contract failed: exit status 1
        default scan paths = [], want [.]
        exit status 1
--- FAIL: TestRankFilesAndFunctionsByComplexity (0.16s)
    01-complexity-tui_test.go:16: ranking public TUI contract failed: exit status 1
        explicit scan paths = [], want [/workspace]
        exit status 1
--- FAIL: TestNavigateFilesAndFunctions (0.16s)
    01-complexity-tui_test.go:20: navigation public TUI contract failed: exit status 1
        file navigation did not update focus and detail:

        exit status 1
--- FAIL: TestRefreshAnAnalysis (0.16s)
    01-complexity-tui_test.go:24: refresh public TUI contract failed: exit status 1
        refresh scan paths = [], want the original path twice
        exit status 1
--- FAIL: TestKeepNarrowTerminalsUsefulAndEasyToLeave (0.16s)
    01-complexity-tui_test.go:28: narrow public TUI contract failed: exit status 1
        narrow view does not show Files pane:

        exit status 1
FAIL
FAIL github.com/shanejonas/cyclomatic-complexity-tui/features 0.809s
?    github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]
FAIL
```

The shared implementation remained untouched. A fresh GREEN run after RED produced:

```text
$ GOCACHE=/tmp/cyclomatic-complexity-tui-qa-final-green-cache go test ./... -count=1
ok   github.com/shanejonas/cyclomatic-complexity-tui/adapters/inbound/tui 0.910s
?    github.com/shanejonas/cyclomatic-complexity-tui/adapters/outbound/gocyclo [no test files]
?    github.com/shanejonas/cyclomatic-complexity-tui/domain [no test files]
ok   github.com/shanejonas/cyclomatic-complexity-tui/features 0.901s
?    github.com/shanejonas/cyclomatic-complexity-tui/ports [no test files]
```

## Edge Cases Probed

QA added temporary tests only inside an isolated `/tmp` copy. Product code and both contracts stayed untouched.

- Analyzer rejects empty paths, missing paths, non-Go files, and malformed Go with contextual errors: PASS.
- Analyzer ignores `.hidden`, `_cache`, `vendor`, and `testdata` directories: PASS.
- Analyzer deduplicates repeated roots and computes total, peak, and average correctly: PASS.
- Analyzer returns an empty report without error for an empty directory: PASS.
- TUI renders analyzer errors and tolerates empty-list navigation: PASS.
- Widths 1, 10, 40, 60, 79, 80, 81, 119, and 120 stay within the requested rune count: PASS.
- Actual terminal-cell width with a CJK path: FAIL. At width 10, the line `› 長い-long-` occupies 12 cells.
- Real self-scan at width 120: FAIL. Every file row starts and ends with the same truncated prefix, and the detail location drops the filename, line, and column.

```text
Files                                  │ Functions                              │ Details
› /home/shanejonas/code/github.com/sha │   sourceFiles · Complexity 9           │ /home/shanejonas/code/github.com/shane
  /home/shanejonas/code/github.com/sha │   collectGoFile · Complexity 7         │ gocyclo
  /home/shanejonas/code/github.com/sha │   analyzeFile · Complexity 7           │ Complexity 9
```

## Issues

1. **No executable TUI.** `go run .` fails, `go list` reports no main package, and `rg '^package main$'` finds nothing. Add a small command that builds `gocyclo.NewAnalyzer()`, passes CLI paths to `tui.NewModel`, and runs Bubble Tea. Add a smoke test for the documented launch command. Model-level quit commands do not replace an executable.

2. **Real scan paths erase the useful data.** `adapters/outbound/gocyclo/analyzer.go:55` converts every path to absolute form, `analyzer.go:142` stores that absolute path, and `adapters/inbound/tui/view.go:91` plus `view.go:150-176` truncate from the right. Render paths relative to `Report.Root`, or preserve the basename and `:line:column` when truncating. Add a regression test with a root longer than one pane.

3. **Unicode paths break the width contract.** `adapters/inbound/tui/view.go:152` pads by `utf8.RuneCountInString`, and `view.go:171` truncates by runes. Terminal cells are not runes. Use display-width-aware truncation and padding, then test CJK and combining-character paths at narrow widths.

4. **Production code is not formatted.** `gofmt -d domain/report.go` changes the `Report` field alignment at lines 3-8. Run `gofmt` and keep `gofmt -d` clean.

Do not change either contract while fixing these issues. Add separate regression tests for executable wiring, long real paths, and display-cell width.
