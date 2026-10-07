# Quality evidence

The quality pane explains each finding in the function's own terms: press `e`
in the TUI to switch the details pane from source to quality evidence, where
every finding shows its rule, the measured value against the limit, and for
`side_effect_density` the weight arithmetic with a per-kind breakdown.
Annotations (notes attached to source lines) persist across restarts in SQLite
and are navigable with `[`/`]`.

## Sub-features

- `quality-toggle` switches the details pane between source and quality with `e`.
- `quality-arithmetic` shows the finding arithmetic, e.g.
  `side_effect_density 5444 > 500: weight 49 over 9 statements (io 27, mutation 9, unknown 9, global 4)`.
- `quality-effects` lists `Effects` and `Mutations` counts with their source lines.
- `annotate-note` attaches a note to a source line with `v` + `a`, persists it
  in SQLite, and revisits it with `[`; `d` removes it.

## How to get to it (user POV)

- Launch `cyclo [paths...]`, select a function, press `e`.
- In Source, move with `j`/`k`, press `v` to start a line selection, `a` to
  attach a note, `esc` to clear the selection.
- Press `[`/`]` to visit the previous/next note; `d` removes the note under
  the cursor.

## Driving it with verify-cyclo

Preconditions:

- `verify-cyclo build "$RUN_DIR"` and `verify-cyclo doctor "$RUN_DIR/cyclo"`.
- A target dir with at least one quality finding (the `check-headless`
  fixture's 5-parameter function trips `fn_params`).

- **Toggle the quality pane.** Run
  `verify-cyclo tui "$RUN_DIR/cyclo" "$TARGET" "$RUN_DIR" --wait 6 --keys 'eq'`
  with a selected function that has findings. Strip ANSI from the typescript
  and assert `Findings · 1`, the finding text (e.g. `fn_params: 5 exceeds 4`),
  `Effects · 0`, and `Mutations · 0`.
- **Arithmetic in headless output.** Run
  `"$RUN_DIR/cyclo" check --format json "$TARGET" > "$RUN_DIR/evidence/check-headless.json`
  on a target with a `side_effect_density` finding. Parse the JSON and assert a
  diagnostic with `rule_id` `side_effect_density` carrying additive `weight`,
  `statements`, and `kind_weights` fields, and a `message` like
  `side_effect_density 5444 > 500: weight 49 over 9 statements (io 27, mutation 9, unknown 9, global 4)`.
  The same arithmetic appears in text output; unknown calls keep weight 1 and
  are attributed, not silently dropped.
- **Annotations persist.** Launch the TUI, move to a source line, press `v`,
  `a`, type a note, quit with `q`. Relaunch with `--keys 'q'` only; press
  nothing — instead run a second TUI drive with `--keys '[q'`: the typescript
  shows the saved note text in the Source pane (an unmatched note shows its
  saved text; a matched note reveals its current source). Proof is the note
  surviving the relaunch, read from the isolated
  `$RUN_DIR/scratch/xdg-state/cyclo/annotations.db` store.

## Gotchas

- `e` toggles the pane for the *selected* function; with no selection the
  quality pane is empty. Drives need a finding on the selected function.
- The `side_effect_density` arithmetic fields exist only on that rule's
  diagnostics — other rules carry `actual`/`limit` without `weight`.
- Notes are isolated by repository in the SQLite store. The helper's isolated
  `XDG_STATE_HOME` means a note attached in one run is invisible to another;
  that is the point — never disable the isolation to "see" notes.
- `d` removes the note under the cursor permanently; attach test notes to
  fixture files, never to real source.
