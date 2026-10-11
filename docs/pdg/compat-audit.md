# Compat readiness audit

Audit date: 2026-10-10. Target: [rstyled/compat PR #8](https://github.com/rstyled/compat/pull/8), head `e1d08c2500e4184444fed70c9d9d1df206ac6962`. This record describes the current Cyclo worktree, including uncommitted changes. It does not certify the Rust producer or cross-engine conformance.

## What upstream selects

The PR proposes a shared PDG contract and a separate CCGraph base profile. The base emits deduplicated accepted pairs and stage evidence. Abstraction proposals, connected groups, AST bypass, whole-function LSH and neighborhood kernels are extensions. Existing behavior must remain available during migration; a new base needs explicit activation.

The schema remains draft 0.1.0. Compared with our previous pin `67b32032e52d88b2b109a55686eb4ed67f9e5171`, this head adds descriptions and a comment, without changing structural validation rules. The local schema snapshot now pins the new head. Cyclo uses the contract as its native IR. `patterns --format pdg-json` now exports
that IR as an array of compat documents; this does not establish semantic conformance.

The inventory records historical source snapshots, not this worktree. Its claims about seven numerical features and missing execution edges describe that earlier Go revision. They must not be copied into a current status report.

## All issues and the PR

All seven issues and PR #8 are open at this snapshot. The issues discuss choices; they do not select an engine architecture.

| Item | Scope | Effect on Cyclo |
| --- | --- | --- |
| [#1](https://github.com/rstyled/compat/issues/1) | One shared engine | Architecture option; no instruction to replace native Go |
| [#2](https://github.com/rstyled/compat/issues/2) | FFI, services, graph deltas | Integration options; no transport selected |
| [#3](https://github.com/rstyled/compat/issues/3) | CCGraph fidelity | Record extraction and matching differences; do not claim exact reproduction |
| [#4](https://github.com/rstyled/compat/issues/4) | Combine strengths | Keep clone detection and refactoring advice distinct |
| [#5](https://github.com/rstyled/compat/issues/5) | Native engines with conformance | Shared stage fixtures can test native implementations |
| [#6](https://github.com/rstyled/compat/issues/6) | Decision map | Compatibility boundaries precede architecture selection |
| [#7](https://github.com/rstyled/compat/issues/7) | Algorithm inventory and standardization | Start with bounded PDG/CCGraph source audit, then specify profiles |
| [PR #8](https://github.com/rstyled/compat/pull/8) | Inventory, plan, draft schema | Current reviewable contract proposal; not a merged executable specification |

## Local readiness

| Boundary | Current behavior | Missing evidence or work |
| --- | --- | --- |
| Native IR | Interned strings/types/evidence; indexed nodes, edges and sparse attributes | Structural representation is present; semantic conformance remains separate |
| Go producer | Interfaces, spans, definitions, reads/writes, calls, reference counts, explicit return-to-output links | Reaching definitions, branch/loop merges, post-dominator control, captures and alias analysis remain incomplete |
| Execution dependencies | Versioned approximate must-precede basis | Not complete exceptional/concurrent execution semantics |
| Projection | Temporary mining view, then normalization | Named CCGraph simplification, taxonomy and trace back to native IDs |
| Numerical features | Nine category-based counts; generic reference counts can be unknown | Shared interpretation of reference variables and node categories; required unknown facts must block a conformant base run |
| String features | Native interfaces available; candidate filter uses function names | Versioned input/output serialization and aggregation with name similarity |
| Admission | Numerical success or name fallback | Profile decision for paper prose/Algorithm 1 conflict, scale cutoff and threshold boundaries |
| WL | Directed typed edges, histogram intersection, larger-graph normalization, capped depth | Shared label partitions, depth/weights and preliminary/final score policies |
| Output | Report groups remain; explicit `cyclo.ccgraph-pairs/1` streams pair evidence | Shared base still needs projection, parameter strings and score policies |
| Migration | LSH removed; AST admission retained; no silent six-line gate; explicit pair API with default regression tests | Compat base remains unavailable until its policies and fixtures are specified |
| Verification | Local extraction, validation, matching and bounded-score tests; Docker measurements | Independently specified shared fixtures and Rust results; local exhaustive WL is not an original CCGraph oracle |

The current IR can carry approximate or unsupported facts. That permits analysis with limits. It does not permit a base profile to silently turn required unknown facts into zero or an empty string.

## Work order

1. Specify the bounded base profile: projection, feature policies, gates, WL rounds, scoring, final verification and pair output. Keep disputed paper choices named and visible.
2. Add independently specified stage fixtures: source-to-IR, projection, nine counts, three strings, admission, per-round label partitions, scores and accepted pairs. Compare label partitions rather than cross-language numeric hashes.
3. Implement the base behind explicit activation. Preserve the existing extension path and test that boundary. Do not call an unversioned default change conformance.
4. Compare both engines on those fixtures. Only then benchmark the same selected profile, with memory and time measured separately.

No engine architecture, new dependency or JSON transport is needed to complete the first two steps. Memory controls already in place should remain: one native graph, interned repeated facts, temporary matching views, bounded verification batches and no retained AST/compiler objects.

## Limits

Upstream inventory tests are inspected assertions, not passing results from this machine. The PR refers to an inventory prompt and paper PDF that are not included in its file tree; the local paper audit and source search record our separate evidence. The paper does not supply one unambiguous executable score policy. No author executable has been established as an oracle. Shared-profile conformance and original-tool reproduction remain different claims.

See [ccgraph-profile.md](ccgraph-profile.md) for the executable local adaptation and
the reserved compat base. The local adaptation does not resolve shared policy choices.
