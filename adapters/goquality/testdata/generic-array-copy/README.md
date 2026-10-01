# Generic array copy falsely classified as external

`Copy[T ~[1]int]` mutates an array value copied from its argument. The caller's
array stays unchanged, but the affected analyzer emits an external mutation
effect because it recognizes concrete arrays and misses array-constrained type
parameters. Indexing and slicing now recognize constraints restricted to value
arrays. Mixed array/slice or pointer constraints keep reference ownership.

Tree-sitter reduction shrinks the original 320-byte input to 111 bytes in 26
checker runs. The checker compiles and executes a Go test requiring both the
changed copy and unchanged caller, then requires the incorrect mutation effect.
It rejects unrelated compilation, runtime, and analysis failures.

From the repository root, with a binary built before this fix:

```sh
cyclo bug-reducer --language go --go-parser /path/to/go.so --tui=false --timeout 30s --output /tmp/generic-array-reduced.go adapters/goquality/testdata/generic-array-copy/input.go.txt -- python3 adapters/goquality/testdata/generic-array-copy/check.py /path/to/affected-cyclo
```

The output must not exist. The reduced input's checker exits 0 with the affected
binary and 1 with the fixed binary. `generic_array_test.go` covers direct array
parameters, copies, slices of copies, array-only unions, named and intersected
constraints, and pointer/slice/mixed constraints. The generated harness adds
six syntax variants for each new generic-array family.
