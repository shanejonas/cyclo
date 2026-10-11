# Graph and detector performance report

Historical report: production LSH and its test harness are now removed. These measurements describe the prior route.

Both routing overhead and graph construction need work. On these repos, candidate filtering plus LSH costs more than it saves. WL construction also allocates far more memory than its retained summaries require. Compact native storage helps retained memory; it does not make construction allocation-free.

This report measures the current worktree on the same pinned Cobra, Gin and Prometheus revisions as [the full-scan comparison](multi-repo-timing.md). No production detector, threshold, graph layout or default changes are made for this report.

## Detector stages

Median wall milliseconds from five fresh, unprofiled processes per repo. These calls use the same production helpers as the filtered route; every run checks the resulting groups against production. The LSH helper is split into vector creation, hyperplane creation, indexing and connected components.

| Stage | Cobra | Gin | Prometheus |
| --- | ---: | ---: | ---: |
| Shared matching labels | 0.45 | 0.69 | 18.64 |
| Numerical/name/AST inputs | 0.28 | 0.50 | 12.37 |
| Candidate pair filters | 1.65 | 1.81 | **347.60** |
| WL summary construction | 9.33 | 14.32 | **312.01** |
| Vector creation | 0.60 | 0.86 | 5.74 |
| LSH hyperplanes | 0.23 | 0.20 | 0.15 |
| LSH indexing | 7.79 | 13.94 | **120.98** |
| LSH connected components | 0.63 | 1.15 | 14.43 |
| WL pair verification and grouping | 0.32 | 0.58 | **24.53** |

Instrumented stage medians are not additive estimates of complete CLI time. Shared-label setup precedes the isolated route timer in the earlier comparison. GC, scheduling and instrumentation change individual runs.

| Retrieval observation | Cobra | Gin | Prometheus |
| --- | ---: | ---: | ---: |
| Unique usable graph IDs | 269 | 483 | 6,159 |
| All unordered pairs | 36,046 | 116,403 | 18,963,561 |
| Admitted filter pairs | 19,268 | 60,273 | 10,435,875 |
| LSH components | **1** | **1** | **1** |
| Largest component | **269** | **483** | **6,159** |
| Pairs sent to verification | 19,268 | 60,273 | 10,435,875 |
| Accepted filtered pairs | 111 | 520 | 34,560 |

LSH removes **zero** admitted verification pairs on all three corpora. Overlapping buckets are joined transitively, and verification then compares admitted pairs anywhere in that component. This is broader than comparing direct bucket mates. The current four sign hashes provide at most 16 buckets per table; there are 16 tables. This configuration and component construction produce poor selectivity here.

Interned IR strings and types do not change the logical vectors or their collisions. Folding WL colors into 128 buckets per round is a separate lossy representation. Its contribution to collisions is not isolated by this report; do not assign all lost selectivity to folding or storage compression.

## Allocated versus retained memory

Prometheus detector-stage cumulative allocations per pass, medians of five runs:

| Stage | Allocated MiB | Allocations |
| --- | ---: | ---: |
| Shared labels | 0.60 | 2,296 |
| Feature inputs | 2.70 | 6,169 |
| Candidate filters/store | 5.10 | 6,292 |
| **WL construction** | **118.91** | **3,084,888** |
| Vector creation | 24.43 | 6,177 |
| LSH planes | 0.26 | 99 |
| LSH indexing | 4.31 | 2,471 |
| LSH components | 0.64 | 36 |
| Verification/grouping | 2.79 | 5,741 |

Allocation profiles of the filtered detector attribute about **76% of sampled allocated bytes** and **98% of sampled allocated objects** to `NewWlLight`. Main allocation sites are `digest`, `adjacency`, `histogram`, and sorting through `reflectlite.Swapper`. These are temporary neighbor lists, refinement scratch buffers and histogram work, not just retained PDG nodes.

Light versus full storage uses the same execution-aware WL computation in both modes. Full mode keeps adjacency, round history and the input pointer; light mode discards them. Three fresh processes per mode:

| Repo | WL light retained | WL full retained | Allocated in either mode |
| --- | ---: | ---: | ---: |
| Cobra | 0.30 MiB | 0.94 MiB | about 3.07 MiB |
| Gin | 0.47 MiB | 1.41 MiB | about 4.45 MiB |
| Prometheus | **9.78 MiB** | **35.19 MiB** | **about 118.6 MiB** |

Light storage cuts retained WL heap by about 72% on Prometheus, but both modes construct the same temporary data. Keeping fewer fields is therefore a memory improvement, not proof of lower construction cost. Timing differences between these three-run probes are noisy; no light-versus-full speed claim is made.

## Extraction and graph views

Three fresh unprofiled processes per repo. Native graph construction runs after typed package loading, with compiler inputs kept live. View and normalization measurements keep input and output live through GC, so retained values are marginal heaps for those stages.

| Stage | Cobra | Gin | Prometheus |
| --- | ---: | ---: | ---: |
| Complete extraction API | 97.4 ms | 162.8 ms | **1,573.9 ms** |
| Native graph construction only | 19.5 ms | 26.1 ms | **556.3 ms** |
| Native-to-mining view | 0.53 ms | 0.86 ms | **16.7 ms** |
| Canonicalization, no rules | 0.19 ms | 0.31 ms | **6.9 ms** |
| Canonicalization, all rules | 16.6 ms | 21.8 ms | **570.9 ms** |

The no-rule clone probe uses three runs for Cobra/Gin and two unprofiled runs for Prometheus; a third Prometheus run profiles native construction and is excluded. All-rule canonicalization is an optional workload, **not the CLI default**. The default abstraction preparation still clones each graph with rules disabled.

Prometheus marginal storage and cumulative allocation:

| Representation/stage | Retained MiB | Allocated MiB |
| --- | ---: | ---: |
| Complete extraction result | 82.52 | 1,057.28 |
| Native graphs only | **53.47** | **407.16** |
| Temporary mining views | **34.02** | **35.68** |
| No-rule canonical graph copies | **32.79** | **32.79** |
| All-rule canonicalization | **35.51** | **1,464.33** |

These are separate marginal measurements, not additive estimates of process peak RSS. Complete extraction includes loading/type checking and other fact extraction. All-rule normalization repeatedly constructs intermediate graphs, explaining why cumulative allocations greatly exceed the final graph size. The native builder's largest sampled allocation sites are node append/growth, attribute packing and edge append/growth.

On this 64-bit machine, native node/edge records are 44/56 bytes. Mining node/edge records are 112/40 bytes. Native nodes also use separate spans and sparse attributes, and mining strings share backing data; record size alone is not a fair total-memory comparison. The measured view heap, not just struct size, establishes its cost.

## CPU profiles

CPU profiles are separate from timing runs. The matching profiles repeat for at least three wall seconds: four filtered passes versus ten exhaustive passes. CPU samples sum across threads, so they can exceed wall time; percentages below are not wall-time percentages.

- Filtered detector: candidate-filter worker loops account for about **86% of CPU samples**, with name matching about **72% cumulative**. `jaroFindMatches` alone is 46% flat. The profile contains 28.25 core-seconds across four passes.
- Exhaustive detector: all-pair verification workers account for about **42% cumulative**; WL construction and GC account for much of the remainder. It contains 7.51 core-seconds across ten passes. Unequal pass counts mean raw profile totals must not be compared as per-run allocations.
- Complete filtered CLI: name matching accounts for about **39% cumulative CPU**, candidate admission about **46%**. Native extraction, type checking, other pattern consumers, formatting and GC remain substantial.
- Complete exhaustive CLI: no name-filter work appears. GC is about **33% cumulative CPU** in this one profile, and scoring/intersection remains visible.
- Native graph construction: natural GC and map operations are visible costs. Forced before/after snapshot GCs are excluded from this CPU profile. Its short 0.52-second duration limits precise function ranking; allocation measurements provide stronger evidence for construction churn.

The complete CLI allocation profile samples about 2.05 GiB of cumulative allocation, **not peak memory**. It also identifies alignment label classes and matching-pair materialization as substantial costs outside this clone route. Reducing only LSH will not remove those costs.

## Recommended work

1. **Fix retrieval semantics before tuning LSH.** Test direct bucket-pair candidates as a separate extension. Sweep hashes/tables and measure candidate count, accepted-pair coverage, CPU, wall time and memory. Compare against the fixed WL predicate. More hashes can improve selectivity but can lose accepted pairs; no setting is proved safe here. Keep this outside the compat CCGraph base.
2. **Reduce WL construction allocations without changing scores.** Reuse per-graph refinement scratch buffers; replace reflection-based sorting with typed sorting; pre-size or pack adjacency storage. Verify labels, histograms, scores and threshold decisions against the existing fixtures and exhaustive reference.
3. **Reduce duplicate graph storage.** Check ownership before avoiding the default no-rule graph copy. Packed mining labels/attributes or a read-only indexed view may reduce the measured 34 MiB temporary view and 33 MiB canonical copy. Preserve immutable inputs and consumer behavior.
4. **Reduce native builder growth and packing copies.** Use the node/edge/attribute allocation stacks to guide capacity planning. Measure retained heap, temporary allocations and full extraction time together; do not trade much more retained memory for a small construction gain without evidence.
5. **Profile alignment consumers separately before changing them.** The complete CLI profile shows costs beyond clone detection. Their pair and class structures need their own bounded behavior checks.

This report supports the user's graph-cost concern. It also confirms that existing retrieval overhead is real. No measured optimization is implemented here, and no paper-fidelity or semantic-accuracy improvement is claimed.

## Evidence and reproduction

[Raw measurements and profile tables](performance-profile-results.json) include pinned input revisions, corpus hashes, all stage/storage/extraction runs, binary hashes, and CPU/allocation top tables. Full-scan wall/RSS measurements remain in [multi-repo-timing-results.json](multi-repo-timing-results.json).

Opt-in harnesses:

- `TestMeasureMatchingPipeline`: set `CYCLO_MATCH_CORPUS`, and `CYCLO_MATCH_VERIFY=1` to compare with production.
- `TestMeasureWlStorage`: set `CYCLO_MATCH_CORPUS` and `CYCLO_WL_STORAGE=light` or `full`.
- `TestMeasureGraphStorage`: set `CYCLO_STORAGE_ROOT`; optionally set `CYCLO_STORAGE_PROFILE` to an absolute profile prefix.
- `TestMeasurePDGMemory`: set `CYCLO_PDG_MEMORY_ROOT` and `CYCLO_PDG_MEMORY_MODE=api-new`.
- `TestProfileMatchingRoute`: set `CYCLO_MATCH_CORPUS` and `CYCLO_PROFILE_PREFIX`.
- `TestProfilePatternCLI`: set `CYCLO_CLI_ROOT` and `CYCLO_CLI_PROFILE`.

Build each test binary normally; build the alternate route with the [exhaustive overlay](measure-exhaustive.py). Use fresh processes, alternate routes, and do not overlap measurements. Analyze `.cpu` files with `go tool pprof -top`; analyze allocation deltas with `-sample_index=alloc_space -base PREFIX-before.heap PREFIX-after.heap`. Local binaries, complete logs, runner scripts and binary profiles remain under `.tmp-build/stage-profiles`.

Scope: Linux root-module production functions, no tests or nested modules; warm dependency/compiler caches; one host. Prometheus has 6,173 extracted functions but 6,159 map entries because existing package `init` IDs collide. Both routes share that limitation. Stage timings use frozen raw matching views, while the full CLI performs its usual preparation and other consumers. Profile heap totals are sampled and include small instrumentation/runtime costs; exact allocation totals come from `runtime.MemStats`. No cache-enabled, cold-start, larger-than-Docker, cross-machine or hardware-counter/cache-miss study is included.

The [LSH tuning experiment](lsh-tuning.md) now measures parameter sweeps, seed
variation, direct-bucket coverage and whole-route timing.
