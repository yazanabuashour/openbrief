# High-Touch Workflow UX Audit

## Status

Planning audit for `ob-5he`.

This note reviews successful OpenBrief eval rows that passed safety and
capability gates but still show workflow ceremony. It does not authorize
runner, skill, schema, storage, or eval-harness changes.

## Taste Flags

Taste flags supplement, not replace, safety and capability gates:

- high step count: many tool calls or command executions for a routine task
- long latency: high wall time relative to similar scenarios
- exact prompt choreography: success depends on spelling out runner JSON,
  sequencing, or final-answer shape in the prompt
- surprising record-delivery sequencing: the agent must build the exact brief
  body, call `record_delivery`, and then report without drifting from the
  recorded message
- brittle config mutation: the user intent is simple, but success depends on
  exact config actions or field-level JSON assembly

Safety pass still means the workflow preserved source authority, provenance,
auditability, local-first behavior, private-state boundaries, runner-only
production access, and approval-before-write. Capability pass means current
runner primitives can express the workflow. UX quality is a separate judgment.

## Audit Table

| Workflow | Evidence | Assistant calls | Tool calls | Commands | Seconds | Taste debt |
| --- | --- | ---: | ---: | ---: | ---: | --- |
| Outlet policy watch audit | `docs/agent-eval-results/openbrief-v0.1.0-final.json` scenario `outlet-policy-watch-audit` | 6 | 16 | 16 | 48.50 | Yes: highest command count; exact outlet policy plus source config choreography. |
| Feed recovery resolves warning | `docs/agent-eval-results/openbrief-v0.1.0-final.json` scenario `feed-recovery-resolves-warning` | 7 | 14 | 14 | 44.74 | Yes: two-turn config mutation plus health-delta interpretation. |
| Repeat run without new items | `docs/agent-eval-results/openbrief-v0.1.0-final.json` scenario `repeat-run-no-new-items` | 7 | 10 | 10 | 42.21 | Moderate: safe and understandable, but still depends on multi-turn record/run sequence. |
| Record delivery suppresses repeats | `docs/agent-eval-results/openbrief-v0.1.0-final.json` scenario `record-delivery-suppresses-repeats` | 4 | 10 | 10 | 39.79 | Yes: exact delivery recording remains the surprising step. |
| Configured max delivery items | `docs/release-notes/v0.1.4.md` and `scripts/agent-eval/openbrief/scenarios.go` scenario `configured-max-delivery-items` | not committed | not committed | not committed | not committed | Moderate: release note proves targeted eval passed, but committed artifact lacks reduced metrics; prompt still combines config mutation, brief assembly, and exact delivery recording. |

## Findings

Already correctly handled:

- The high-touch rows passed the safety and capability gate without direct
  SQLite access, broad repo search, repo inspection, or environment access.
- The runner exposes the data needed to complete the workflows through JSON.
- `max_delivery_items` moved into runner-owned runtime configuration and
  `run_brief` output, reducing one previous source of manual config reads.

Likely taste debt:

- Outlet policy watch audit has the clearest step-count signal. A normal user
  wants to know whether watched outlets were audited and candidates were still
  allowed, not to choreograph outlet policy config plus result inspection.
- Feed recovery has a strong latency and turn-count signal. A normal user wants
  a compact status explanation of new and resolved feed health warnings.
- Delivery recording is safe but ceremonial. The exact-current-brief-body
  requirement is auditable, but it is also a recurring source of brittle manual
  sequencing across repeat suppression, run history, and max delivery rows.

## Follow-Up Scope

Future revisit issues should compare candidate surfaces and remain eval/design
scoped. Candidate shapes may include status quo JSON, compact runner help,
runner-owned handoff fields, or a targeted runner action. Any promotion must
show safety, capability, and UX quality separately before implementation work.

Filed follow-up Beads:

- `ob-1qj`: Compare outlet policy audit handoff surfaces
- `ob-z19`: Compare feed recovery status handoff surfaces
- `ob-brh`: Compare delivery recording handoff surfaces
