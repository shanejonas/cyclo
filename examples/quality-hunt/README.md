# Generated quality-analysis checks

Run from the repository root:

```sh
go run ./examples/quality-hunt
```

The harness generates 126 programs: 21 ownership families crossed with six
syntax variants. Families cover shared and owned pointers, slices and maps,
array copies, indirect writes, range assignment, embedded pointers, pointers
inside containers, closure parameters, pointer reassignment, and generic pointer
and slice forms, plus generic array copies, array slicing, and array-only
constraint unions, local helper mutation chains, and effect-free local helpers. Variants include the baseline, parentheses around places and
callees, renamed locals,
comments, a discarded literal, and a combination of those transformations.

It compiles and executes the generated programs in one temporary Go module,
records whether each changes caller-owned state, extracts typed quality facts,
and checks three invariants:

- A runtime caller-state change cannot receive a complete, effect-free report.
- The explicitly known owned-storage examples remain complete and effect-free.
- Equivalent syntax preserves mutation target grouping, field paths and
  provenance, effect kinds, completeness, and unclassified-call counts.

Physical source positions, names, code length, and numeric density are excluded
from equivalence comparison. Formatting changes source positions, and adding a
harmless discard adds a statement to the density denominator.

This is a deterministic, bounded corpus, not a general proof of soundness or a
random fuzzer. It covers specific inputs and does not establish correctness for
all possible runtime values, arbitrary aliases, generic constraints, unsafe
code, or interprocedural effects. Extend the family table when adding supported
ownership patterns; generator expectation failures stop the run.

## Reduce a mismatch

The current checkout's analyzer is the default. To hunt against a saved build:

```sh
go run ./examples/quality-hunt --cyclo /path/to/frozen-cyclo --go-parser /path/to/go.so --output /tmp/new-quality-failure
```

The output directory must not exist. `--cyclo` takes a binary file path. A
mismatch exits nonzero and automatically invokes the Tree-sitter Go reducer,
keeping only candidates that compile, preserve the expected runtime ownership
behavior, retain the same mismatch, and remain equivalent under the generated
transformations. The independent AST equivalence check prevents reductions from
turning legitimate differences between two programs into apparent analyzer bugs.

Tree-sitter is required only when reducing a mismatch. Omit `--go-parser` if its
CLI already resolves the `source.go` grammar. Parser failures stop reduction and
retain the confirmed seed; the harness never falls back to line deletion.
`--timeout` defaults to 30 seconds per runtime-and-analysis check. Ctrl-C stops
work, and an interrupted reduction saves its best confirmed candidate.

Saved evidence includes `seed.go.txt`, `reduced.go.txt`, and `failure.json` with
the selected cases, expected runtime behavior, failure identity, and checker
count. Recheck a saved directory without another reduction:

```sh
go run ./examples/quality-hunt --recheck /tmp/new-quality-failure
go run ./examples/quality-hunt --cyclo /path/to/frozen-cyclo --recheck /tmp/new-quality-failure
```

PASS exits 0. A confirmed mismatch, invalid candidate, timeout, or operational
failure exits nonzero, with a diagnostic identifying the reason. The external
analyzer must support `check --format facts`; classification uses the current
checkout's default policy so the comparison focuses on extracted facts.

## Validation

`go test ./examples/quality-hunt` compiles and runs the corpus against the current
analyzer, checks that independent program edits are rejected, verifies bad
runtime expectations are caught, and tests detection of hidden writes and
phantom mutations. It also rechecks the saved reduced counterexample. These
tests run automatically through the repository's normal test and race jobs;
Tree-sitter and frozen binaries are not required for CI.

The first negative-control experiment uses a build from before the
parenthesized-target fixes. It detects a phantom blank-identifier mutation,
shrinks the original 80-program corpus from 8,540 to 242 bytes in 32 checks, and
saves the pair in `testdata/phantom-discard`. The frozen analyzer fails the
recheck; the fixed analyzer passes. The corpus subsequently expands to 96
programs by adding combined transformations.

## New findings from corpus expansion

Adding parenthesized callees reveals that `(new)(State)` and `(make)([]int, 1)`
lose owned-allocation provenance, producing false unknown effects. The fixed
analyzer unwraps the callee before checking builtin identity. The saved pair in
`testdata/parenthesized-allocation` fails against a build frozen immediately
before this expansion and passes after the fix.

The installed Tree-sitter Go grammar rejects parenthesized builtin allocation
calls even though Go compiles and executes them. That fixture is isolated from
the generated corpus, not Tree-sitter-reduced; its checker count is zero. Parser
rejection preserves the confirmed Go input and never triggers line fallback.

Generic array copies expose a second false effect: indexing a value whose type
parameter is restricted to arrays was classified as external. Slices of those
array copies have the same problem. Array-only unions and embedded constraints
now retain value ownership; constraints permitting references remain
conservative. The runtime-and-report oracle and 320-to-111-byte Tree-sitter
reduction (26 checks) live in
`../../adapters/goquality/testdata/generic-array-copy`.
