# Source Intake UX Audit

## Status

Planning audit for `ob-5sb`.

This note records source and provider intake taste review. It does not
authorize runner, schema, storage, skill, or migration changes.

## Baseline Evidence

- `docs/architecture/provider-strategy-poc.md` chose generic feed processing
  instead of provider-specific source kinds.
- `docs/architecture/db-backed-configuration-adr.md` keeps private source
  inventories, outlet policies, delivery history, latest-seen state, and run
  state out of the repository and behind runner-backed configuration.
- `docs/evals/agent-production.md` covers configured RSS, GitHub release,
  generic processing fields, invalid source config rejection, and runner-only
  production hygiene.
- `docs/agent-eval-results/openbrief-v0.1.0-final.md` records the selected
  v0.1.0 agent eval rows with neutral `<run-root>` placeholders.
- `skills/openbrief/SKILL.md` documents the production JSON runner shapes for
  `inspect_config`, `replace_sources`, `upsert_source`, `delete_source`,
  `replace_outlet_policies`, and `set_brief_options`.

## Current Intake Boundary

The current runner surface is correct when the source shape is already known.
`upsert_source` and `replace_sources` accept reviewed `rss`, `atom`, and
`github_release` sources, reject invalid source fields through runner JSON, and
keep durable writes behind `openbrief config`. Generic processing fields cover
Google News article URL canonicalization, outlet extraction, dedup groups,
priority rank, and always-report behavior without adding a `google_news`
source kind.

That is a capability pass, not proof that natural source intake is good UX.
The current skill and runner make a routine operator or agent assemble exact
source JSON before the durable write. That is acceptable for explicit config
maintenance, but it is too rigid for requests where the user supplies public
intake material and expects OpenBrief to draft a reviewed source candidate.

## Intake Ladder

1. Public read, fetch, or inspect: user-provided public feed URLs, GitHub
   release sources, Google News topic inputs, and explicitly named legacy
   config or automation files can justify inspection for drafting a candidate.
   Inspection permission is not durable configuration approval.
2. Draft for review: source keys, labels, sections, thresholds, generic
   processing fields, outlet policies, and brief options should be proposed as
   reviewed candidates when they can be inferred from the user-provided input.
3. Durable write: storing or replacing sources, outlet policies, or brief
   options still requires explicit user approval and must go through
   `openbrief config`.
4. Unsupported private or state import: delivery history, latest-seen state,
   run state, inferred private configuration, and unreviewed source inventory
   imports remain unsupported unless a future runner-backed import path is
   promoted through evidence.

## Audit Notes

Already correctly handled:

- Generic feed parity is the right model for ordinary RSS, Atom, blogs,
  newsletters, and Google News feeds. They share the same fetch interface, so a
  new source kind would add surface area without improving provenance or
  auditability.
- GitHub releases are correctly modeled as `github_release` because their fetch
  interface is different from feed parsing.
- Invalid source fields are runner rejections, so agents do not need direct
  SQLite reads or source-built validation.
- User-pointed legacy migration is correctly limited to inspecting the named
  input, drafting sources and outlet policies, and applying approved config
  through the runner.

Over-rigid intake boundaries:

- "Track this feed" still requires the agent to hand-derive a key, label,
  section, threshold, and optional generic fields before calling
  `upsert_source`.
- "Track this GitHub project" still requires the agent to know the
  `github_release` source shape and the `owner/name` repository form.
- "Add this Google News topic" still requires the agent to know that the result
  is an RSS source plus `google_news_article_url`, not a new source kind.
- "Migrate these user-pointed sources" is allowed as a reviewed draft, but
  there is no runner-owned dry-run or candidate report shape that keeps the
  inspection, proposal, and approval steps compact.

## Follow-Up Scope

Future eval or design follow-ups should compare source-capture candidates, not
authorize implementation. Candidate shapes may include status quo JSON,
compact runner help, or a runner-owned draft action. Any promoted shape must
preserve local-first behavior, source authority, provenance, runner-only
production access, private-state boundaries, and approval-before-write.

Filed follow-up Beads:

- `ob-rab`: Design eval for feed URL source capture
- `ob-8nt`: Design eval for GitHub release source capture
- `ob-k92`: Design eval for Google News topic source capture
- `ob-ivy`: Design eval for user-pointed legacy source migration draft
