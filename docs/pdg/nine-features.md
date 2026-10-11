# Nine CCGraph numerical features

Historical measurements from earlier worktree snapshots. Production now removes LSH and adds explicit return-to-output links. See [return-link-timing.md](return-link-timing.md) for the final return-link comparison.

The production CCGraph vector now includes all nine feature categories from
[Table 1 of CCGraph](https://yinxingxue.github.io/papers/ase2020_CCGraph%20A%20PDG%20based%20Code%20Clone%20Detector%20With%20Approximate%20Graph%20Matching.pdf).
Counts come from the canonical IR, not guessed from rstyle's abstraction labels.
The vector order is declarations, assignments, controls, calls, other nodes,
control edges, data edges, execution edges, reference bindings. This permutation
of Table 1 does not change cosine similarity. Existing synthetic mining graphs
without native source metadata retain their older seven-feature fallback.

## Execution policy

`cyclo.go-must-precede/1` exports a sparse precedence basis:

- Consecutive reachable statement/condition anchors in one basic block.
- The nearest dominating operation before the first anchor of another block.

CFG successors are inputs to immediate-dominator analysis, not exported edges.
No edge from one conditional arm to a join is asserted merely because that arm
can reach it. Unreachable operations after goto, continue, return or direct panic
are excluded. Back edges do not claim loop-carried must-precede relations.
The policy computes a basis, not all transitive pairs, so storage remains linear
in the number of operation anchors. It preserves existing nodes and spans.

This is an approximate Go source analysis. It does not model ordering inside a
statement, short-circuit operand events, panic/recover, deferred-call execution,
interprocedural termination or goroutine scheduling completely. Go/defer anchors
represent registration/spawn statements, not the eventual callee execution.
Evidence and the capability remain `Approximated`; graph conformance remains
incomplete. Those limits are available in the IR.

The implementation uses the already installed `golang.org/x/tools/go/cfg`, then
computes immediate dominators with reverse postorder and predecessor intersection.
Temporary arrays and predecessor lists are linear in CFG size. CFGs, source AST
maps and compiler objects are not retained by the returned IR.

## Reference policy

`cyclo.go-reference-bindings/1` counts distinct recorded variable bindings with
pointer, slice, map, channel, function, interface or unsafe-pointer types. Named
types and aliases use the underlying Go type. Parameters, receivers, named results,
locals and referenced globals participate. Fields are not separate variable
bindings. Arrays and structs are not counted merely because they contain a pointer;
strings and uintptr are not reference-bearing kinds in this policy.

Uses do not increase the count. Two distinct pointer variables count twice even
if they might alias. No shared storage or ownership/reference-counter value is
inferred. Type parameters remain unknown rather than being guessed scalar or
reference-bearing. The CountFact retains the reason and policy. An unavailable
vector cannot cause candidate rejection; pairs involving it proceed to WL.

## Matching and validation

`MiningView` derives one immutable nine-value feature record and maps execution
edges onto surviving matching nodes. Source counts include categories omitted by
abstraction matching, such as simple declarations; arithmetic stays Computation
rather than becoming Assignment. Entry, exit and formal-output metadata nodes are
excluded; formal input bindings count as declarations. Normalization preserves
source characteristics while remapping graph edges.

CCGraph's WL kernel includes execution edges with a distinct edge tag. The existing
abstraction kernel retains its data/control profile and mining characteristics,
so the original loop/refactoring behavior remains verified. These are matching
profiles of the same IR, not separately retained production PDGs. WL cache version
3 invalidates the previous computation format. No new dependency is installed.

Tests check nine native counts, unknown handling, normalization metadata, distinct
edge tags, reference aliases/named types and repeated uses. Dominators are checked
against independent reachability with each candidate dominator removed. Execution
fixtures cover sequence, branch joins, loops, goto, continue, return and panic.
Existing abstraction similarity and smoke tests pass without weakening their
thresholds. Build, complexity, cognitive, race, vet and refreshed quality checks pass.

## Docker reference

The fresh root-module Docker corpus contains 7,601 functions and 7,592 usable mining
graphs. Nineteen reference counts remain unknown due to type parameters. The IR
adds 59,410 execution edges, of which 39,116 map between surviving matching nodes.
Native nodes remain 215,616; total native edges rise from 261,714 to 321,124.

The new unfiltered execution-edge WL reference has 32,603 accepted pairs. The
paper-based candidate route retains 32,184 (98.7%). The remaining 419 pairs fail
both numerical and name admission without AST bypass; LSH excludes none here.
Numerical and name failure counts overlap and are not individually lost-pair
counts. Production groups match independently filtered full-score groups.
The previous 32,627-pair reference predates execution edges and is not reused as
an unchanged ground truth. Thresholds are not adjusted to force full agreement.

Local frozen input: `.tmp-build/docker-nine-corpus.json`. Use the measurement
commands in [matching.md](matching.md) with this corpus path. The old dataset is
retained as a separate historical artifact. Memory and full-report measurements
appear below and in [nine-feature-results.json](nine-feature-results.json).

## Memory and time

Three fresh-process graph runs and three alternating complete-report runs compare
against the compact IR before execution/reference analysis. The report baseline
already includes the candidate-admission correction and shared matching features.
Graph runs retain the same loaded compiler inputs through GC and measure only the
additional retained graph heap. Report runs measure the complete CLI process.

| Metric | Before | Nine source features |
| --- | ---: | ---: |
| Retained native graph heap, MiB | 55.64 | 59.00 |
| Graph construction, median ms | 601.887 | 647.343 |
| Complete report, median seconds | 4.287 | 4.318 |
| Complete report peak RSS, median MiB | 513.32 | 509.71 |

Retained graph heap increases by 6.1%.
Node and edge records remain 44 and 56 bytes. The reference count uses an existing
CountFact field, and policies/evidence/provenance use shared tables. Execution
adds edges rather than duplicating a PDG or materializing transitive closure.
Matching adds one temporary immutable feature record per function. It stores
no extra evidence or vectors per pair. Small report-time/RSS changes can be run
noise; these measurements do not establish equal performance on other corpora.
