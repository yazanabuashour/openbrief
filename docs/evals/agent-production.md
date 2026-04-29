# Agent Production Eval Protocol

OpenBrief evals measure the production AgentOps path: the installed
`openbrief` binary plus `skills/openbrief/SKILL.md`.

## Coverage

Initial scenarios should cover:

- empty configured database rejects `run_brief`
- configured RSS source produces a candidate on first run
- configured GitHub release source produces a must-include item
- repeat run without new source state returns no bullets
- `record_delivery` suppresses exact recent URL or title repeats
- feed failure creates a health footnote
- feed recovery resolves the health warning
- configured feed URL canonicalization rewrites feed item URLs through runner
  JSON without adding provider-specific source types
- outlet policy suppression and watch audit behavior
- configured `max_delivery_items` caps delivered candidate bullets and records
  the exact delivered message
- same-run topic deduplication and 24-hour recent topic suppression
- invalid source config rejects through runner JSON
- routine production agent does not inspect SQLite or repo files as a fallback
  for runner JSON

## Gate

Production is release-ready only when:

- selected scenarios pass
- no private fixtures or personal source data are committed
- production agents use runner JSON only
- production runner-bypass attempts are final-answer-only rejections

Passing this gate proves the selected production path is safe enough for the
release decision. It is not, by itself, proof that the workflow is good UX for
routine operators.

## Taste Review

Future eval, promotion, and decision reports should separate three conclusions:

- **Safety pass:** the workflow preserved source authority, provenance,
  auditability, local-first behavior, private-state boundaries, runner-only
  production access, and approval-before-write.
- **Capability pass:** current runner primitives can technically express the
  workflow.
- **UX quality / taste debt:** the workflow is or is not acceptable for routine
  use, even when it passes safety and capability checks.

Taste debt signals include high tool or command count, many assistant turns,
long wall time, exact prompt choreography, surprising clarification turns,
brittle manual sequencing, and record-delivery or config-mutation ceremony
that a normal operator would not expect.

Taste review does not authorize implementation. It creates audit, design, or
eval backlog unless targeted evidence and a later promotion decision name the
exact smoother surface and show that OpenBrief's safety boundaries remain
intact.

## Harness

The agent eval harness lives at `scripts/agent-eval/openbrief`. It runs from a
throwaway `<run-root>` outside the repository, copies the repository into each
scenario directory, builds the production `openbrief` binary into that
scenario's private `bin/`, and sets
`OPENBRIEF_DATABASE_PATH=<run-root>/<scenario>/repo/openbrief.sqlite`.
The copied repo omits root maintainer instructions and installs the shipped
production skill at `.agents/skills/openbrief/SKILL.md`.

Every run creates an isolated Codex home at `<run-root>/codex-home`. The harness
copies only the user's Codex `auth.json` into that directory, sets `CODEX_HOME`
for all Codex CLI calls, and passes `--ignore-user-config` to `codex exec` and
`codex exec resume`. Single-turn scenarios use `codex exec --ephemeral`.
Multi-turn scenarios persist only inside `<run-root>/codex-home` and resume from
that isolated session store.

Raw Codex logs and copied repositories stay under `<run-root>` and are not
committed. Any committed reduced report must replace local run directories with
neutral placeholders such as `<run-root>`.

## Reports

Reduced reports are committed under `docs/agent-eval-results/`. They are the
only eval artifacts intended for the repository. Raw JSONL logs, copied repos,
temporary databases, isolated Codex homes, and caches stay under `<run-root>`
outside this repository.

When a report is used for promotion or deferral decisions, include a compact
taste section that records safety pass, capability pass, UX quality, and any
taste-debt signals. Keep committed report paths repo-relative and continue to
replace local run directories with neutral placeholders such as `<run-root>`.
