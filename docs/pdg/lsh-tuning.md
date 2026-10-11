# LSH tuning experiment

Historical report: production LSH and its test harness are now removed. These measurements describe the prior route.

LSH can be made cheaper. Better direct-bucket selectivity has a measurable coverage cost. Tuning the current component route does not fix its lack of pruning on these corpora.

Production constants and behavior remain unchanged. All settings below run through opt-in tests or temporary Go overlays. The target is the existing WL acceptance predicate, not labeled semantic clones or original CCGraph conformance.

## Sweep and seed checks

Screen: tables `{1,4,8,16}` × hashes `{4,8,12,16,24,32}`, seed 42, on the pinned Cobra, Gin and Prometheus corpora. Follow-up: five configurations at seeds 7, 42 and 99, each repeated in three fresh processes. Every process builds an independent full-score all-pairs reference and checks bounded scoring against it. Retrieved pair scoring is also checked against that reference.

The sweep compares two retrieval semantics:

- **Components:** current production behavior joins overlapping buckets transitively, then verifies admitted pairs within a component.
- **Direct buckets:** a candidate must share at least one bucket directly. A bounded row-word bitset deduplicates unordered pairs; transitive connections do not add candidates.

The direct-bucket bitset is experimental test storage, not a new production implementation. It is quadratic in bits. Direct verification still uses a striped all-pair traversal to consult the bitset; it is not an optimized sparse candidate iterator. Tuning CPU and timings therefore describe this tested implementation, not every possible direct retrieval implementation.

Prometheus has 6,159 unique graph IDs, 18,963,561 unordered pairs, and 34,957 full-score reference pairs. The existing numerical/name/AST admission retains 34,560 of those pairs. LSH-only coverage below uses the full 34,957 reference, so filter loss is not blamed on LSH.

| Tables × hashes | Index median | Direct candidate pairs across seeds | Direct reference coverage across seeds | Component reference coverage |
| --- | ---: | ---: | ---: | ---: |
| 16 × 4, current | 156.5 ms | 15.61–16.00 million | 100% | 100% |
| **4 × 4** | **41.0 ms** | 6.90–8.88 million | **99.87–100%** | **100%** |
| 1 × 4 | 13.0 ms | 1.90–3.47 million | 99.76–99.87% | 99.76–99.87% |
| 1 × 12 | 34.2 ms | 79,248–209,646 | 99.59–99.75% | 99.59–99.75% |
| 4 × 8 | 75.4 ms | 0.99–1.65 million | 99.63–99.98% | 100% |

Index timings include hyperplane creation and adding all vectors; vector creation is outside this table. Medians pool three fresh runs at each of three seeds. Pair counts and coverage are deterministic for a fixed corpus, seed and configuration.

Four tables cut median indexing cost about 74%. With component semantics, both 16×4 and 4×4 still produce one component containing every graph ID on all three corpora at all tested seeds. Thus four tables reduce overhead but remove no admitted verification pairs. Four-table component coverage is an observed result, not a guarantee for other corpora.

Direct 4×4 loses 45 Prometheus reference pairs at seed 7, even though its component variant loses none. Direct 1×12 loses 88–144 reference pairs depending on seed. One-table configurations also lose two of Cobra's 111 reference pairs at seed 7; a single successful seed must not be reported as reliable 100% coverage. Gin's 520 reference pairs survive all five shortlisted configurations at all three seeds.

## Is LSH faster than no LSH?

For Prometheus, the isolated bounded all-pairs reference takes a median 31.8 ms in the follow-up runs. This excludes WL construction. The fastest shortlisted direct retrieval/scoring route, 1×4, takes about 35.1 ms; 1×12 takes about 43.0 ms. Those totals include index construction, direct candidate-bitset construction and verification. They exclude vector construction and component bookkeeping, making them favorable to direct LSH. No shortlisted direct mode demonstrates a scoring-stage speed improvement over bounded all-pairs traversal in this implementation, and one-table modes lose reference pairs.

A separate complete-route comparison holds all existing filters, WL construction and grouping fixed. It compares default 16×4, 4×4, and removal of LSH alone. Five fresh processes per route alternate order. No-LSH here still has the numerical/name/AST candidate filters; it is not the exhaustive route from the earlier report.

| Route | Detector median | Full CLI median | Full CLI range | Peak RSS median |
| --- | ---: | ---: | ---: | ---: |
| Current 16×4 | 830.0 ms | 4.186 s | 3.974–6.171 s | 602.7 MiB |
| Tuned 4×4 | 750.0 ms | 4.649 s | 4.334–5.826 s | 608.8 MiB |
| Existing filters, no LSH | 671.0 ms | 4.140 s | 3.878–4.200 s | 607.3 MiB |

Four tables lower the detector median by 9.6%, but the complete scan median is **slower** in this batch. There is no demonstrated full-scan speedup from this tuning. Full-command variation is large; do not replace that observation with a causal claim about background load or compiler behavior. Removing LSH lowers the detector median 19.2%, while its full-scan median differs by only 1.1%. Neither peak-RSS comparison establishes a memory improvement.

All fifteen route runs have the same clone-group digest and report content after ignoring top-level candidate order. This does not prove accepted pair equivalence on arbitrary corpora; the sweep's reference-pair checks are the separate retrieval evidence.


## What to use

For an optional extension using the current component semantics, **4 tables × 4 hashes, seed 42** is a cheaper tested setting. It preserves observed pair coverage and groups on these corpora, but has no retrieval benefit here. Keeping it is a compatibility choice, not evidence that LSH helps matching.

For direct-bucket experiments, **1 table × 12 hashes** provides a useful selective test point, with known losses. Do not enable it as a recall-preserving optimization. Any future sparse iterator or vector change needs a new timing and accepted-pair coverage check.

No setting is selected for the compat CCGraph base. Whole-function LSH remains an extension, distinct from the paper's label compression. These results do not justify silently changing detector defaults or equating approximate retrieval with the WL acceptance predicate.

## Evidence and limits

Historical harness (now removed): `TestTuneLSH` in `domain/patterns/lsh_tuning_test.go`. Set `CYCLO_MATCH_CORPUS` and `CYCLO_LSH_TUNING_OUTPUT`. Optional `CYCLO_LSH_SETTINGS` is a JSON array of `{Tables,Hashes,Seed}` records. Invalid table/hash settings fail before indexing. A normal regression fixture confirms that direct A–B and B–C bucket links do not introduce A–C.

[Raw sweep and route results](lsh-tuning-results.json) retain corpus revisions, every screened/shortlisted setting, seed, candidate count, reference hits, allocations and fresh-process route measurements. Local overlays, binaries and complete logs are in `.tmp-build/lsh-tuning`.

Scope and limits are the same as [the performance profile](performance-profile.md): one Linux host, warm compiler/dependency caches, root-module production functions, current function-ID collisions, current WL labels and score. No semantic accuracy, universal coverage, larger-corpus advantage, parameter optimality or cross-language conformance is established. A timing batch that overlapped local verification is discarded and excluded from the reported route medians.
