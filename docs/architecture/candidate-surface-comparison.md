# Candidate Surface Comparison

## Status

Accepted eval/design comparison for `ob-rab`, `ob-8nt`, `ob-k92`, `ob-ivy`,
`ob-776`, `ob-7an`, `ob-zxn`, `ob-1qj`, `ob-z19`, and `ob-brh`.

This comparison closes the current taste follow-ups without authorizing runner,
skill, schema, storage, migration, or CI changes. Reopen the tracks only with
new eval evidence that shows repeated operator failure, high ceremony, or
unsafe ambiguity after the documented guidance below is applied.

## Candidate Set

The comparison used the same candidate set across source capture, field policy,
and high-touch handoff work:

- status quo JSON: keep current `openbrief config` and `openbrief brief`
  actions only
- compact docs/help: document infer/propose/ask and handoff rules outside the
  production skill
- runner-owned draft or handoff fields: add machine-readable candidate or
  summary output to an existing action
- targeted runner action: add a new narrow action for one workflow

The production safety boundary stays unchanged: durable config writes require
approval and go through `openbrief config`; routine brief generation uses
runner JSON; private state imports remain unsupported without promoted runner
support.

## Source Capture Decisions

| Bead | Flow | Decision | Rationale |
| --- | --- | --- | --- |
| `ob-rab` | Feed URL source capture | Choose compact docs/help for now. | `upsert_source` already covers the write. The missing part is candidate assembly, and `docs/architecture/source-intake-ux-audit.md` plus `docs/architecture/source-config-field-ux-audit.md` now define the inspection and approval ladder. |
| `ob-8nt` | GitHub release source capture | Choose compact docs/help for now. | `github_release` already has a distinct runner source kind and `owner/name` validation. A new draft action is not justified without eval evidence that agents still fail after the field policy. |
| `ob-k92` | Google News topic source capture | Choose compact docs/help and keep generic feed processing. | The provider strategy already rejects a `google_news` source kind. The right candidate is RSS plus `google_news_article_url` when a public topic feed is supplied. |
| `ob-ivy` | User-pointed legacy source migration draft | Keep manual reviewed drafts; do not select a runner import or dry-run action yet. | Legacy inputs can vary widely and may contain private state. The current allowed path, inspect only the named input, draft sources and outlet policies, then write approved config, is the safest viable shape. |

Outcome: no source-capture implementation follow-up is filed. The selected
surface is compact docs/help plus current runner JSON. A runner-owned draft
action remains unpromoted until a future eval shows repeated failure under this
policy.

## Field Policy Decisions

| Bead | Flow | Decision | Rationale |
| --- | --- | --- | --- |
| `ob-776` | Source identity fields | Choose compact docs/help. | Key, label, kind, and `enabled` defaults can be inferred or proposed using the field policy without changing runner behavior. |
| `ob-7an` | Generic processing fields | Choose compact docs/help. | URL canonicalization, outlet extraction, dedup group, and priority rank need reviewed proposals, but a provider-specific API would weaken the generic feed decision. |
| `ob-zxn` | Threshold and always-report policy | Choose explicit reviewed proposals; do not allow silent escalation. | Threshold and always-report affect alerting priority and final brief shape, so durable writes still need approval. |

Outcome: no field-policy implementation follow-up is filed. The field-policy
doc is the selected surface until eval evidence shows it is insufficient.

## High-Touch Handoff Decisions

| Bead | Flow | Decision | Rationale |
| --- | --- | --- | --- |
| `ob-1qj` | Outlet policy audit handoff | Keep status quo JSON plus compact answer guidance. | `suppressed_policy` already carries the audit evidence. A targeted audit summary action would duplicate existing result data. |
| `ob-z19` | Feed recovery status handoff | Keep status quo JSON plus compact answer guidance. | `health_footnote` and `health_delta` already distinguish new and resolved warnings. A new health summary action is not justified yet. |
| `ob-brh` | Delivery recording handoff | Keep exact `record_delivery` ceremony. | The exact delivered message is the audit artifact that powers repeat suppression and history. Runner-owned shortcuts risk recording a message the user did not actually deliver. |

Outcome: no high-touch implementation follow-up is filed. The selected shape is
current runner JSON plus the existing skill/report guidance. Future evals may
reopen this only if the exact-message and health/audit interpretation rules
continue to produce failures or excessive ceremony.

## Closure

The current candidate comparison chooses the smallest safe surface: current
runner JSON plus compact docs/help. No runner API is promoted, no direct-copy
OpenClerk action is adopted, and no additional follow-up Beads are needed from
this comparison.
