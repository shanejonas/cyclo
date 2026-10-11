# Filtered versus exhaustive on other repos

Historical measurements from earlier worktree snapshots. Production now removes LSH and adds explicit return-to-output links. See [return-link-timing.md](return-link-timing.md) for the final return-link comparison.

Exhaustive routing is faster on all three additional repos in this run. This compares Cyclo routes using the same current IR and WL predicate; it does not compare against the original CCGraph tool or measure semantic clone accuracy. Production routing is unchanged.

| Repo | Unique usable graph IDs | Full filtered | Full exhaustive | Detector filtered | Detector exhaustive | Peak RSS filtered / exhaustive |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Cobra | 269 | 0.169 s | 0.160 s | 19.9 ms | 10.1 ms | 39.4 / 39.2 MiB |
| Gin | 483 | 0.247 s | 0.232 s | 31.9 ms | 14.8 ms | 61.9 / 60.7 MiB |
| Prometheus | 6,159 | 4.079 s | 3.584 s | 812.9 ms | 355.1 ms | 614.7 / 608.9 MiB |

Medians of five fresh process runs. Full-scan reductions are 5.7%, 5.8% and 12.1%; isolated detector reductions are 49.0%, 53.5% and 56.3%. Small full-scan differences on Cobra/Gin are sensitive to process and loader noise. Peak-memory differences are small; no general memory reduction claim follows.

## Output checks

Both routes have identical sorted clone groups on every run: Cobra 36, Gin 34, Prometheus 494. Each exhaustive run also checks its groups against independent full-score all-pairs evaluation outside the timed detector stage.

Cobra and Gin complete reports are byte-identical across both routes and all runs. Prometheus reports have identical content after sorting the top-level candidates list by the full candidate record. Two specification findings can swap order between fresh processes; this also occurs within the same route. Raw byte hashes therefore differ. No candidate content is removed for the comparison. Equal groups or report content do not imply equal accepted pair sets.

Prometheus extraction contains 6,173 functions, but the existing function-ID map
reduces this to 6,159 graph entries: multiple package `init` functions share IDs.
Both benchmark routes use that same map. This is a separate identity limitation,
not evidence that every extracted function is compared independently.

## Inputs and method

- Cobra: `spf13/cobra`, commit `adbc8813901bba65827259daa8e22ff94ec1f30e`.
- Gin: `gin-gonic/gin`, commit `0f09c3a9b4626d9fe9979cf4d0521d6ae32ab646`.
- Prometheus: `prometheus/prometheus`, commit `2213c3c192574b8a339acd336ea6385974a7567b`.
- Linux/amd64, Go 1.27.1, 24 reported CPUs. Root-module build-selected production functions; tests and nested modules excluded.
- Repos are separate read-only benchmark inputs under `/tmp/cyclo-bench-{name}`. No input source changes are made. Compiler/dependency caches are populated during extraction before timed runs; these are warm-cache timings, not cold dependency downloads.
- Normal and [exhaustive overlay](measure-exhaustive.py) builds come from the same current worktree. Default reports retain AST bypass and LSH. Exhaustive routing removes those stages and numerical/name admission, retaining the safe WL bound and bounded result batches.
- Frozen inputs come from `TestWriteMatchingCorpus`. `TestMeasureMatchingRoute` measures complete detector routing/grouping in separate fresh test processes. Complete CLI runs execute `patterns --format json .`.
- Route order alternates each run. Measurements are sequential to avoid competing benchmark workloads. Full CLI peak RSS uses a fresh Python process and `resource.getrusage(RUSAGE_CHILDREN)`, as in the earlier Docker measurement. Allocated bytes in API output are cumulative allocations, not peak memory.

[Raw results](multi-repo-timing-results.json) retain every run, source revisions, corpus hashes, output hashes, allocation measurements and environment. Local binaries, frozen corpora, runner scripts and full reports are under `.tmp-build/repo-timings`; binaries have the `.tmp-build/repos-` prefix.

These samples support the earlier observation: filtering costs more than it saves on these inputs. They do not establish that exhaustive wins on all inputs, or on corpora larger than Docker. No detector default is changed by this benchmark.
