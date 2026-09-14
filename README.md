# cyclo

See cyclomatic paths and cognitive load in Go code in a TUI or hand it to an agent

## Install

Install `cyclo` globally with Go:

```sh
go install github.com/shanejonas/cyclo@latest
```

## Run

Scan the current directory:

```sh
cyclo .
```

Pass one or more files or directories to scan them instead:

```sh
cyclo ./domain ./adapters
```

The treemap combines both complexity scores: tile area shows cyclomatic complexity,
and color shows cognitive complexity from green to red. Files group their function
tiles. Click a tile to inspect its source and both scores. The map appears in
terminals at least 100 columns wide and 30 rows tall.

Inside Git, Cyclo automatically shows added and deleted source lines against `main` or `master`.

Cyclo starts a localhost JSON-RPC control API on port `8197`. Pick another port with `--control-port`:

```sh
cyclo --control-port 9000 .
```

Ask the running app for its OpenRPC document:

```sh
curl -s http://127.0.0.1:9000 \
  -H 'content-type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"rpc.discover"}'
```

The control API reads and changes the live TUI model. `cyclo.getReport` returns cyclomatic scores, cognitive scores, and the source locations that contribute to cognitive load. `cyclo.getState` includes the selected function's source. Agents can focus panes, select files and functions, scroll source, refresh the analysis, and wait for a new revision without polling. See [application/openrpc.json](application/openrpc.json) for the full contract.

Print the operating skill for coding agents:

```sh
cyclo --skill
```

## Build

```sh
go build -o cyclo .
./cyclo
```

## Reduce a bug reproducer

`bug-reducer` works with inputs for any codebase. Supply a command that confirms
one specific bug; Cyclo does not parse the input or decide what counts as a bug.

```sh
cyclo bug-reducer failing-input -- ./check-bug.sh
# Run directly from this repository:
go run . bug-reducer failing-input -- ./check-bug.sh
```

The checker receives an absolute candidate file path as its **last argument**,
following any arguments you supply. It runs in the directory you launched Cyclo
from, inherits your environment, and receives no stdin. Output is suppressed.
Exit **0** means the same bug remains; an ordinary nonzero exit rejects the
candidate. The checker must reject unrelated syntax errors or other failures.
For project-dependent tests, the checker arranges its own build/test workspace
or overlay using that candidate path. Candidates are temporary single files,
not copies of the entire project.

The command saves to `failing-input.reduced`, preserves the original, and refuses
to overwrite an existing output. Flags go before the input:

```sh
cyclo bug-reducer --timeout 30s --output smaller.json failing.json -- ./check-bug.sh --strict
cyclo bug-reducer --help
```

Input must be a regular file; FIFOs, devices, and directories are rejected before
reading. Symlinks to regular files are supported.

The timeout applies to each checker invocation. Timeout, Ctrl-C, Unix SIGTERM, checker signal,
or launch failure stops reduction; once the seed is accepted, the best accepted
candidate is saved before the command reports the error. If the seed is rejected,
no output file remains. On Unix, cancellation and normal checker completion clean up its process group,
including ordinary child processes; descendants that detach into another group are outside that
scope. Other platforms currently cancel only the direct checker process. Checkers
should wait for their own subprocesses and avoid leaving background jobs running.

Reduction currently deletes chunks of whole lines until no single remaining line
can be removed while preserving the check. It is format-independent, but cannot
simplify within a line and does not promise a global minimum. Use deterministic
checks. A permissive check can accept an empty file.

For a quick non-Go smoke test (the marker stands in for a bug):

```sh
printf 'noise\nspecific failure\nmore noise\n' > /tmp/reducer-demo.txt
cyclo bug-reducer /tmp/reducer-demo.txt -- grep -Fq -- 'specific failure'
cat /tmp/reducer-demo.txt.reduced
# specific failure
```

Finding bugs still needs tests, fuzzing, or inspection. For Cyclo, a checker can
compare the analyzer's reports for equivalent Go forms, or assert a known wrong
score, source location, or panic. Keep each check focused on the specific failure;
use the reduced input as the regression test after fixing it.

To check a specific Go input for initializer inconsistencies and reduce failures:

```sh
go run ./examples/cyclo-hunt path/to/input.go
```

See [the Cyclo checker guide](examples/cyclo-hunt/README.md) for its supported inputs
and checks. The exploratory fixture generators have been removed.
