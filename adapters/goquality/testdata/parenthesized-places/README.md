# Parenthesized assignment targets

Two bugs arise when assignment classification checks only bare identifiers:

- `((local)) = shared` is missing from the local pointer's binding history. A
  later `local.Count++` changes caller state, but the analyzer reports complete
  coverage with zero density and no effects.
- `((_)) = 42` discards the value but produces a phantom temporary mutation.

Both fixes unwrap parentheses before recognizing the identifier. Existing
conservative ownership merging applies to the recorded assignment. Owned
allocations stay local; mixed local and external assignments stay unknown.
Discarded targets contribute no mutation events.

| Bug | Original | Tree-sitter result | Checker runs |
| --- | --- | --- | --- |
| Hidden caller mutation | 24 lines / 379 bytes | 14 lines / 151 bytes | 47 |
| Phantom blank mutation | 18 lines / 257 bytes | 8 lines / 68 bytes | 16 |

Runs use Tree-sitter CLI 0.26.9 and an installed Go parser. The affected binary
includes the earlier aliasing, closure-parameter, and suppression-string fixes.

The hidden-mutation case uses `../promoted-pointer/check.py`: it compiles and
runs a Go test proving the caller's count becomes 1, then requires a complete
zero-density report with no effects. The blank case uses `blank-check.py`: it
compiles and runs `Blank`, then requires a temporary mutation attributed to its
discard. Both reject unrelated failures and correctly analyzed candidates.

The blank checker disables vet because Go 1.25's printf vet analysis panics on
parenthesized blank identifiers. Compiler and runtime checks remain enabled.
The repository's own vet checks remain enabled and pass.

From the repository root, with a binary built before these fixes:

```sh
cyclo bug-reducer --language go --go-parser /path/to/go.so --tui=false --timeout 30s --output /tmp/paren-reduced.go adapters/goquality/testdata/parenthesized-places/input.go.txt -- python3 adapters/goquality/testdata/promoted-pointer/check.py /path/to/affected-cyclo
cyclo bug-reducer --language go --go-parser /path/to/go.so --tui=false --timeout 30s --output /tmp/blank-reduced.go adapters/goquality/testdata/parenthesized-places/blank-input.go.txt -- python3 adapters/goquality/testdata/parenthesized-places/blank-check.py /path/to/affected-cyclo
```

Outputs must not exist. Rechecking the saved results returns 0 with the affected
binary and 1 with the fixed binary. `parenthesized_place_test.go` covers the
reduced alias case, pointer and slice assignments, unreachable branches,
multiple assignment, owned storage, and blank targets mixed with real writes.
