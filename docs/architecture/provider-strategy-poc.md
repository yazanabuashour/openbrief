# POC: Provider Strategy

## Status

Initial production scope selected.

## Decision

The initial `openbrief` runner supports:

- RSS feeds
- Atom feeds
- GitHub releases via the public GitHub releases API

The runner does not implement Google News decoding, paywall heuristics, or
advanced topic deduplication in the initial scaffold.

## Rationale

RSS/Atom and GitHub releases are stable enough to validate the AgentOps surface,
SQLite-backed configuration, source state, delivery deduplication, and health
warnings without committing private configuration or reverse-engineered provider
logic.

Google News decoding, paywall policy behavior, and more aggressive deduplication
need separate ADRs, fixtures, failure-mode analysis, and production agent evals
before they become part of the shipped contract.
