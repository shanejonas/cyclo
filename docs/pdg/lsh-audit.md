# Gabel / DECKARD LSH audit

Historical report: production LSH and its test harness are now removed. These measurements describe the prior route.

The Gabel paper uses selected PDG subgraphs, their syntactic images (AST forests),
DECKARD vectors, size partitioning and near-neighbor search. Its connected
components concern nodes inside a procedure's PDG, not hash collisions between
functions. [Gabel §2.2–3.3](https://web.cs.ucdavis.edu/~su/publications/icse08-clone.pdf)

DECKARD §3.3 uses Euclidean-distance LSH, with projections quantized by bucket
width and offset. Algorithm 3 queries a vector's distance-bounded neighbor set.
It does not specify our transitive closure of raw collisions.
[DECKARD §3.3](https://web.cs.ucdavis.edu/~su/publications/icse07.pdf)

## Confirmed implementation differences

- `Vectorize`: whole-function WL histograms folded into 512 coordinates.
- `lshTable.signature`: sign-only projections; no distance radius, offset or width.
- `ClusterClones`: union raw bucket collisions before verification.
- `ccGraphGroups`: verify all admitted pairs inside resulting components.
- No lossless vector-size partitioning or semantic-subgraph-to-AST mapping.

These are substantial algorithm changes, not just different tuning parameters.
A–B and B–C collisions can schedule A–C verification without a direct collision.
Grouping verified matches later is separate from expanding candidate collisions.

## Docker observation

The production-helper audit reads the same nine-feature frozen corpus and verifies
its groups against production. It reports 28,815,436 possible pairs, 16,465,135
paper/AST-filter candidates, 7,592 WL graphs, and one LSH component containing all
7,592 graphs. All candidate pairs reach verification. Diagnostic stage times are
513 ms for pair filtering, 269 ms for LSH, and 58 ms for verification; these are one
instrumented run, not the five-run timing medians.

Raw diagnostic output: `.tmp-build/docker-filter-audit.txt`. Five-run route results:
[exhaustive-comparison.md](exhaustive-comparison.md).

## Consequence

The candidate construction does not reproduce the referenced algorithm. Direct
bucket-neighbor candidates remove transitive expansion, but alone do not reproduce
Euclidean neighbor filtering, size partitioning or Gabel's representations. Any
replacement must measure direct collision counts and reference-pair coverage;
smaller candidate sets do not guarantee preserved recall. The audit initially changed only documentation. Production LSH is now removed.
