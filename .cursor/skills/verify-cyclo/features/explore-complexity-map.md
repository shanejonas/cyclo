# Explore the complexity map

The TUI is cyclo's primary surface: launch `cyclo [paths...]` and a full-screen
treemap shows cyclomatic complexity as tile area and cognitive complexity as
tile color, with Files, Functions, and Source panes, live counts in the
header/footer, and an automatic git diff gutter inside a worktree.

## Sub-features

- `map-launch` starts the TUI on a target dir and renders the treemap panes.
- `map-footer` shows live analysis counts: functions, mean cyclomatic and
  cognitive complexity, and the quality finding count.
- `map-panes` switches the details pane with `tab`/`shift+tab` and moves with
  `j`/`k`.
- `map-files` changes the selected file with `,`/`.`.
- `map-refresh` re-runs analysis with `r`.
- `map-quit` exits cleanly with `q` (exit 0) and leaves no residue.

## How to get to it (user POV)

- Run `cyclo .` in a terminal at least 100 columns wide and 30 rows tall.
- Run `cyclo ./domain ./adapters` to scan selected paths.
- Press `q` to quit.

## Driving it with verify-cyclo

Preconditions:

- `verify-cyclo build "$RUN_DIR"` and `verify-cyclo doctor "$RUN_DIR/cyclo"`.
- A target dir with Go files (the fixture repo from `check-headless` works).

- **Launch and render.** Run
  `verify-cyclo tui "$RUN_DIR/cyclo" "$TARGET" "$RUN_DIR" --wait 6 --keys 'q'`.
  The typescript shows the `CYCLO` header and the pane titles
  (`COMPLEXITY`, `TREEMAP`, `Files`, `Functions`, `Source`). The exit code is 0.
- **Footer counts.** Wait for analysis to finish, then strip ANSI from the
  typescript and assert a footer like `4 funcs · cc 1.0 · cog 0.0 · RPC :<port>
  · Q 3`: nonzero function count, mean complexities, a random RPC port
  (the `--control-port 0` binding), and the quality finding count `Q`.
- **Switch panes.** Run with `--keys 'eq'` — wait, then `e`, then `q`. The
  typescript shows the details pane title change to `Quality` and back toward
  `Source` (see `quality-evidence` for the full quality-pane proof).
- **Refresh.** Run with `--keys 'rq'`: after `r` the footer re-renders with
  the same counts (analysis re-ran against unchanged code).

## Gotchas

- The TUI renders nothing on a terminal smaller than 100x30 or on a bare pipe
  — a drive that skips `verify-cyclo tui` proves nothing about the TUI.
- Analysis is async: keys sent before the footer shows a nonzero `funcs`
  count act on an empty model. Bump `--wait` for large targets instead of
  guessing.
- The typescript is a raw PTY capture with ANSI escape codes and alternate
  screen sequences; always strip ANSI before asserting strings.
- The control server binds localhost only and dies with the TUI. If a drive
  times out, cleanup still removes the scratch state; check the port is free
  before the next TUI drive.
