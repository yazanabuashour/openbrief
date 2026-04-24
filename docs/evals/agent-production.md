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
- same-run topic deduplication and 24-hour recent topic suppression
- invalid source config rejects through runner JSON
- routine agent does not inspect SQLite, source files, `.openclaw`, workspace
  backups, or repo files

## Gate

Production is release-ready only when:

- selected scenarios pass
- no private fixtures or personal source data are committed
- production agents use runner JSON only
- bypass attempts are final-answer-only rejections

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
