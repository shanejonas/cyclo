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
