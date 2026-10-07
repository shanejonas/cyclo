# Bug reducer

`cyclo bug-reducer` minimizes an input file while a checker program keeps
reproducing the bug. The checker receives the absolute candidate path as its
last argument and exits 0 to accept the candidate, any ordinary nonzero exit
to reject it. Default mode removes whole lines; `--language go` removes
Tree-sitter Go syntax units. With a terminal it shows a live dashboard;
`--tui=false` forces the script-friendly summary line. The original input is
preserved; the reduced output defaults to `INPUT.reduced` and must not already
exist.

## Sub-features

- `reduce-lines` minimizes line-by-line (default mode).
- `reduce-go` removes Go syntax units with `--language go`.
- `reduce-summary` prints `N -> M bytes; K checks; saved <path>` with
  `--tui=false`.
- `reduce-output` writes `INPUT.reduced` and leaves the original byte-identical.

## How to get to it (user POV)

- Run `cyclo bug-reducer [--tui=false] INPUT -- CHECKER [ARGS...]`.
- In the dashboard: `tab` changes panes, `j`/`k` scroll, `enter` expands,
  `q` stops and saves.

## Driving it with verify-cyclo

Preconditions:

- `verify-cyclo build "$RUN_DIR"` and `verify-cyclo doctor "$RUN_DIR/cyclo"`.
- A scratch dir `$RUN_DIR/scratch/reducer` with `input.txt`:

  ```
  line one
  line two
  line three BUG
  line four
  line five
  ```

  and an executable `checker.sh`:

  ```sh
  #!/bin/sh
  grep -q BUG "$1" && exit 0 || exit 1
  ```

- **Reduction.** Run from the scratch dir:
  `"$RUN_DIR/cyclo" bug-reducer --tui=false input.txt -- ./checker.sh`.
  Exit code is 0. Stdout ends with a summary like
  `53 -> 15 bytes; 14 checks; saved input.txt.reduced`.
- **Output proof.** `cat input.txt.reduced` shows only `line three BUG`.
  Read-only second view: `cmp input.txt` against a pristine copy (or re-read
  it) to confirm the original is unchanged — reduction never edits in place.
- **Existing output.** Run the same command again. It fails because
  `input.txt.reduced` already exists: the reducer never overwrites. Remove
  the output between drives.
- **Timeout.** `--timeout 1s` on a slow checker stops the reduction; the
  summary reports the stop, and no partial output is left behind.

## Gotchas

- The checker runs in the current directory — drive from the scratch dir so
  `./checker.sh` and the `INPUT.reduced` output land in scratch, not the repo.
- The candidate path arrives as the checker's *last* argument; a checker that
  reads `$1` instead of `"$@"`-last breaks on extra args.
- Timeouts, signals, and launch errors stop the reduction — a drive that
  kills the checker externally proves the stop path, not a reduction.
- `--language go` needs the tree-sitter CLI and a configured Go grammar, or
  `--go-parser PATH`. Without either, go-mode drives fail loudly; that is a
  missing prerequisite, not a product bug — record it as such.
