# Explicit return-link timing

The new return links add little elapsed time in this Prometheus comparison.
This is a local producer and exporter comparison, not semantic conformance or
original CCGraph performance evidence.

| Mode | Without return-link pass | With return-link pass | Change |
| --- | ---: | ---: | ---: |
| Pattern analysis, JSON report | 3.922 s | 4.010 s | +2.2% |
| Compat PDG JSON export | 6.660 s | 6.686 s | +0.4% |

Median peak RSS is 589.0 → 623.6 MiB for pattern analysis, and 669.4 → 675.6 MiB
for export. RSS is a whole-process peak, not retained IR storage. Three trials
per configuration are too few to establish a general performance claim; small
time differences can be run variation.

## Method

- Prometheus commit `2213c3c192574b8a339acd336ea6385974a7567b`, the same approximately 6,173 extracted functions used in the prior measurements.
- Two binaries from the final worktree, both without LSH. The comparison overlay removes only the `returnValues` call in `buildGraph`; output-node bookkeeping remains in both builds.
- Three fresh processes per route and mode, alternating before/after order. Existing host Go build cache; Cyclo WL cache disabled. Output goes to `/dev/null`, so no output-file storage latency is measured.
- Wall time from a monotonic parent-process clock. Peak RSS from `wait4` resource usage on Linux.
- An initial attempt using a separate temporary Go cache hit disk quota while compiling dependencies. Its partial, cold-cache trial is discarded and is absent from the results.
- Pattern analysis and PDG export are separate commands. Export skips mining. The return links add canonical data-edge counts; formal outputs stay outside the WL mining view. This comparison does not assert identical accepted pairs.

Raw trials: [return-link-timing-results.json](return-link-timing-results.json).
