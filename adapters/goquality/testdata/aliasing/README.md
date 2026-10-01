# Aliasing density false negatives

Each reduced input changes caller-owned state at runtime, while the pre-fix
quality analyzer claims complete coverage with zero density and no effects.
The pre-fix binary includes the first promoted-pointer fix.

| Case | Original | Reduced | Checks |
| --- | --- | --- | --- |
| Indirect pointer-variable replacement | 27 lines / 533 bytes | 8 lines / 161 bytes | 135 |
| Range assignment to an existing pointer | 26 lines / 536 bytes | 7 lines / 164 bytes | 117 |
| Allocated wrapper with an embedded pointer | 26 lines / 528 bytes | 7 lines / 156 bytes | 117 |

The fixtures retain the exact reducer input and output. Each reduction uses the
shared runtime-and-report checker in `../promoted-pointer/check.py`. The checker
requires a valid Go module, a runtime test proving `Mutate` changes the caller's
count to 1, and a quality report falsely claiming complete coverage with zero
effects and density. It rejects unrelated syntax, typing, runtime, or analysis
failures, as well as a correct report.

From the repository root, using a binary built before these aliasing fixes:

```sh
python3 adapters/goquality/testdata/promoted-pointer/check.py /path/to/pre-fix-cyclo adapters/goquality/testdata/aliasing/indirect-reduced.go.txt
cyclo bug-reducer --tui=false --timeout 30s --output /tmp/indirect-reduced.go.txt adapters/goquality/testdata/aliasing/indirect-input.go.txt -- python3 adapters/goquality/testdata/promoted-pointer/check.py /path/to/pre-fix-cyclo
```

The output path must not already exist. Replace `indirect` with `range` or
`allocated-wrapper` for the other cases. The fixed binary makes the checker exit
1: the runtime mutation still occurs, but the report now records an unknown
mutation effect, nonzero density, and incomplete coverage.

The fixes preserve uncertainty for bindings populated by range or exposed through
a variable's address, record writes performed by range assignment, and inspect
every promoted-field step for embedded pointer indirection. An address-exposed
reference is conservative even if a particular use does not replace it; no
interprocedural ownership proof is claimed. Ordinary owned pointers, embedded
values, local scalar addresses, and array value copies retain local provenance.

`aliasing_test.go` uses all three reduced fixtures to verify the quality evidence
attached to TUI functions and checks related aliasing and owned-storage cases.
