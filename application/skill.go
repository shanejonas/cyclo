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
description: Inspect Go complexity, check mutation and effect guardrails, and minimize bug reproducers.
---

# Cyclo

Use Cyclo to locate Go code whose control flow deserves inspection. Treat complexity as a signal, not a score to game.

## Inspect

Run ` + "`cyclo .`" + ` for the current repository or ` + "`cyclo [paths...]`" + ` for selected Go files and directories.

The treemap combines both scores: area shows cyclomatic complexity, and color shows cognitive complexity from green to red. Files contain function tiles. Click a tile to select its source; the white outline marks the selected function. The map needs at least 100 columns and 30 rows.

- Files shows cyclomatic aggregates and a purple cognitive peak as space allows.
- Functions shows cyclomatic complexity as ` + "`CC`" + `, purple cognitive complexity as ` + "`COG`" + `, and physical size as ` + "`LINES`" + `.
- Source shows the selected function. Cyclomatic source uses amber text. Cognitive source gets a dark purple background. Shared lines show amber on purple. Line numbers stay neutral. Red connectors enclose guards that return errors. Red return values are errors. Green return values are successful results; a trailing nil error remains neutral.

The header shows ` + "`Q`" + ` with the quality finding count. Press ` + "`e`" + ` to switch the details pane between source and quality evidence, then j/k to scroll. Quality shows density, unknown coverage, rule findings, effects, and mutation events with source lines. Switching back preserves the source cursor. Use ` + "`cyclo --config PATH [paths...]`" + ` to apply the same TOML policy as the headless check. The TUI includes tests. Typed failures stay visible as quality failed; complexity inspection still works.

Inside a Git worktree, Source automatically shows the diff against ` + "`main`" + ` or ` + "`master`" + `. It falls back to their ` + "`origin/*`" + ` refs, then ` + "`HEAD`" + `. The Source title shows the chosen base and change counts. Green ` + "`+`" + ` gutters mark added lines. Red ` + "`−`" + ` rows preserve deleted lines beside the current function. Outside Git, Source stays unchanged.

Use ` + "`tab`" + ` and ` + "`shift+tab`" + ` to change panes, ` + "`j/k`" + ` to move, ` + "`,`" + ` and ` + "`.`" + ` to change files from any pane, ` + "`r`" + ` to refresh, and ` + "`q`" + ` to quit.

Press ` + "`[`" + ` and ` + "`]`" + ` from any pane to visit the previous or next note across the report. Navigation includes all saved notes and wraps at either end. The footer shows your position. Matched notes reveal their current source. Unmatched notes show their saved text in Source; ` + "`j/k`" + ` scrolls it and ` + "`d`" + ` removes the note. Files and Functions mark rows containing notes with amber diamonds.

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

Quality is shared too. ` + "`cyclo.getReport`" + ` includes ` + "`quality.status`" + ` (ready or error) and, on success, ` + "`quality.report`" + ` with its summary, functions, and diagnostics. Function entries and ` + "`cyclo.getState.selection.function.quality`" + ` include density_milli, complete, unclassified_calls, effects, mutation_events, and diagnostics. Nested quality fields retain the headless report's snake_case names. Missing function quality means unavailable; it is not a clean result.

Use ` + "`cyclo.setDetailsView`" + ` with ` + "`{\"view\":\"quality\"}`" + ` or ` + "`{\"view\":\"source\"}`" + ` to change the shared details pane. State includes detailsView and qualityOffset. ` + "`cyclo.revealLines`" + ` and ` + "`cyclo.scrollSource`" + ` return to source view. ` + "`cyclo.refresh`" + ` reruns complexity and typed quality analysis together.

## Fix patterns automatically

Run ` + "`cyclo patterns [paths...]`" + ` to find mechanical code patterns across 18 kinds: guard clauses, value objects, parameterize candidates, anemic models, primitive obsession, type switches, enum dispatch, trait methods, capability sets, generic functions, entity identity, missing identity, factories, specifications, and domain services (detection-only). These are structural — the fix is a deterministic AST transform.

Run ` + "`cyclo fix --kind all [paths...]`" + ` to auto-fix them. Dry-run by default (shows a diff); add ` + "`--apply`" + ` to write. No LLM, no tokens — pure static analysis. Use ` + "`--kind guard_clause|value_object|parameterize|anemic_model|primitive_obsession|type_switch|enum_dispatch|trait_method|capability_set|generic_fn|entity_identity|missing_identity|factory|specification|domain_service`" + ` to fix one kind.

Use ` + "`cyclo fix --phased --apply [paths...]`" + ` when fixes interact: it mines, applies one phase, re-mines, and repeats to a fixpoint (max 3 cycles). Phases run guard/value-object work first, structural patterns next, and parameterize last.

` + "`domain_service`" + ` is detection-only: it identifies stateless functions operating on 2+ domain types as legitimate Domain Services (Evans), and suppresses the corresponding ` + "`anemic_model`" + ` suggestions. ` + "`specification`" + ` extracts repeated boolean business rules into named predicates (a method on the type, or a function for external types). ` + "`trait_method`" + ` proposes interfaces from parallel methods on types in the same package.

**Workflow:** Run ` + "`cyclo patterns`" + ` first, then ` + "`cyclo fix --apply`" + ` to clear the mechanical issues. This saves tokens — don't hand-rewrite what the fixer handles. In an agent loop, use ` + "`cyclo fix --changed --apply`" + ` to fix only candidates in functions your diff touched (against ` + "`--base REF`" + `, defaulting to the merge-base with main/master).

**One at a time:** ` + "`cyclo next [paths...]`" + ` shows the single highest-impact candidate with fix instructions — the one thing to fix now. ` + "`cyclo score [paths...]`" + ` gives a 0-100 north-star (100 = clean, weighted by severity). Suppressed items show separately, so you can't suppress your way to 100.

## Check quality guardrails (needs your judgment)

Run ` + "`cyclo check --format json [paths...]`" + ` for typed Go mutation and side-effect diagnostics without a TUI. Directories scan packages recursively; Go file arguments report only those files after loading their enclosing packages. Run from the repository root. Use ` + "`--config PATH`" + ` for TOML policy, ` + "`--tests`" + ` to include tests, and ` + "`--tags TAGS`" + ` for build tags. Use ` + "`--changed`" + ` to report only findings in functions the git diff touches (against ` + "`--base REF`" + `, defaulting to the merge-base with main/master), so an agent loop can gate on what its own edits introduced; untracked files count as fully changed.

The rules are fn_length, fn_params, mutation_per_target, mutated_targets, side_effect_density, invalid_suppression, and the DDD rules aggregate, repository, and mutable_identity. Findings carry actual, limit, rule_id, and source location. Density findings include effect kinds, labels, and lines; the message shows the finding's arithmetic (weight over statements with per-kind weights), and the diagnostic carries weight, statements, and kind_weights fields. The Go standard library is fully classified, so unknown now means a call cyclo cannot see into — third-party code or dynamic dispatch — rather than an ordinary stdlib call. Review that evidence before changing code. Preserve legitimate IO boundaries and do not extract helpers solely to lower a score.

Static calls to named helpers in the same package use conservative body summaries. A helper with no modeled effects is effect-free; effectful helpers contribute evidence at the caller line. Recursive cycles, dynamic dispatch, missing bodies, and summary limits remain unknown. Prefix policy overrides still apply, including when reevaluating saved facts. Summaries do not remap parameter writes to caller-owned arguments. Intrinsic effects are syntactic; nested closure bodies contribute even if not called. Pointer-receiver calls are not automatically mutations. Zero density is not proof of purity.

Facts export uses schema_version 2 with helper summaries; version 1 remains readable and keeps absent helper information unknown. Save versioned facts with ` + "`cyclo check --format facts .`" + `, then use ` + "`cyclo check --facts-in PATH --config POLICY --format json`" + ` to reevaluate without loading Go packages. Exit 0 means no findings or successful export, 1 means guardrail findings, and 2 means an operational failure. Type errors must not be interpreted as a clean check.

Suppressions use ` + "`// cyclo-allow(rule_a, rule_b): reason`" + ` above a function, with intervening doc comments allowed. Reasons are required; unknown rules fail validation. Prefer documenting a deliberate exception over hiding evidence.

**Patterns vs quality:** ` + "`cyclo fix`" + ` handles mechanical patterns automatically. ` + "`cyclo check`" + ` findings (side_effect_density, mutated_targets, etc.) are design smells — they need your judgment to refactor. The check tells you *where* to look; you decide *how* to restructure. Don't try to auto-fix quality findings.

## Reduce a bug reproducer

Use the bug reducer once you have an input that reproduces a specific failure and a deterministic checker for that failure. Default mode works with inputs for any codebase and removes whole lines; it does not find or fix bugs itself. Go mode uses Tree-sitter to remove complete declarations, statements, and adjacent groups, largest first, reparsing each accepted reduction.

` + "```sh" + `
cyclo bug-reducer command.sh -- ./checker.sh
cyclo bug-reducer --timeout 30s --output reduced.sh command.sh -- ./checker.sh
cyclo bug-reducer --language go --go-parser /path/to/go.so --tui=false input.go -- ./checker.sh
` + "```" + `

Here, ` + "`command.sh`" + ` is the input file being minimized. ` + "`checker.sh`" + ` receives the absolute candidate file path as its last argument, after any checker arguments supplied on the command line. The reducer invokes the checker; the checker decides how to run or inspect the candidate. Input can also be Go source, JSON, or another file format.

Prefer --language go for Go reproducers. Use the default line mode for other formats or when deliberately experimenting with text reduction. Go mode requires the tree-sitter CLI and a configured source.go grammar, or an explicit Go parser dynamic library supplied with --go-parser. Omit --go-parser when the grammar is configured. If the parser is unavailable, report the setup error rather than silently switching modes.

Go syntax is validated before every candidate reaches the checker. The checker must still validate compilation and behavior; syntax alone does not establish the bug. Go mode has no line-deletion fallback. Adjacent groups allow related statements, such as a declaration and its only use, to disappear together. Structural reduction can use fewer checker runs while leaving a slightly larger reproducer than line reduction; compare both checker count and final size when evaluating it.

Write the checker to exit **0 only when the same bug still occurs**. An ordinary nonzero exit rejects the candidate. Reject unrelated syntax errors, build failures, or different crashes. Confirm that the original reproduces the failure before reducing it. For project-dependent checks, arrange the required build workspace or overlay in the checker.

The checker runs in the launch directory, inherits the environment, and receives no stdin. Terminals automatically show a live dashboard with statistics, size over time, accepted deletions, and checker output. Use tab to change panes, j/k to scroll, enter to expand, and q to stop and save the best accepted input. Completed runs stay open for inspection; q closes the dashboard. Use ` + "`--tui=false`" + ` for unattended runs, or ` + "`--tui`" + ` to force the dashboard. Redirected output uses the plain final summary and suppresses checker output. The dashboard retains the last 16 KiB of output per check.

Candidates are temporary single files. Input must be a regular file. The original stays intact; output defaults to ` + "`command.sh.reduced`" + ` and must not already exist. Flags go before the input; the default timeout is 10 seconds per check.

Timeout, Ctrl-C, Unix SIGTERM, checker signals, and launch errors stop reduction. After the original is accepted, the best accepted candidate is saved even when the run stops with an error. Unix runs clean up the checker's process group; detached descendants and child-process cleanup on other platforms need checker-managed cleanup.

Recheck the reduced file, use it as a regression test, and then fix the bug. Report the original and reduced sizes, the checker used, and validation results. The result is a local minimum for the selected deletion units, not a guarantee of the smallest possible reproducer. Use ` + "`cyclo bug-reducer --help`" + ` for the current CLI contract.
`
