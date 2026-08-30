package application

import (
	"errors"
	"io"
)

func WriteSkill(writer io.Writer) error {
	if writer == nil {
		return errors.New("skill writer is required")
	}

	_, err := io.WriteString(writer, agentSkill)
	return err
}

const agentSkill = `---
name: cyclo
description: Inspect Go codebases with the Cyclo TUI to find cyclomatic paths, cognitive load, and control-flow hotspots.
---

# Cyclo

Use Cyclo to locate Go code whose control flow deserves inspection. Treat complexity as a signal, not a score to game.

## Inspect

Run ` + "`cyclo .`" + ` for the current repository or ` + "`cyclo [paths...]`" + ` for selected Go files and directories.

- Files shows cyclomatic aggregates and a purple cognitive peak as space allows.
- Functions shows cyclomatic complexity as ` + "`CC`" + `, purple cognitive complexity as ` + "`COG`" + `, and physical size as ` + "`LINES`" + `.
- Source shows the selected function. Cyclomatic source uses amber text. Cognitive source gets a dark purple background. Shared lines show amber on purple. Line numbers stay neutral. Red connectors enclose guards that return errors. Red return values are errors. Green return values are successful results; a trailing nil error remains neutral.

Inside a Git worktree, Source automatically shows the diff against ` + "`main`" + ` or ` + "`master`" + `. It falls back to their ` + "`origin/*`" + ` refs, then ` + "`HEAD`" + `. The Source title shows the chosen base and change counts. Green ` + "`+`" + ` gutters mark added lines. Red ` + "`−`" + ` rows preserve deleted lines beside the current function. Outside Git, Source stays unchanged.

Use ` + "`tab`" + ` and ` + "`shift+tab`" + ` to change panes, ` + "`j/k`" + ` to move, ` + "`,`" + ` and ` + "`.`" + ` to change files from any pane, ` + "`r`" + ` to refresh, and ` + "`q`" + ` to quit.

Press ` + "`[`" + ` and ` + "`]`" + ` from any pane to visit the previous or next note across the report. Cyclo selects its file and function, then reveals it in Source. Files and Functions mark rows containing notes with amber diamonds.

In Source, ` + "`j/k`" + ` moves a line cursor and keeps it visible. Press ` + "`v`" + ` to start or clear a visual line selection, then ` + "`a`" + ` to attach a note. Use ` + "`d`" + ` to remove the note under the cursor. ` + "`esc`" + ` clears the line selection.

Annotations persist across restarts in SQLite at ` + "`$XDG_STATE_HOME/cyclo/annotations.db`" + `, or ` + "`~/.local/state/cyclo/annotations.db`" + ` when ` + "`XDG_STATE_HOME`" + ` is unset. They are isolated by repository.

## Control with JSON-RPC

Treat a running Cyclo TUI as a shared screen. Preserve unrelated focus and selections, and leave the view useful to the user.

Cyclo exposes its control API on ` + "`http://127.0.0.1:8197`" + ` by default. Probe that endpoint before starting another Cyclo process. A custom instance uses ` + "`cyclo --control-port PORT [paths...]`" + `. Pass ` + "`0`" + ` to choose a free port; Cyclo keeps the active port visible in its header.

Connect in this order:

1. Call ` + "`rpc.discover`" + `. Its runtime OpenRPC document is the authority for methods and parameters.
2. Call ` + "`cyclo.getState`" + `. Record the revision, focus, and selected file and function before changing anything.
3. Inspect first. Mutate the shared view only when it helps the user.

Use any JSON-RPC client. This shell helper is enough:

` + "```bash" + `
CONTROL_URL=http://127.0.0.1:8197
rpc() {
  local method="$1" params="${2-}"
  if [ -z "$params" ]; then params='{}'; fi
  curl -fsS "$CONTROL_URL" \
    -H 'content-type: application/json' \
    --data "$(printf '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"%s\",\"params\":%s}' "$method" "$params")"
}

rpc rpc.discover
rpc cyclo.getState
rpc cyclo.setFocus '{"pane":"functions"}'
` + "```" + `

Treat a JSON-RPC ` + "`error`" + ` envelope as failure even when HTTP returns 200.

` + "`cyclo.getReport`" + ` returns ranked complexity metadata. Each function includes ` + "`complexity`" + `, ` + "`cognitiveComplexity`" + `, ` + "`cyclomaticDiagnostics`" + `, and ` + "`cognitiveDiagnostics`" + `. The diagnostics identify each increment, its nesting cost, and its source position. The selected code lives at ` + "`result.selection.function.source`" + ` from ` + "`cyclo.getState`" + `. Selections use zero-based indexes from the ranked file and function arrays. ` + "`cyclo.setFocus`" + `, ` + "`cyclo.selectFile`" + `, ` + "`cyclo.selectFunction`" + `, and ` + "`cyclo.scrollSource`" + ` return the updated state. ` + "`cyclo.refresh`" + ` waits for analysis to finish before it replies.

Source review state is shared too. ` + "`cyclo.revealLines`" + ` focuses and highlights a range, while ` + "`cyclo.clearLineSelection`" + ` clears it. ` + "`cyclo.annotateLines`" + ` adds a note without changing the visible selection; ` + "`cyclo.removeAnnotation`" + ` deletes one by ID. Line parameters accept displayed source numbers or one-based lines relative to the selected function. Read ` + "`cursor`" + `, ` + "`lineSelection`" + `, ` + "`annotations`" + `, and ` + "`activeAnnotationId`" + ` from ` + "`cyclo.getState`" + `. Use the runtime OpenRPC document for exact result shapes.

Use ` + "`cyclo.waitForChange`" + ` instead of polling. Pass the last ` + "`revision`" + ` as ` + "`afterRevision`" + ` and an optional ` + "`timeoutMs`" + ` from 0 to 60000. It returns when the TUI advances or the timeout expires; compare the returned revision to tell which happened.

For inspection-only work, report the evidence without changing the TUI. Restore temporary focus or selection changes when they no longer help the user.

## Pull requests

Scan only tracked Go files changed from ` + "`main`" + `:

` + "`git diff --name-only -z --diff-filter=ACMR main -- '*.go' | xargs -0 -r -o cyclo`" + `

The ` + "`-o`" + ` flag reconnects Cyclo to the terminal after ` + "`xargs`" + ` reads the pipe. Replace ` + "`main`" + ` with the pull request's base branch. This includes branch commits plus tracked staged and unstaged changes. Git omits untracked files, so pass those paths to Cyclo explicitly when they matter.

## Simplify

Use ` + "`CC`" + ` to find path-heavy functions and ` + "`COG`" + ` to find code that is hard to follow. A wide gap between them is useful evidence. Prefer guard clauses, early returns, and flatter control flow. Do not extract helpers solely to lower a metric or trade readable code for a smaller number.

After editing, run the relevant tests and Cyclo again. Report what became easier to follow, not only how the score changed.
`
