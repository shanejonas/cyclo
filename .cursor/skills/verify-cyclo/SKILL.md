---
name: verify-cyclo
description: "Drive cyclo the way a user or agent does: the complexity TUI treemap, headless `cyclo check`, `check --changed` as an agent-loop gate, `--skill`, and `bug-reducer`. Use when you need to prove cyclo's behavior on a real repo instead of trusting its unit tests."
---

# Verify cyclo

This skill drives the real cyclo binary against real Go code and captures proof.
It is written for an agent that has never seen this repo: treat every command as
literal, and keep quoted names and flags unchanged.

## Launch

Cyclo has two runtimes. Build the binary once per run, then drive each feature
in its own isolated session:

```sh
verify-cyclo build "$RUN_DIR"   # builds $RUN_DIR/cyclo from this repo
```

`verify-cyclo` is the shipped helper at `bin/verify-cyclo` (see Helpers).

- **Headless commands** (`check`, `--skill`, `bug-reducer --tui=false`) run
  directly as child processes. Pipe their output: when stdout is not a TTY,
  `bug-reducer` and `check` emit plain text instead of dashboards.
- **The TUI** (`cyclo [paths...]`) needs a real terminal: at least 100 columns
  by 30 rows, driven through `script(1)` so bubbletea sees a PTY. It renders
  nothing useful on a smaller terminal or a bare pipe.

```sh
verify-cyclo tui "$RUN_DIR/cyclo" "$TARGET_DIR" "$RUN_DIR" --wait 6 --keys 'eq'
```

This launches the TUI inside `script(1)` with `stty cols 160 rows 50`, waits
6 seconds for analysis to finish, then sends `e` (toggle the quality pane)
and `q` (quit). The full terminal transcript lands at
`$RUN_DIR/evidence/tui.typescript`.

**Isolation is mandatory, not optional:**

- `go` must be on `PATH`: `cyclo check` shells out to the Go toolchain to load
  packages, and exits 2 with `go command required` without it.
- Point `XDG_STATE_HOME` at a per-run directory. The TUI persists annotation
  notes in `$XDG_STATE_HOME/cyclo/annotations.db`; driving without isolation
  writes into the operator's real annotation store. The helper does this for
  TUI drives automatically (`$RUN_DIR/scratch/xdg-state`).
- Pass `--control-port 0` to the TUI so the localhost JSON-RPC control server
  binds a random free port instead of the default `8197`. Never assume 8197 is
  free; never drive two TUIs on the same port.
- `--changed` drives need their own temp git repo with at least one commit.
  Craft fixtures there; never run `--changed` drives against the cyclo repo
  itself unless the point is dogfooding its current diff.

## Doctor

One read-only check that answers "is this binary worth driving?" Run it before
the first drive, and again after any failed drive before retrying:

```sh
verify-cyclo doctor "$RUN_DIR/cyclo"
```

It verifies: the binary is executable; `check --help` prints the usage line;
`go version` works; `git --version` works; `script(1)` exists for TUI drives.
A doctor failure caused by this skill's drift (wrong flags, moved paths) is
doc drift: fix the skill, rebuild if the fix touched code, and re-run doctor
once before calling the pass blocked.

For the TUI specifically, a drive is healthy when the stripped typescript
contains `CYCLO` in the header and `funcs·` with a nonzero count in the footer,
e.g. `945 funcs·cc4.0·cog3.5`. A footer that never advances past
`0 funcs·cc0.0·cog0.0` after the wait means analysis never finished — doctor
the target dir (does it contain Go files? does `go list` work there?) rather
than sending more keys.

## Drive

The harness recipe is per feature; exact commands live in the feature files
under `features/`. The conventions:

- Prefer stable handles: quality rule ids (`fn_params`, `side_effect_density`),
  CLI flags, and key letters from the footer hints (`tab/shift+tab` panes,
  `j/k` move, `,`/`.` files, `e` quality pane, `[`/`]` notes, `v`/`a`/`d`
  annotations, `r` refresh, `q` quit).
- Headless drives are one process per drive. TUI drives are one `script(1)`
  session per drive — never two drives sharing a TUI instance.
- For `--changed`, commit a baseline first, then leave the change under test
  uncommitted (uncommitted edits count as fully changed files).
- `bug-reducer` takes `INPUT -- CHECKER [ARGS...]`; the checker receives the
  absolute candidate path as its last argument and exits 0 to accept.
  `--tui=false` forces the script-friendly summary line.
- `cyclo --skill` takes no other arguments; anything extra prints a usage
  error and exits nonzero.

## Evidence

Capture the action and the resulting state, not just the final screen:

- **Headless:** the full command, stdout, stderr, and exit code. `check` exits
  0 for no findings (or facts exported), 1 for findings, 2 for
  analysis/config/IO failure. Parse `--format json` output and assert
  `schema_version` (1) plus the diagnostic fields the feature claims
  (`rule_id`, `message`, `actual`, `limit`, `line`, `column`; `weight`,
  `statements`, `kind_weights`, `effects` on `side_effect_density`).
  `--format facts` exports the typed facts at `schema_version` 2 (version 1
  stays readable via `--facts-in`).
- **TUI:** the typescript at `$RUN_DIR/evidence/tui.typescript`. Strip ANSI and
  assert the claimed strings (header `CYCLO`, footer counts, `qualityfailed`
  when the target has findings). A screenshot is not available headless; the
  typescript is the proof.
- **Mutations:** prove side effects with a read-only second view. After
  `bug-reducer`, `cat INPUT.reduced` and confirm the original `INPUT` is
  unchanged. After attaching an annotation, relaunch the TUI and press `[`
  to visit the saved note.

Evidence goes to `$RUN_DIR/evidence/` with the feature id in the filename.
Never put evidence under `scratch/` — cleanup removes scratch.

## Cleanup

```sh
verify-cyclo cleanup "$RUN_DIR"
```

This removes `$RUN_DIR/scratch` (isolated `XDG_STATE_HOME`, temp git repos,
reducer inputs) and the built binary, and keeps `$RUN_DIR/evidence`. It never
kills by process name: the TUI is quit with `q` inside its own session, and
the control server dies with it. Run cleanup after every failed iteration too,
so broken attempts don't strand PTY sessions or ports. After cleanup, confirm
the evidence still exists at its named location — a cleanup that eats the
proof fails the run.

## Helpers

`bin/verify-cyclo` is the only helper. It is executable; every subcommand:

```sh
verify-cyclo build <run-dir>            # build $RUN_DIR/cyclo from the repo
verify-cyclo doctor <cyclo-bin>         # read-only readiness check
verify-cyclo tui <cyclo-bin> <target-dir> <run-dir> [--wait N] [--keys KEYS]
                                       # drive the TUI under script(1), save typescript
verify-cyclo cleanup <run-dir>         # remove scratch + binary, keep evidence
```

`--keys` is a literal key string sent after `--wait` seconds (default 6);
`%b` escapes are honored, so `\x1b` sends escape. Key letters come from the
TUI footer hints; the feature files name the ones each drive needs.
