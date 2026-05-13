---
decision_id: openclerk-pattern-adoption-audit
decision_status: accepted
decision_scope: openclerk-pattern-adoption
decision_owner: agentops
decision_date: 2026-05-13
source_refs: AGENTS.md, docs/maintainers.md, docs/architecture/openbrief-taste-review-backlog.md, skills/openbrief/SKILL.md, OpenClerk AGENTS reference, OpenClerk skill-reduction ADR reference, OpenClerk thin-skill workflow decision reference
---

# OpenClerk Pattern Adoption Audit

## Context

This audit compares OpenClerk's AgentOps operating model against OpenBrief after
the v0.1.x OpenBrief runner and skill shipped. OpenClerk is evidence, not source
text to copy. OpenBrief remains a local-first brief runtime with generic
feed/release sources, outlet policies, delivery history, latest-seen state, and
private operator configuration kept behind the installed JSON runner.

The comparison covered `AGENTS.md`, `skills/*/SKILL.md`, `mise.toml`,
`CONTRIBUTING.md`, maintainer docs, runner entrypoints, skill validators, and
the OpenClerk decisions on skill reduction and thin workflow surfaces.

## Decisions

| Pattern | Decision | OpenBrief action |
| --- | --- | --- |
| Taste review with follow-up discipline | Adopt | Strengthen `AGENTS.md` so non-promotion decisions separate safety, capability, and UX quality, then create or link Beads when the need remains valid. |
| Work-item completion model | Adapt | Preserve OpenBrief's existing manual-review pause before staging, commit, `bd dolt push`, or `git push`. Do not copy OpenClerk's auto-commit-after-review rule without a separate workflow decision. |
| Maintainer tool pinning | Adopt | Pin Beads and Dolt in `mise.toml` so maintainer workflow commands can use the same tool versions across local docs and CI. |
| All-skill validation wrapper | Adopt lightly | Add `scripts/validate-all-agent-skills.sh` even though OpenBrief currently ships one skill; this keeps future optional skills or modules from bypassing validation. |
| Thin skill doctrine | Adapt | Treat `skills/openbrief/SKILL.md` as activation, routing, and safety policy. Move growing brief/config recipes to runner results, compact help, eval docs, or maintainer docs when evidence supports the move. |
| OpenClerk module architecture | Defer | OpenBrief has no accepted optional-provider or module surface. Do not add module docs, installers, or module skill packaging from this audit alone. |
| Runner `capabilities` or workflow guide | Defer pending comparison | OpenBrief has only `config` and `brief` commands today. A read-only `capabilities` or workflow-guide action may be useful, but it needs a candidate comparison against compact help and current primitives. |
| `agent_handoff` result fields | Defer pending eval evidence | OpenBrief `run_brief` already returns candidate, suppression, health, and history fields. Add handoff fields only if evals show repeated answer assembly ceremony that runner output can safely reduce. |
| OpenClerk source/document workflow actions | Reject as direct copy | OpenClerk document, retrieval, provenance, synthesis, OCR, and semantic retrieval surfaces do not map directly to OpenBrief's brief-generation domain. |

## Accepted Follow-Up

The audit does not authorize runner schema changes or storage migrations. It
does authorize focused follow-up Beads for the remaining accepted questions:

- `ob-qbu`: define an OpenBrief skill-size budget and migration policy for moving workflow
  detail out of `skills/openbrief/SKILL.md`.
- `ob-wgw`: compare OpenBrief runner-owned discovery surfaces: status quo plus compact
  help, a read-only `capabilities` command, or a narrow workflow guide.
- `ob-48t`: assess committed-artifact validation for public prompts, release docs, and
  repo-relative path hygiene if the current release checks leave gaps.

Existing taste-review Beads remain the right place for source-intake,
source-naming, and high-touch successful workflow audits. This decision adds the
OpenClerk-derived operating pattern; it does not replace those eval tracks.

## Compatibility

OpenBrief's production interface remains unchanged: agents use installed
`openbrief config` and `openbrief brief` JSON. Routine production tasks must not
inspect SQLite directly, use source-built commands, bypass the runner with HTTP
or MCP, import private operational state without reviewed runner support, or
write durable configuration without approval.

All committed docs, reports, and artifact references must stay repo-relative or
use neutral placeholders. Private source inventories, outlet policy evidence,
delivery logs, run history, `.openclaw` content, workspace backups, and local
SQLite databases remain out of the repository.
