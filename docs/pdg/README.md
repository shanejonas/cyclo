# Compact PDG IR

`domain/pdg` is cyclo's canonical PDG. It covers the fields in compat's draft
0.1.0 at commit `e1d08c2500e4184444fed70c9d9d1df206ac6962` (PR #8).
`compat-0.1.0.schema.json` pins that contract. Optional JSON export expands
the compact records into this schema; the normal analysis path keeps compact storage.

## JSON export

```sh
cyclo patterns --format pdg-json . > pdg.json
```

The output is a JSON array. Each element is one function document conforming to
the pinned compat draft's structural schema. Export includes source spans,
interfaces, symbols, definitions, memory locations, nodes, typed edges,
provenance, capability evidence, diagnostics, conformance records and extensions.
It resolves table indexes to document IDs and text. Unknown facts retain their
reasons; absent optional facts stay absent. Empty required collections are arrays.

Export bypasses pattern mining, normalization, WL matching and its cache. The
matching threshold does not affect export. `--cache` cannot be used with this
format. Existing `--format json` still writes the pattern report.

Native graphs are validated before output starts. Expanded JSON records are
allocated for one function at a time; no extraction-wide expanded graph set is
retained. Extraction still retains all compact function graphs. JSON repeats
shared evidence as the schema requires, so the output file can be larger than the
native IR. A write error or cancellation can leave partial output; discard it
when the command fails. Structural validity does not establish semantic parity
with Rust or CCGraph profile conformance.

Validate every element with a Draft 2020-12 validator. The helper below uses the
Python `jsonschema` package as a separate development tool, not a Cyclo dependency:

```sh
python3 docs/pdg/validate-export.py pdg.json
```

## Storage

- `Text` is a 32-bit reference to an extraction-wide string table. Names, full
  types, facts, matching labels, original kinds and unknown reasons share storage.
- `Ref` is a one-based 32-bit index; zero means absent. Known empty strings have
  a nonzero Text ID. IDs identify graph items; they are not matching labels.
- Policies, capability evidence and common provenance are shared across functions.
  Construction maps and compiler objects are released after extraction.
- Nodes are 44 bytes and edges are 56 bytes on the measured 64-bit system.
  Nodes include an interned matching-label reference and a display line number.
  Source spans use separate 12-byte records.
- Sparse attributes are packed into shared records and lists within each graph.
  Empty attributes use index zero. Scalar reads are facts on their owning
  operations. Each read does not require a separate node.
- `Compact` removes spare construction capacity when it exceeds one eighth of
  the retained length. It preserves indexes, values and graph order.
- Optional positions use position+1; zero means unrecorded. LoopCarried encodes
  unrecorded/false/true as 0/1/2.
- Namespaced extensions retain their JSON values as interned text. Extraction
  does not marshal or retain a second JSON graph.
- Shared tables and attribute lists are immutable by convention. Builders have
  one owner and do not support concurrent writes.

## Extraction and matching

`gopatterns.Extract` returns `FuncPdg.Pdg` as `pdg.Graph`. There is one production
construction path, `buildGraph`. There is no separate `ExtractIR` entrypoint.
The returned IR contains no AST, types.Info or types.Object references.

`patterns.FuncFacts` carries this graph. `patterns.Run` derives a temporary
`MiningGraph` view for the matcher. The view resolves the interned cyclo labels,
remaps node indexes, preserves parallel edges and excludes nodes with no matching
label. It includes data, lexical control and typed execution edges. CCGraph uses
execution edges in WL; the abstraction kernel keeps its existing data/control
profile. Both derive from the same native IR. No second graph is retained in it.
Normalization, WL and alignment work on this temporary view.

The `cyclo.abstraction` profile preserves existing first-binding mining behavior.
Its interned MatchLabels are separate from base IR categories, identity and
provenance. Effects remain available for safety checks and do not enter WL labels.
Synthetic abstraction operations carry desugaring provenance.

The producer records source byte spans, full function interfaces and types,
bindings, distinct syntactic definitions, read/write facts and call targets.
`cyclo.go-return-values/1` connects each explicit return operation to every
formal output slot, with result positions recorded on the edges. This covers
multiple explicit values and a tuple-valued call. Bare returns record reads of
named result bindings. Returns inside nested closures do not connect to the
outer function. These edges are approximate: reaching definitions, deferred
result writes, and implicit or exceptional exits remain incomplete. Output nodes
remain outside the WL mining view; canonical data-edge counts include these new
return links.

Unknown memory targets stay unknown; the producer does not invent shared locations.
First-binding edges are marked as approximate mining dependencies. They carry
no reaching-definition references and must not be used as precise def-use facts.
Lexical control edges are also marked approximate.

This provides the draft representation fields, with explicit analysis limits; it
does not establish shared-profile conformance. Merge values,
loop-carried definitions, precise reaching definitions, post-dominator control,
closure captures, alias targets and exceptional/concurrent dependencies remain
unsupported or unknown. Execution edges now form an approximate must-precede basis
from statement order and dominance. CFG successor links are analysis inputs, not
exported execution dependencies. Reference counts use distinct typed Go bindings;
generic reference kinds remain unknown. All required capabilities are present;
graphs carry non-conformance evidence and an incomplete-dependency diagnostic.

## Validation and measurement

`pdg.Validate` checks facts, metadata, capabilities, IDs, references, definition
ownership, span ordering, namespaced extensions and configuration shape. It
preserves parallel edges and self edges. It does not prove source semantics,
source-byte bounds without source content, or profile conformance. Validate IR
from other producers before passing it to the matcher.

Tests cover canonical extraction, shared storage, matching-view remapping,
unchanged matching labels, indirect calls, unknown memory, definitions and
compaction. Run `make check`, `make cognitive` and `make quality-gate`.

See [memory.md](memory.md) for measured costs and [memory-results.json](memory-results.json)
for raw runs. Construction allocations and peak RSS remain higher than before;
retained graph storage is bounded and does not duplicate a stored legacy graph.

## Review notes

Graph construction updates several columns and related attribute records.
Quality checks report some of these as mutation or aggregate findings. These
updates construct one graph before publication. They remain visible without
suppression. Edge identity is assigned in the constructor.

Make targets accept `QUALITY_TMP_DIR` (default `/tmp`). This permits checks when
the host's `/tmp` quota is exhausted.

See [matching.md](matching.md) for shared matching features, Docker timing and
an exhaustive WL reference that measures the current candidate filters' recall.

See [paper-audit.md](paper-audit.md) for the corrected candidate-admission rule,
source ambiguity and remaining differences from CCGraph.

The CCGraph numerical vector uses nine source features from canonical categories,
including execution edges and reference bindings. It excludes entry/exit and output
metadata nodes. It distinguishes assignment categories from arithmetic computations.
Features are computed once per temporary matching view and survive normalization.
Unknown generic reference counts cannot reject candidates and do not become
known zeroes. WL cache format version 3 invalidates older matching computations.
See [nine-features.md](nine-features.md) for policy details and measured costs.

See [exhaustive-comparison.md](exhaustive-comparison.md) for the five-run
comparison of filtered and exhaustive routing with the current IR and WL kernel.

See [compat-audit.md](compat-audit.md) for the current upstream issues, PR snapshot,
local readiness, and the remaining shared-profile work.

See [ccgraph-profile.md](ccgraph-profile.md) for explicit pair evidence routing,
preserved report defaults, and the unavailable compat base profile.

See [performance-profile.md](performance-profile.md) for graph, WL, routing, CPU
and allocation measurements on Cobra, Gin and Prometheus.

See [return-link-timing.md](return-link-timing.md) for the final producer/export
comparison with explicit return-to-output links.
