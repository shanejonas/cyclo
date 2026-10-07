# Headless check

`cyclo check` extracts typed Go facts and evaluates the quality guardrails
without the TUI: the agent-loop entry point. Text output lists one finding per
line with its rule arithmetic; `--format json` emits the report
(`schema_version` 1); `--format facts` exports the typed facts at schema
version 2, which `--facts-in` re-evaluates (version 1 stays readable). Exit 0 means no findings (or facts exported), 1 means
findings, 2 means analysis/config/IO failure. `cyclo --skill` prints the
agent-facing operating skill for cyclo itself.

## Sub-features

- `check-text` reports findings as `path:line:col: pkg.Func: rule: arithmetic`.
- `check-json` emits the typed report with `schema_version`, `functions`,
  `diagnostics`, and `summary`.
- `check-facts` exports extraction only, for `--facts-in` re-evaluation.
- `check-exit-codes` distinguishes 0 (clean), 1 (findings), 2 (failure).
- `check-config` applies a TOML policy via `--config`; `--tests` includes
  test files.
- `skill-emit` prints the agent skill: YAML frontmatter with `name: cyclo`
  and the operating guide.

## How to get to it (user POV)

- Run `cyclo check .` (defaults to the current directory).
- Run `cyclo check --format json ./domain` for the typed report.
- Run `cyclo --skill` and hand the output to a coding agent.

## Driving it with verify-cyclo

Preconditions:

- `verify-cyclo build "$RUN_DIR"` and `verify-cyclo doctor "$RUN_DIR/cyclo"`.
- A fixture dir under `$RUN_DIR/scratch/fixture` with `go.mod` and one file:

  ```go
  package main

  func add(a, b, c, d, e int) int { return a + b + c + d + e }

  func main() { println(add(1, 2, 3, 4, 5)) }
  ```

- **Findings in text.** Run from inside the fixture dir:
  `"$RUN_DIR/cyclo" check .`. Exit code is 1.
  Stdout contains `main.go:3:1: fixture.add: fn_params: 5 exceeds 4` and a
  summary line like `2 functions; 1 findings;`. Stderr carries
  `quality guardrails exceeded`.
- **Clean exit.** Remove the offending function (leave `main` calling a
  2-parameter `add`). `check` exits 0 with `0 findings` in the summary.
- **Analysis failure.** Run `check` on a dir with no `go.mod` (or with `go`
  hidden from `PATH`). Exit code is 2 and stderr explains the failure
  (`go command required` / package load error).
- **JSON report.** From the fixture dir, run
  `"$RUN_DIR/cyclo" check --format json . > "$RUN_DIR/evidence/check.json"`.
  Parse it: `schema_version` is 1, `diagnostics[0]` carries `rule_id`
  `fn_params`, `actual` 5, `limit` 4, `line` 3, `column` 1, and `message`
  `fn_params: 5 exceeds 4`.
- **Facts export.** Run `"$RUN_DIR/cyclo" check --format facts .`
  and confirm it exits 0 with `schema_version` 2 in the output and no
  findings evaluated. Re-evaluate with
  `"$RUN_DIR/cyclo" check --facts-in "$RUN_DIR/evidence/check-facts.json"`
  (no source paths — facts-in cannot be combined with them) and confirm the
  same finding is reported, exit 1.
- **Agent skill.** Run `"$RUN_DIR/cyclo" --skill`. Stdout starts with
  `---` frontmatter naming `name: cyclo` and describes the treemap, the
  control API, and the headless check. Any extra argument (e.g.
  `cyclo --skill extra`) prints a usage error and exits nonzero.

## Gotchas

- `check` needs the Go toolchain on `PATH` to load packages — a missing `go`
  is exit 2, not a clean bill of health.
- Exit 1 is the *expected* signal for findings; only exit 2 fails a gate.
  The repo's own `Makefile` quality-gate relies on this (`|| test $? -eq 1`).
- `--format json` writes the report to stdout and still exits 1 on findings;
  redirect before asserting the exit code.
- The finding location format is `path:line:col` — `column` is 1-based.
