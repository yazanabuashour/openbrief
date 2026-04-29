# OpenBrief Taste Review Backlog

## Status

Planning backlog created after the v0.1.x production runner and skill process
was already working.

This note records a process correction, not a new public API. It keeps the
existing ADR, POC, eval, decision, and implementation-follow-up workflow while
adding a taste review for cases where OpenBrief is technically safe but
unnecessarily awkward.

## Baseline Evidence

OpenBrief already has strong safety and evidence boundaries:

- `docs/architecture/db-backed-configuration-adr.md` keeps private source
  inventories, outlet policies, delivery history, latest-seen state, and run
  state out of the repository and behind runner-backed configuration.
- `docs/architecture/provider-strategy-poc.md` chose generic feed processing
  over provider-specific source kinds when the fetch interface did not justify
  a new source type.
- `docs/evals/agent-production.md` defines the production runner/skill eval
  gate, and `docs/agent-eval-results/openbrief-v0.1.0-final.md` records the
  selected v0.1.0 scenarios with neutral `<run-root>` placeholders.
- Closed Beads such as `ob-7nv`, `ob-8cp`, and `ob-ek2` corrected over-broad
  rejection policy around legacy migration, repository inspection, and
  non-routine debugging while preserving runner-only production behavior.
- Closed Beads such as `ob-76n`, `ob-tyv`, `ob-h29`, `ob-drf`, `ob-5a7`, and
  `ob-xp7` show the project already turns eval or migration pressure into
  targeted runner-backed decisions only after evidence identifies the exact
  surface and gates.

## Taste Review Lens

Future deferral, reference, or promotion decisions should ask one more question
after the safety and capability checks: would a normal user reasonably expect a
simpler OpenBrief surface here?

Useful signals include:

- a workflow passes but needs many runner calls, assistant turns, exact JSON
  choreography, or brittle manual sequencing
- the user intent fits the natural scope of an existing runner command, but the
  current process treats it as unsupported or forces hand assembly
- the agent asks for approval before a read, fetch, or inspect step when the
  real approval boundary is durable configuration, private state import, or
  another lasting write
- the result is safe but ceremonial, slow, surprising, or hard to explain to a
  routine operator

This lens does not weaken OpenBrief invariants. Source authority, provenance,
auditability, local-first operation, runner-only production access, private
state boundaries, and approval-before-write still decide whether a smoother
surface is acceptable.

## Initial Audit Targets

Re-audit source intake and provider configuration UX. Use the generic
feed-processing decision as the good baseline, then compare natural requests
such as tracking a feed, tracking a GitHub project, adding a Google News topic,
or migrating user-pointed legacy sources. The question is whether the existing
runner surfaces are correct but overly ceremonial for common operator intent.

Re-audit source naming, defaults, and config-field UX. Initial fields include
source key, label, section, threshold, dedup group, priority rank,
always-report, outlet extraction, and URL canonicalization. The question is
when an agent should infer, propose a reviewed candidate, or ask before a
durable config write.

Re-audit high-touch successful workflows. Initial candidates include outlet
policy watch audit, feed recovery, repeat suppression, delivery recording, and
max delivery item configuration. Passing eval rows should still record taste
flags when they require high tool count, long latency, exact prompt
choreography, or surprising record-delivery sequencing.

## Tracker Backlog

The following Beads epics track the revisit work:

- `ob-5sb`: Re-audit source intake and provider configuration UX
- `ob-uy0`: Re-audit source naming, defaults, and config-field UX
- `ob-5he`: Re-audit high-touch successful OpenBrief workflows
- `ob-bho`: Update OpenBrief decision process for taste

These epics are docs and evaluation-design backlog only. They do not authorize
runner actions, schema changes, storage migrations, skill behavior changes, or
implementation follow-up. Any future implementation still needs targeted eval
evidence and an explicit promotion decision naming the exact surface and gates.

## Future Report Shape

Future decision and eval reports should separate:

- safety pass: the workflow preserved provenance, source authority,
  auditability, local-first behavior, private-state boundaries, runner-only
  production access, and approval-before-write
- capability pass: current primitives can technically express the workflow
- UX quality: the workflow is or is not acceptable for routine use, including
  any taste debt from ceremony, latency, high step count, brittle prompts, or
  surprising clarification
