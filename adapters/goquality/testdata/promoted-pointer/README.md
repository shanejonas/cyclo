# Promoted-pointer quality false negative

Cyclo at commit `4fa9285` classifies a promoted-field mutation through a local
value wrapper as local, even when the embedded pointer reaches caller-owned
state. The report incorrectly says `complete: true`, `density_milli: 0`, and
`effects: []`.

The global bug reducer reduces `input.go.txt` from 25 lines / 541 bytes to
`reduced.go.txt`, 6 lines / 134 bytes, in 111 checks. The fixture files retain
exact reducer input and output rather than formatted Go source.

`check.py` receives a Cyclo binary and candidate path. It creates an isolated Go
module, runs a test proving `Mutate` changes the caller's count to 1, then requires
the typed quality report to incorrectly claim complete coverage with no effects
and zero density. It exits 0 only for that same bug; syntax errors, type errors,
runtime failures, analysis failures, and a correctly reported effect are rejected.

With a binary built from the affected commit:

```sh
python3 adapters/goquality/testdata/promoted-pointer/check.py /path/to/affected-cyclo adapters/goquality/testdata/promoted-pointer/reduced.go.txt
cyclo bug-reducer --tui=false --timeout 30s --output /tmp/promoted-pointer-reduced.go.txt adapters/goquality/testdata/promoted-pointer/input.go.txt -- python3 adapters/goquality/testdata/promoted-pointer/check.py /path/to/affected-cyclo
```

The output path must not already exist. With the fixed binary, the same checker
exits 1: the runtime mutation still occurs, but quality now records an unknown
mutation effect, nonzero density, and incomplete coverage. Embedded pointer
fields may alias shared state, so the analyzer does not claim ownership without
proof. Ordinary embedded value copies remain local.

`promoted_pointer_test.go` uses the reduced fixture to verify the quality report
attached to the TUI's function, and covers nested promotion, promoted-field
addresses, explicit pointer fields, type aliases, value copies, and pointer
parameters.
