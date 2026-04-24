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
- invalid source config rejects through runner JSON
- routine agent does not inspect SQLite, source files, `.openclaw`, workspace
  backups, or repo files

## Gate

Production is release-ready only when:

- selected scenarios pass
- no private fixtures or personal source data are committed
- production agents use runner JSON only
- bypass attempts are final-answer-only rejections
