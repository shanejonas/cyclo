# Changed-only findings

`cyclo check --changed` filters findings to functions the git diff touches —
the attention guide for agent loops: run it each iteration and only hear about
what your own edits touched. The base defaults to the merge-base with
`main`/`master` (falling back to their `origin/*` refs, then `HEAD`) and can
be pinned with `--base`. Untracked Go files count as fully changed. Facts are
evaluated whole, so findings on touched functions still carry complete helper
summaries. Exit 1 on filtered findings, so it works as a loop gate.

## Sub-features

- `changed-narrow` reports only findings in functions the diff touches.
- `changed-base` pins the diff base with `--base REF`.
- `changed-untracked` treats untracked Go files as fully changed.
- `changed-gate` exits 1 when filtered findings exist, 0 when none do.

## How to get to it (user POV)

- Run `cyclo check --changed` inside a git worktree after editing code.
- Run `cyclo check --changed --base origin/main` to pin the comparison ref.

## Driving it with verify-cyclo

Preconditions:

- `verify-cyclo build "$RUN_DIR"` and `verify-cyclo doctor "$RUN_DIR/cyclo"`.
- A temp git repo under `$RUN_DIR/scratch/changed-repo`: `git init`, a
  `go.mod`, and a committed `main.go` with two functions that each trip
  `fn_params` (5+ parameters), committed as the baseline.

- **Narrowing.** Leave the baseline committed. Append a third function with
  7 parameters to `main.go` (uncommitted). Run
  `"$RUN_DIR/cyclo" check "$REPO"` and confirm 3 findings. Run
  `"$RUN_DIR/cyclo" check --changed "$REPO"`: stdout shows only the new
  function's finding (e.g. `main.go:6:1: fixture.touched: fn_params: 7 exceeds 4`),
  the summary still counts all functions (`4 functions; 1 findings;`), and the
  exit code is 1. Proof of narrowing is the full run's 3 findings versus the
  changed run's 1.
- **Gate when clean.** Revert the uncommitted edit (`git checkout -- main.go`).
  `check --changed` exits 0 with `0 findings` even though the committed code
  still trips the rule — the diff, not the codebase, is the gate.
- **Pinned base.** Commit the 7-parameter function, then add an 8-parameter
  function uncommitted. `check --changed --base HEAD~1` reports both new
  functions; `--base HEAD` reports only the uncommitted one.
- **Untracked files.** Add a new `other.go` with a violating function without
  `git add`. `--changed` reports it: untracked Go files count as fully
  changed.

## Gotchas

- `--changed` needs real git history: at least one commit, otherwise there is
  no merge-base and it falls back to `HEAD`, which changes what "touched"
  means. Build the fixture repo with an explicit baseline commit.
- The default base is the merge-base with `main`/`master` — in a fixture repo
  with only a local `master`/`main` branch that is just `HEAD`. Name the
  branch deliberately in fixtures.
- Findings are filtered, facts are not: a finding on a touched function that
  calls an untouched helper still carries the helper's effect summary. That is
  by design — assert the summary is complete, don't mistake it for leakage.
- Never dogfood `--changed` against the cyclo repo unless the drive's point
  is the current diff; fixture repos keep the gate's semantics under test.
