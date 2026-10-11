# Matching feature reuse and filter reference

Historical measurements from earlier worktree snapshots. Production now removes LSH and adds explicit return-to-output links. See [return-link-timing.md](return-link-timing.md) for the final return-link comparison.

The matcher now computes each graph-vector norm once and shares base WL label
hashes from the IR's interned MatchLabel tables. Hash keys contain the same fields
as the existing matcher labels. They use content, not local table IDs. Normalized
nodes use their current fields; labels absent from the shared cache use the
original hash function. The published cache is read-only.

The canonical PDG's node and edge sizes do not change. Temporary mining views
carry one additional cache pointer. Cache entries scale with distinct matching
labels, not function pairs. No per-pair evidence or vectors are stored in the IR.
Thresholds, candidate rules and clone output stay unchanged.

## Docker measurements

Source: Moby commit `9fabd6d`, default Linux root module, excluding tests and nested
modules. Extraction produces 7,601 functions; 7,592 have nonempty matching graphs.
The frozen input is `.tmp-build/docker-matching-corpus.json`. This is a benchmark
artifact, not a production IR export. Measurements run on this Linux amd64 host.

Three fresh processes per version replay the same input through the CCGraph
stages. The before binary contains the native IR migration and the original
matcher; the after binary adds shared label hashes and cached vector norms.

| Stage | Before median ms | After median ms |
| --- | ---: | ---: |
| Shared label cache | — | 22.929 |
| Graph vectors and AST shapes | 25.548 | 21.652 |
| Pair filters | 451.715 | 414.997 |
| WL features | 294.207 | 273.703 |
| LSH | 200.581 | 174.450 |
| Verification | 24.731 | 20.519 |
| Sum of stages, median per run | 1,031.470 | 928.583 |

The measured stages improve by 10.0%. Stage medians do not sum to the median run.
Replay builds the label cache by visiting mining nodes; production builds it from
unique shared IR tables. Verification-pair counting is outside the timed stages.
These measurements exclude extraction and the other pattern analyses.

Both versions yield 4,190,873 candidate pairs and 26,680 accepted pairs. The clone
group digest is identical:
`c9cef7fb34c436e66560218a4c3b57a30b6ebd6a636271151ee96e9330780348`.
The after harness also checks equivalence with the production CCGraph entrypoint.
A separate complete `patterns --format json .` run produces byte-identical reports:
4.640 seconds before and 4.185 seconds after. These are single runs, not a stable
end-to-end benchmark. Both runs report peak child RSS of 530,608 KiB; this single
sample cannot establish a small memory difference.

The first replay run reduces WL allocation calls from about 3.16 million to
3.05 million and allocation volume from 124.2 MB to 121.9 MB. Those are temporary
allocations, not retained PDG size. See [matching-results.json](matching-results.json)
for all recorded runs.

## Exhaustive reference

The reference tests all 28,815,436 unordered pairs without the name, vector, AST
or LSH candidate filters. A pair is accepted when its existing WL similarity is
at least 900 milli. This measures recall against our WL rule; it is not a labeled
semantic clone ground truth.

| Outcome | Pairs |
| --- | ---: |
| Exhaustive WL matches | 32,627 |
| Kept by current pipeline | 26,680 |
| Missed by name filter | 5,945 |
| Missed by vector filter | 2 |
| Missed by LSH after pair filters | 0 |

The current filters retain 81.8% of this reference. The AST bypass is included
when attributing misses. On this corpus the name and vector misses are disjoint.
LSH produces one connected component containing all 7,592 graphs, so it does not
further reduce verification work here.

An independent full-score pass and the existing safe WL upper-bound predicate
produce exactly the same reference pairs in each of three processes. The bounded
pass runs first. Median pair-comparison time is 34.830 ms with the bound versus
1,044.049 ms with full scores. This excludes WL construction and grouping. It is
not an end-to-end speed claim. It suggests that a bounded exhaustive route is worth
measuring before adding more candidate filters. Production routing stays unchanged.

The local dataset is `.tmp-build/docker-clone-reference.json` (1,181,588 bytes).
It stores one sorted function-ID dictionary and 32,627 pairs of dictionary indexes,
with schema version 1 and threshold 900. SHA-256:
`67aeeddaad121eeb909ba77ff3f74a30ebb42086f88234c040d4c31c5f61c559`.
The large corpus and dataset remain local artifacts, outside the source change.

## Reproduce

The measurement tests skip unless their environment variables are set. Run these
commands from the cyclo checkout, with a local Moby checkout in the indicated path:

```sh
CYCLO_MATCH_ROOT=/tmp/cyclo-docker-source \
CYCLO_MATCH_CORPUS="$PWD/.tmp-build/docker-matching-corpus.json" \
go test ./adapters/gopatterns -run '^TestWriteMatchingCorpus$' -count=1 -v

CYCLO_MATCH_CORPUS="$PWD/.tmp-build/docker-matching-corpus.json" \
CYCLO_MATCH_VERIFY=1 \
go test ./domain/patterns -run '^TestMeasureMatchingPipeline$' -count=1 -v

CYCLO_MATCH_CORPUS="$PWD/.tmp-build/docker-matching-corpus.json" \
CYCLO_MATCH_EXHAUSTIVE=1 \
CYCLO_MATCH_DATASET="$PWD/.tmp-build/docker-clone-reference.json" \
go test ./domain/patterns -run '^TestMeasureExhaustiveClones$' -count=1 -v
```

Tests compare cached cosine values bit for bit with the previous calculation and
check WL hashes before and after normalization mutations. Race tests cover the
pattern, extraction and IR packages. `make check`, `make cognitive`, `go vet ./...`
and the refreshed quality gate pass.

## Paper-based candidate admission correction

The subsequent correction follows Algorithm 1's numerical admission OR string
fallback. Filters, AST bypass, LSH and the unchanged WL threshold remain in use.
The publication's ambiguity and remaining implementation gaps are documented in
[paper-audit.md](paper-audit.md). The results above describe the earlier AND route.

Three alternating runs compare the saved matcher with shared features against the
corrected matcher on the same frozen input, followed by complete Docker reports.

| Measure | Previous AND route | Corrected alternative route |
| --- | ---: | ---: |
| Candidate pairs | 4,190,873 | 17,992,778 |
| Accepted WL pairs | 26,680 | 32,627 |
| Exhaustive-reference coverage | 81.8% | 100% on this corpus |
| Median matching stage sum, ms | 912.603 | 922.252 |
| Median complete report time, s | 4.119 | 4.174 |
| Median peak RSS, MiB | 505.68 | 495.26 |

The reference check independently applies numerical and name scores to full-score
WL matches, then checks the production groups. All groups agree. The fixed group
digest is `b2d9e7d7912cfa78697d92553103c6889ca8b9c60b3be8399631cc2f44519978`. Production output now includes the
recovered matches, so reports intentionally differ from the previous AND route.

More pairs reach WL verification. The measurements do not establish equal speed
or guaranteed recall on other repositories. The canonical PDG layout is unchanged.
Candidate bitset capacity is unchanged; accepted pairs now use bounded batches
instead of accumulating full per-worker match lists. Full raw runs appear in
[matching-results.json](matching-results.json).

## Nine source features and execution-edge WL

The later execution/reference change uses a fresh corpus and changes the WL
reference. Historical timing and recall above do not describe this new kernel.
See [nine-features.md](nine-features.md) for policies, current recall and paired
memory/time measurements, and [nine-feature-results.json](nine-feature-results.json)
for raw runs. The shared hashes, cached norms, bounded match batches, thresholds,
Algorithm 1 admission rule, AST bypass and LSH remain in use.
