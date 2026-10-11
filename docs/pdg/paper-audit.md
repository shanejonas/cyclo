# CCGraph filtering audit

Source: [Zou et al., ASE 2020](https://yinxingxue.github.io/papers/ase2020_CCGraph%20A%20PDG%20based%20Code%20Clone%20Detector%20With%20Approximate%20Graph%20Matching.pdf),
sections 3.2–3.4, Table 1 and Algorithm 1, pages 5–6 of the PDF.

## Candidate admission fix

The previous implementation requires numerical similarity AND name similarity,
except for cyclo's AST bypass. Algorithm 1 explicitly adds a numerical match to
the candidate set and continues before testing strings. Strings form an alternative
admission route when the numerical score fails. The implementation now uses:

```
AST bypass OR numerical cosine >= 0.9 OR name Jaro-Winkler >= 0.5
```

This follows the pseudocode's admission control flow. The publication is ambiguous:
its prose describes sequential filtering, and Algorithm 1 swaps the numerical and
string threshold symbols. We use the numeric values stated in sections 3.3.1 and
3.3.2, including equality at the thresholds. We do not claim this resolves the
ambiguity in the authors' implementation. The name-only string fallback remains
cyclo's adaptation; Table 1 also lists incoming and return parameters.

A high numerical score therefore survives dissimilar names. A low numerical score
can survive similar names. A pair failing both is still rejected unless the AST
bypass admits it. The WL threshold is unchanged at 900 milli. LSH remains the added
scaling stage. It can miss pairs on other corpora and is not a recall guarantee.

The shared IR, shared content hashes, cached vector norms and safe WL bound remain.
Accepted pairs now stream in batches of at most 64. The queue holds at most one
batch per worker, with one additional active batch per worker and one at the
consumer. Worker match storage no longer grows with accepted-pair count. The
existing candidate bitset still uses O(functions squared) bits, approximately
6.89 MiB for Docker's 7,592 usable functions. No extra evidence is stored per pair.

## Remaining differences

| Paper component | Current implementation |
| --- | --- |
| Nine numerical features, including execution edges and reference variables | Nine canonical source features; execution order is approximated and generic reference kinds remain unknown |
| Declaration and assignment categories | Counts use canonical Declaration/Assignment categories, including bindings omitted by the abstraction labels; arithmetic is Computation |
| Incoming/return parameters and names as string characteristics | Name fallback only; full interface types exist in the new IR but are not yet used here |
| Minimum size of six lines | Not applied by the CCGraph entrypoint; compact mining nodes are not source lines |
| Graph scale-ratio threshold | Not applied separately; the publication does not state its numeric value in the audited section |
| System-node removal and call-subgraph merging | Mining projection excludes metadata nodes and has opaque call nodes; not an exact Java/C extractor reproduction |
| Directed labeled WL, diameter-based rounds and weights | Adapted WL with cyclo/rstyle labels and a refinement-depth cap |
| Candidate-cluster LSH | Cyclo addition; not a separate candidate stage in Algorithm 1 |
| High-AST-similarity bypass | Cyclo addition |

Execution analysis now produces a sparse must-precede basis using statement order
and dominance, under a named Go policy. CFG successors are not counted as execution
dependencies. Reference bindings are classified from Go types. Unknown generic
reference counts cannot reject candidates. See [nine-features.md](nine-features.md).

## Validation

Tests cover each admission branch, randomized comparison against an independent
cosine/Jaro-Winkler formula, exact cached cosine values, and ownership of batch
buffers. The opt-in Docker reference uses full WL scores without filters. It then
applies the candidate rule independently and checks production clone groups.

Docker's exhaustive WL reference contains 32,627 pairs. The old AND route retains
26,680 (81.8%). The corrected alternative-admission route retains all 32,627 in
this corpus, including the two pairs below the numerical threshold and 5,945 pairs
below the name threshold. LSH excludes none here. This is agreement with the fixed
WL rule, not a semantic clone ground truth or a general recall guarantee.

See [matching.md](matching.md) and [matching-results.json](matching-results.json)
for measurements. No changes are committed or pushed.

The 32,627-pair result above is the reference before execution/reference features.
The new execution-edge kernel changes the reference. The fresh Docker corpus has
32,603 unfiltered WL pairs; the nine-feature pipeline retains 32,184 (98.7%).
The remaining 419 pairs fail both alternative admission scores without AST bypass.
LSH drops none on this corpus. We retain the thresholds and report the loss.
