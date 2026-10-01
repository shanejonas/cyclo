# Closure parameter provenance

The closure mutates caller-owned state. Before the fix, assigning an allocation
to its pointer parameter in an unreachable branch makes the analyzer classify
`value.Count++` as local. Nested function parameters now retain parameter
ownership; assigning the variable itself remains a local write.

The checker compiles and runs a Go test proving the caller's count becomes 1,
then requires the incorrect local provenance in exported facts. It rejects
syntax, typing, runtime, and analysis failures and a correct report.

| Reduction | Original | Reduced | Checker runs |
| --- | --- | --- | --- |
| Lines | 25 lines / 540 bytes | 9 lines / 201 bytes | 117 |
| Tree-sitter Go units | 25 lines / 540 bytes | 13 lines / 205 bytes | 64 |

The Tree-sitter result preserves blank separators; the units remove code rather
than format it. Both saved results reproduce the same bug in the affected binary.
The structural run uses Tree-sitter CLI 0.26.9 with an installed Go parser.

From the repository root, with a binary built before the closure parameter fix:

```sh
cyclo bug-reducer --language go --go-parser /path/to/go.so --tui=false --timeout 30s --output /tmp/closure-reduced.go input.go -- python3 adapters/goquality/testdata/closure-parameters/check.py /path/to/affected-cyclo
```

Use `adapters/goquality/testdata/closure-parameters/input.go.txt` as `input.go`.
The output must not exist. Recheck either saved result with the same checker.
The affected binary exits 0; the fixed binary exits 1 because the parameter-field
mutation is now external. `closure_parameter_test.go` covers the reduced input,
nested closures, pointer/slice parameters, value parameters, and owned locals.
