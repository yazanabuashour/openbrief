# ADR: DB-Backed Configuration And State

## Status

Accepted for the initial OpenBrief scaffold.

## Context

OpenBrief is intended to be open sourced. Personal source inventories, paywall
policy, delivery history, and latest-seen state must not be committed to the
repository or encoded in the skill.

Agents also need a narrow production interface that does not require source
inspection, workspace reads, direct SQLite queries, or legacy scripts.

## Decision

OpenBrief stores runtime configuration and mutable state in SQLite. The
database path is the storage anchor.

The only app-specific environment variable is `OPENBRIEF_DATABASE_PATH`.
The runner also accepts `--db` for explicit datasets and tests. If neither is
provided, it uses `${XDG_DATA_HOME:-~/.local/share}/openbrief/openbrief.sqlite`.

The repository seeds no personal sources, outlet policies, latest-seen state,
delivery records, or run history. A fresh database contains only schema and
runtime defaults. Operators configure sources through `openbrief config` or by
preparing a host database outside this repository.

## Consequences

- The shipped artifact can be public without private brief data.
- Routine agents use runner JSON results instead of reading files.
- Local operators can keep private configuration in host storage.
- Import/migration from legacy personal workflows is outside the repository.
