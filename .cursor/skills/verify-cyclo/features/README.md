# Cyclo verification map

This directory is the maintained source for verifying the user-facing behavior
of cyclo. Read the index before driving the app, then use the matching feature
file as the recipe.

## Baseline preconditions

- Build the binary with `verify-cyclo build "$RUN_DIR"` and require
  `verify-cyclo doctor "$RUN_DIR/cyclo"` to report `doctor: ok`.
- `go` is on `PATH` (cyclo shells out to the Go toolchain to load packages;
  without it every `check` drive exits 2 with `go command required`).
- TUI drives run only under `verify-cyclo tui` (a `script(1)` PTY, 160x50),
  with the helper's isolated `XDG_STATE_HOME`. Never drive the TUI against
  the operator's real annotation store.
- `--changed` drives use a temp git repo created per run under
  `$RUN_DIR/scratch`, with at least one commit. Never against the cyclo repo
  itself unless the drive is explicitly dogfooding the current diff.
- Keep every drive's evidence under `$RUN_DIR/evidence/` with the feature id
  in the filename; `verify-cyclo cleanup` removes everything else.

## Driving conventions

- Headless commands (`check`, `--skill`, `bug-reducer --tui=false`) run as
  direct child processes with piped output. Pipe them: non-TTY stdout switches
  `check` and `bug-reducer` to their plain script-friendly output.
- The TUI takes no piped stdin interaction beyond the keys `verify-cyclo tui`
  injects; every TUI drive is a fresh `script(1)` session. Never two drives on
  one TUI instance.
- Treat every command as literal. Keep quoted names and flags unchanged.
- Wait for analysis before sending TUI keys: the footer must show a nonzero
  `funcs` count (e.g. `4 funcs · cc 1.0 · cog 0.0`) before the run's keys are
  injected. The `--wait` flag covers this; bump it for large targets.
- Fixture code lives in `$RUN_DIR/scratch` and is removed by cleanup.
  Craft fixtures that trip exactly the rule under test.

## Proof and skip reporting

- Capture the action and the resulting state, not just the final screen.
- TUI proof is the typescript at `$RUN_DIR/evidence/tui.typescript`, stripped
  of ANSI, with the claimed strings quoted.
- CLI proof is the command, stdout, stderr, and exit code.
- JSON proof parses `--format json` and asserts `schema_version` and the
  claimed diagnostic fields.
- Mutation proof uses a read-only second view: after `bug-reducer`, `cat`
  `INPUT.reduced` and confirm the original `INPUT` is byte-identical; after
  attaching an annotation, relaunch the TUI and press `[` to revisit the note.
- Report a feature you could not reach with the attempted route and the
  concrete prerequisite (auth, OS, external state). Do not report a skipped
  entry point as verified through a different path.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the
user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with verify-cyclo` starts with `Preconditions:` and uses
   labeled bullets that pair each user action with an exact command and
   observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable
handles, required state, commands, and observable proof.

## Features

- [Explore the complexity map](./explore-complexity-map.md) covers launching
  the TUI, the treemap panes, footer counts, pane/file switching, and refresh.
- [Quality evidence](./quality-evidence.md) covers the `e` quality pane, the
  self-explaining finding arithmetic, and note annotations.
- [Headless check](./check-headless.md) covers `cyclo check` in text/json/facts
  formats, exit codes, `--config`, `--tests`, and `cyclo --skill`.
- [Changed-only findings](./check-changed.md) covers `cyclo check --changed`
  narrowing findings to git-touched functions.
- [Bug reducer](./bug-reducer.md) covers minimizing a reproducer with the
  checker contract and script-friendly output.
- [Pattern mining](./patterns.md) covers `cyclo patterns`: the 18 pattern
  kinds, detection-only kinds, and the miner's suppression rules.
- [Pattern autofix](./fix.md) covers `cyclo fix`: dry-run vs `--apply`,
  `--kind`, `--phased`, `--changed`, and idempotency.
