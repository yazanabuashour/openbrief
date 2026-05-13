# Source Config Field UX Audit

## Status

Planning audit for `ob-uy0`.

This note records source configuration field taste review. It does not add
implementation behavior or change the production runner contract.

## Baseline Evidence

- `internal/domain/source.go` normalizes and validates source keys, kinds,
  URLs, GitHub repositories, sections, thresholds, URL canonicalization,
  outlet extraction, dedup groups, priority rank, and always-report settings.
- `internal/runner/config.go` exposes runner-owned `inspect_config`,
  `replace_sources`, `upsert_source`, `delete_source`,
  `replace_outlet_policies`, and `set_brief_options` actions.
- `skills/openbrief/SKILL.md` documents the supported source kinds,
  thresholds, generic feed-processing fields, and approval-sensitive
  configuration categories.
- `docs/architecture/source-intake-ux-audit.md` separates public inspection,
  reviewed candidate drafting, durable config approval, and unsupported private
  or state imports.

## Field Policy

Use this policy when evaluating future source configuration UX. It is a
documentation and eval-design boundary, not an implementation change.

Infer low-risk defaults when the user intent and provided input make the value
mechanical:

- source kind from an RSS or Atom feed URL, or from an explicit GitHub
  `owner/name` release source
- source key from a reviewed label, host, topic, or repository slug
- label from public feed metadata, page title, topic text, or repository name
- `enabled: true` for newly requested sources
- omitted `threshold`, `url_canonicalization`, and `outlet_extraction` values
  when the runner default is the intended neutral behavior

Propose a reviewed candidate when the value is likely inferable but affects how
briefs are grouped, ranked, displayed, or deduplicated:

- section
- threshold
- dedup group
- priority rank
- always-report
- Google News URL canonicalization
- outlet extraction strategy
- outlet policy drafts

Ask or require explicit approval when the value changes authority, durability,
suppression, private state, or a destructive operation:

- durable `openbrief config` writes
- replacing, deleting, or disabling existing sources
- outlet policy suppression
- source URLs, GitHub repositories, or credentials that were not supplied by
  the user or discoverable from user-provided public input
- imports of delivery history, latest-seen state, run state, or inferred
  private configuration

## Audit Notes

Required clarification is still correct for source authority, credentials,
private data, destructive config changes, outlet suppression, and any durable
write. These checks preserve source authority, provenance, auditability,
local-first behavior, runner-only production access, private-state boundaries,
and approval-before-write.

Likely taste debt appears when an agent asks the user to provide every source
field before drafting anything. Source key, label, kind, neutral defaults, and
some generic processing choices are often candidate material. The better
workflow is usually to inspect the user-provided public input, propose a
complete source candidate, and then ask for approval before the runner write.

The current runner correctly rejects invalid source shapes and keeps writes
behind `openbrief config`. The UX gap is not validation. The gap is the lack of
a compact, runner-owned or documented way to turn a natural request into a
reviewed source candidate without exact JSON choreography.

## Follow-Up Scope

Future eval/design follow-ups should pressure-test field-policy candidates,
not authorize implementation. Candidate shapes may include status quo JSON,
compact runner help, or a runner-owned draft action. Any promoted shape must
keep durable writes approval-gated through `openbrief config`.

Filed follow-up Beads:

- `ob-776`: Design eval for source identity field candidates
- `ob-7an`: Design eval for generic processing field candidates
- `ob-zxn`: Design eval for threshold and always-report approval policy
