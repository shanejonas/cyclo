# Clone profile boundary

The report default remains Cyclo's existing extension route: characteristic/name admission, AST bypass, verification of every admitted pair with standard WL and connected groups. It has no source-line minimum. LSH retrieval is removed. The compact-IR migration does not claim to preserve every historical result.

`EvaluateClonePairs` requires explicit activation of `cyclo.ccgraph-pairs/1`. It is a bounded, executable Cyclo adaptation, not compat conformance or original CCGraph reproduction. It consumes prepared mining graphs. Native IR extraction and normalization remain caller-owned stages.

## Executable policy: cyclo.ccgraph-pairs/1

| Stage | Version 1 rule |
| --- | --- |
| IDs | Usable, nonempty graphs sorted by function ID; distinct unordered pairs, no self pairs |
| Required facts | Known nine-feature vector, including known reference count; known function name. Missing required facts stop before any evidence is emitted |
| Counts | Current native category policy; order: declarations, assignments, controls, calls, other, control edges, data edges, execution edges, reference bindings |
| Numerical score | Cosine; zero norm scores zero |
| Name score | Existing Jaro–Winkler on short names; known empty/empty scores one, empty/nonempty zero |
| Admission | Numerical score >= 0.9 OR name score >= 0.5 |
| Size | No source-line or graph-scale gate; this is an explicit adaptation choice |
| WL | Existing directed, typed-edge labels, execution edges included; at most four histogram levels; current depth convention and decreasing weights |
| Verification | Weighted histogram intersection / weighted larger graph node count; integer thousandths; accept >= 900 |
| Evidence | One sorted record per unordered pair: numerical/name scores, admission, whether WL was computed, WL score, acceptance and profile ID |
| Output | Streaming callback; accepted records are clone pairs. No transitive grouping, AST bypass, LSH, neighborhood kernel or ranking |
| Errors | Unknown profile, missing sink or required facts return an error. Sink failure stops the stream and preserves the wrapped error |

The API retains one light WL summary per usable graph, not a pair list or candidate matrix. Callback retention is controlled by the caller. Evaluation remains quadratic in pair count; this change has no new performance claim.

Fixtures in `domain/patterns/ccgraph_profile_test.go` specify identical-graph scores, numerical admission despite unrelated names, orthogonal-vector/name rejection, required-unknown rejection, explicit activation, sink errors and report-default preservation. Native feature and execution-edge fixtures remain in `paper_characteristics_test.go`. These are local expectations, not shared Rust results.

## Reserved profile: compat.ccgraph-base/draft

This profile cannot execute. Requesting it returns an explicit error. These choices need a shared specification and independently defined stage fixtures before a conformant implementation can be enabled:

- Native-to-base projection, node taxonomy, auxiliary removal and call collapse, with provenance.
- Reference-variable meaning; source-line/node-size gate and graph-scale threshold.
- Input/output parameter strings: roles, ordering, names/types, separators, escaping, variadic state and aggregation with Name.
- Numerical/string gate relationship and strict versus inclusive thresholds.
- Initial labels, directed neighborhood encoding, multiplicity, synchronous rounds, diameter/depth and iteration weights.
- Preliminary kernel/normalization versus final verification, including the paper's conflicting denominators.

Do not copy the adaptation's choices into this reserved profile silently. Do not advertise a conformant run with required unknown interface or reference facts. JSON export and an engine architecture decision are not prerequisites for these local boundaries.

## Next shared fixture contract

Each fixture needs its profile ID, native input facts, projected nodes/edges with correspondence, nine named counts, three serialized strings, each gate decision, per-round label equivalence partitions and histograms, depth/weights, preliminary/final denominators and scores, and sorted accepted pairs. Compare partitions rather than numeric hash values. Include exact-threshold, parallel-edge, cyclic, empty-interface and required-unknown cases. Record Go and Rust observations separately from expected values.
