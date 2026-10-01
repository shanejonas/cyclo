# String literals mistaken for suppression comments

The input has a valid string constant containing a documentation example of
`cyclo-allow`, immediately before `Plain`. The previous raw-text scan treats it
as suppression metadata and emits an invalid suppression diagnostic. Suppression
metadata now comes only from actual comment groups attached to the declaration.

Line reduction shrinks 17 lines / 393 bytes to 3 lines / 111 bytes in 47 checker
runs. The checker compiles and runs a test of `Plain`, requires the mistaken
constant metadata, and requires the invalid suppression diagnostic. It rejects
unrelated failures and correctly analyzed candidates.

From the repository root, with a binary built before this fix:

```sh
python3 adapters/goquality/testdata/suppression-strings/check.py /path/to/affected-cyclo adapters/goquality/testdata/suppression-strings/reduced.go.txt
```

The affected binary exits 0; the fixed binary exits 1. Regression coverage in
`suppression_comment_test.go` includes the reduced input, raw multiline strings,
real directives with documentation, malformed directives, blank-separated
comments, and comments trailing unrelated declarations.
