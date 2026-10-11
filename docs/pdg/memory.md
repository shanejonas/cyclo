# PDG memory measurements

Historical measurements from earlier worktree snapshots. Production now removes LSH and adds explicit return-to-output links. See [return-link-timing.md](return-link-timing.md) for the final return-link comparison.

Measured on Linux amd64 with Go 1.27.1. Values are medians of three fresh
processes per mode. Both binaries analyze the same migrated working-tree source:
2,231 functions. The old builder emits 42,742 nodes; the canonical IR emits
49,990 nodes, including explicit definitions and outputs. Both emit 58,833 edges.

| Scope and metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Graph construction: retained heap (MiB) | 10.54 | 14.00 | +32.9% |
| Graph construction: allocation volume (MiB) | 20.90 | 98.23 | +370.0% |
| Graph construction: allocations | 85,206 | 732,090 | +759.2% |
| Graph construction: elapsed time (ms) | 36.63 | 141.92 | +287.4% |
| Graph construction: peak process RSS (MiB) | 87.04 | 112.28 | +29.0% |
| Full Extract API: retained heap (MiB) | 15.46 | 18.14 | +17.3% |
| Full Extract API: allocation volume (MiB) | 120.67 | 204.92 | +69.8% |
| Full Extract API: allocations | 1,177,247 | 1,824,582 | +55.0% |
| Full Extract API: elapsed time (ms) | 337.49 | 451.06 | +33.7% |
| Full Extract API: peak process RSS (MiB) | 94.92 | 116.47 | +22.7% |

The native graph retains about 3.46 MiB more, or 33%. The full extraction result
retains about 2.68 MiB more, or 17%. These scopes have different starting heaps
and cannot be subtracted to infer the cost of individual metadata fields.

The cost is not zero: graph construction allocates about 4.7 times as many bytes.
The full API allocates about 1.7 times as many bytes and has higher peak RSS.
Interning limits retained duplicates; it does not eliminate construction work.
These results cover extraction, not a full mining run or a larger project.
The matcher derives a temporary view, so mining has an additional transient cost.

Before the migration, the measurement test binary was saved locally as
`.tmp-build/pdg-memory-before`. It uses the legacy Extract and builder from the
`21f54c2` base. It does not call the separate experimental IR extractor.
The after binary uses the sole production `buildGraph` path. No legacy graph is
stored in the new extraction result.

Graph mode loads typed packages before the starting GC and keeps them alive
through the measured GC. It excludes package loading and quality findings from
construction deltas. API mode measures the complete Extract call, including
loading and existing findings. Retained heap is the post-GC change in HeapAlloc.
Allocation volume is the change in TotalAlloc; it is not resident memory.
Peak RSS comes from getrusage and includes the process and compiler loader.
Timings and RSS vary with the host; these are local measurements, not guarantees.

## Repeat locally

The opt-in Linux harness is `adapters/gopatterns/ir_memory_measure_linux_test.go`.
Build the after binary, then run each mode three times in separate processes:

```sh
go test -buildvcs=false -c -o .tmp-build/pdg-memory-after ./adapters/gopatterns
CYCLO_PDG_MEMORY_MODE=graph-new CYCLO_PDG_MEMORY_ROOT="$PWD"   .tmp-build/pdg-memory-after -test.run '^TestMeasurePDGMemory$' -test.v
CYCLO_PDG_MEMORY_MODE=api-new CYCLO_PDG_MEMORY_ROOT="$PWD"   .tmp-build/pdg-memory-after -test.run '^TestMeasurePDGMemory$' -test.v
```

For the preserved before binary, use `graph-old` and `api-old`. Both binaries
must use the same source corpus. The before binary is a local measurement
artifact, not a checked-in parallel production path. Set TMPDIR and GOTMPDIR to
a writable build directory if needed. Raw results are in
[memory-results.json](memory-results.json).

Full-corpus IR validation runs outside the timed measurement:

```sh
CYCLO_PDG_VALIDATE_ROOT="$PWD" go test ./adapters/gopatterns   -run '^TestValidatePDGCorpus$' -v
```
