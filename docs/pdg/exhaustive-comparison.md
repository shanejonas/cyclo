# Filtered versus exhaustive CCGraph

Historical measurements from earlier worktree snapshots. Production now removes LSH and adds explicit return-to-output links. See [return-link-timing.md](return-link-timing.md) for the final return-link comparison.

On Docker, exhaustive CCGraph is 18.8% faster for the full report. Peak process memory is about the same. This benchmark does not change the production default.

| Median of five runs | Filtered | Exhaustive |
| --- | ---: | ---: |
| Full report wall time | 4.788 s | 3.888 s |
| Complete CCGraph stage, isolated | 1131.2 ms | 373.2 ms |
| CCGraph allocated bytes, cumulative | 177.23 MiB | 127.81 MiB |
| Full report peak RSS | 507.60 MiB | 513.02 MiB |
| Accepted reference pairs | 32,184 | 32,603 |
| Final groups | 484 | 484 |

## What this proves

Both routes use the same compact IR, execution edges, reference counts, labels, WL threshold (900/1000), shared label cache and bounded result batches. The exhaustive build removes the numerical/name/AST candidate filters and LSH stage. It retains the safe WL similarity upper bound, which avoids full scoring when a pair cannot reach the threshold.

An independent full-score reference checks all unordered pairs. Exhaustive groups match that reference. A separate property test checks the bounded WL predicate against full scores with execution edges. The filtered route keeps 98.7% of reference pairs; exhaustive keeps 100%. This is coverage of this WL predicate on this corpus, not semantic clone recall against labeled examples.

The extra 419 accepted links connect functions already in the same groups. Final complete CLI reports are byte-identical, and all runs have the same group digest. Removing filtering improves pair coverage here without changing the report.

The filters cost more than they save on this input. Exhaustive avoids their feature, candidate-matrix and LSH work. The native IR stays unchanged. Pair results stream through bounded batches; the benchmark does not retain all candidate pairs. The exhaustive search still considers O(n²) pairs, so this timing does not establish its cost on larger or less similar corpora.

## Method and limits

Input: Moby/Docker commit prefix `9fabd6d`, root Go module, Linux build, tests and nested modules excluded. Extraction has 7,601 functions and 7,592 usable graphs. This is the same nine-feature corpus used in [nine-features.md](nine-features.md).

Each route runs five times in fresh processes, alternating which route runs first. The isolated API stage includes complete CCGraph routing and grouping; corpus loading and independent reference verification are outside that stage. Complete CLI runs include extraction and the other report analyses. API and CLI measurements are separate runs. Cumulative allocated bytes are not peak or retained memory. Peak RSS varies between runs; the small increase in its median does not support a memory reduction claim.

[Raw results](exhaustive-comparison-results.json) include every run. Local outputs and binaries are in `.tmp-build/`. Production remains filtered. No commit or push is made.

## Reproduce the alternate build

From the repository root, [measure-exhaustive.py](measure-exhaustive.py) writes a temporary Go overlay of `ccgraph.go`. It keeps production source unchanged. Create `.tmp-build` first, then run:

```sh
python3 docs/pdg/measure-exhaustive.py
go build -buildvcs=false -o .tmp-build/cyclo-matching-nine-filtered .
go build -buildvcs=false -overlay .tmp-build/exhaustive-overlay.json -o .tmp-build/cyclo-matching-nine-exhaustive .
```

For isolated timings, generate a frozen corpus with `TestWriteMatchingCorpus` using `CYCLO_MATCH_ROOT` and `CYCLO_MATCH_CORPUS`. Build normal and overlay test binaries for `./domain/patterns`. Run `TestMeasureMatchingRoute` with that same corpus and `CYCLO_MATCH_METHOD=filtered` or `exhaustive`, using the corresponding binary. Set `TMPDIR` and `GOTMPDIR` if the host temporary directory has insufficient space.

See [multi-repo-timing.md](multi-repo-timing.md) for the same comparison on Cobra,
Gin and Prometheus using the current stabilized routing boundary.
