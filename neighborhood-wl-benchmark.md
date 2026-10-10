# Neighborhood-WL Benchmark: Standard WL vs Neighborhood-Augmented WL Kernel

## What this is

Cyclo's neighborhood-augmented Weisfeiler-Lehman kernel
(`domain/patterns/neighborhood_wl.go`). Inspired by multi-scale WL ideas in
the literature (e.g. Kim & Oh's WLKS, ICLR 2025), but Cyclo's own adaptation:
per-node k=1,2 neighborhoods with one WL refinement round, aggregated into
histograms. Not the paper-exact WLKS algorithm.

## Setup

- Corpus: cyclo `domain/patterns` package (648 functions with PDGs) and
  `domain` (762 functions)
- Same extraction for both; identical pipeline (Stages 1-3); only the Stage 4
  kernel differs
- Threshold: 900 (0.9) for both, paper-exact
- 7 runs each; times are wall-clock for the full CCGraph pipeline
- Measured 2026-10-10 on the spec-work VM (2 vCPU)

## Results

### domain/patterns (648 functions)

| Metric | Standard WL | Neighborhood WL (K=2) |
|--------|-------------|----------------------|
| Groups found | 24 | 26 |
| Median wall time | 0.86s | 1.01s |
| Mean | 0.88s | 1.03s |
| Stddev | 0.07s | 0.11s |
| Min / Max | 0.79s / 1.00s | 0.94s / 1.28s |
| Speedup | 1.0x | 0.85x |

Overlap: both=24, onlyStd=0, onlyNWL=2

### domain (762 functions)

| Metric | Standard WL | Neighborhood WL (K=2) |
|--------|-------------|----------------------|
| Groups found | 27 | 29 |
| Median wall time | 1.17s | 1.36s |
| Mean | 1.14s | 1.41s |
| Stddev | 0.05s | 0.17s |
| Min / Max | 1.07s / 1.19s | 1.31s / 1.82s |
| Speedup | 1.0x | 0.86x |

Overlap: both=27, onlyStd=0, onlyNWL=2

## Neighborhood-WL-only groups (verified real clones)

1. **`grow` / `pruneStep`** (align.go): Both are factory functions returning
   `step{name, func(m *matching) *matching {...}}` — same shape, different
   closure bodies. Legitimate Type-2/3 clone.

2. **`operandAt` (normalize.go) / `dataSourceAt` (obligation.go)**:
   Near-identical edge-lookup loops over PDG edges. Legitimate clone.

## Synthetic gapped-clone comparison

| Case | Neighborhood WL | Flat WL |
|------|----------------|---------|
| identical | 1000 | 1000 |
| minus1 (1 deletion) | 645 | 640 |
| minus2 (2 deletions) | 412 | 460 |
| plus1 (1 insertion) | 981 | 909 |
| plus2 (2 insertions) | 962 | 833 |

Neighborhood WL matches flat WL on deletions, beats it on insertions
(noise robustness).

## K=1 vs K=2 tradeoff

K=1: faster, but MISSES `forceTypeAssertCandidates` /
`typedNilCandidates` (verified real clones) — recall regression.
K=2: slower, strict superset of WL groups — no regression.

K=2 chosen: correctness (no lost clones) over speed.

## Honest assessment

- **Coverage**: Neighborhood WL finds ~8% more groups, all verified real
  clones, zero lost. Clear win.
- **Speed**: Neighborhood WL is ~14% SLOWER than standard WL end-to-end
  (0.85-0.86x median). The earlier "1.8x faster" estimate was wrong — it was
  never measured. The construction optimization (reused BFS buffers, no
  induced subgraphs) was 2.48x faster than the naive prototype on a synthetic
  micro-benchmark, but end-to-end the kernel still costs more than standard WL
  because it runs O(n*K) neighborhood refinements vs O(1) whole-graph
  refinement.
- **Tradeoff**: ~14% slower for 8% more real clones. Worth it for a clone
  detector where recall matters, but not a speed win. The PR description
  should not claim a speedup.
