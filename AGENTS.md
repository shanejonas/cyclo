# AGENTS.md

Instructions for agents working in this repo. Every contributor here is an
agent that sees only the files it opened, copies the nearest example, and
takes the shortest path that compiles. These rules exist because an agent
(including a past me) got each one wrong at least twice.

## Rule table

| Rule | Enforced by |
|------|-------------|
| A PR must be a real improvement on its own, not cleanup after your own previous PRs | This file (judgment call) |
| An improvement PR must regenerate `quality-baseline.json` in the same PR | `internal/qualitygate` fails the build when any rule's excess rate improves past the 5% tolerance (`make quality-baseline` refreshes it) |
| New stdlib packages must be classified for `side_effect_density` | `TestStdlibCoverage` in `domain/quality` fails if `go list std` reports an unclassified package |
| Golden reports must be regenerated when behavior changes | `TestFixtureGoldenAndDoubleRun` fails; run with `-update` to refresh `adapters/goquality/testdata/sample/report.golden.json` |
| Production code stays under the complexity ceilings | `make check` runs gocyclo (≤6) and gocognit (≤15); `cyclo-check` enforces ≤15 on both dimensions |

## Working agreements

- Open PRs autonomously; never merge without the operator's explicit instruction.
- `make check` and `make quality-gate` must be green before pushing.
- When the tool's own rules flag your draft, fix the code — don't contort it to appease the metric, and don't suppress without a reason. If splitting a function would only game the metric, say so and leave it.
- Prefer the honest tradeoff on style questions; the operator decides fast when the options are real.
