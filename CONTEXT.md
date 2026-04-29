# OpenBrief Context

This file defines shared project vocabulary for architecture reviews and
implementation discussions.

## Terms

- **Runner**: The installed `openbrief` command that accepts one structured
  JSON request on stdin and returns one structured JSON result on stdout.
- **Skill**: The single-file agent policy in `skills/openbrief/SKILL.md` that
  tells agents when and how to use the runner.
- **Source**: Operator configuration for an input stream such as RSS, Atom, or
  GitHub releases. Sources include fetch settings and generic feed-processing
  settings such as URL canonicalization, outlet extraction, dedup group,
  priority rank, and always-report behavior.
- **Outlet policy**: Operator configuration that allows, blocks, or audits
  items from a named outlet and its aliases.
- **Source state**: Mutable latest-seen state for a source. It is stored in the
  host SQLite database and is not committed to this repository.
- **Delivery**: The final brief message recorded through `record_delivery` so
  recent item suppression can avoid repeats.
- **Health warning**: Mutable runner state that reports new or resolved runtime
  problems such as failing feeds, repeated fetch failures, or stale heartbeat
  data.
- **Agent eval**: A production-path scenario that verifies the installed
  runner and shipped skill preserve runner-only behavior, private-state
  boundaries, and expected brief results.

## Invariants

- Routine production tasks go through runner JSON, not direct SQLite reads,
  source-built runners, broad repository inspection, or ad hoc scripts.
- Personal source inventories, outlet policy evidence, delivery history,
  latest-seen state, run history, workspace backups, and local databases stay
  out of the repository.
- Durable configuration writes require explicit operator intent and go through
  `openbrief config`.
- Public runner JSON changes require targeted eval evidence and an explicit
  promotion decision.
