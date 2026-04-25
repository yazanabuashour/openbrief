# ADR: DB-Backed Configuration And State

## Status

Accepted for the initial OpenBrief scaffold.

## Context

OpenBrief is intended to be open sourced. Personal source inventories, paywall
policy, delivery history, and latest-seen state must not be committed to the
repository or encoded in the skill.

Agents also need a narrow production interface that does not require source
inspection, workspace reads, direct SQLite queries, or legacy scripts for
routine runtime tasks. Repository development, docs review, tests, release
verification, security review, and migration design can still inspect public
repository files.

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

Private historical artifacts are not authoritative production configuration.
Recovering or importing personal source inventories, outlet policies, delivery
history, or run state remains unsupported until the runner provides an explicit
import path.

Configuration version `v2` adds generic feed-processing fields to sources:
URL canonicalization, outlet extraction, dedup group, priority rank, and
always-report behavior. These are generic source settings; they do not embed any
operator feed inventory.

## Consequences

- The shipped artifact can be public without private brief data.
- Routine production agents use runner JSON results instead of reading files.
- Local operators can keep private configuration in host storage.
- Repo development and migration design can inspect public repository files.
- Import/migration from legacy personal workflows is outside the repository
  until it is implemented as a runner-backed feature.
