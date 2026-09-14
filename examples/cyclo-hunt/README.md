# Check Cyclo function initializers

```sh
go run ./examples/cyclo-hunt path/to/input.go
```

Supply a self-contained Go input with package-level function initializers. This
checker compares each initializer with an equivalent named declaration. It checks
function count, names, scores, physical source ranges, source text, and diagnostic
totals. It matches by name, regardless of report ordering.

On a mismatch it reduces the input, keeping the same failed rule and function
name where applicable. It saves `seed.go`, `reduced.go`, and `failure.txt` in a new
`.cyclo-hunt-*` directory and exits nonzero. Original files remain untouched.

This is a focused checker, not a general Go project analyzer: its input must
type-check without imports, its function bodies must work as named declarations,
and ignored functions and mixed named declarations are outside its intended input
shape. It cannot detect mistakes shared by both analyzer paths or capture panics.
The broad exploratory generators were removed after their cases passed. Tests of
the checker and reducer remain, alongside actual analyzer regression tests.
