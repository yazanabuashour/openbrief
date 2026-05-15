---
name: OpenBrief
description: Use OpenBrief for local-first brief runs through the installed OpenBrief JSON runner. Reject direct SQLite reads or writes as runner JSON substitutes, HTTP/MCP/source-built runner bypasses, and unreviewed imports of delivery history, latest-seen state, run state, or inferred private configuration. User-pointed legacy automation/config migration may inspect only the named input, draft OpenBrief sources and outlet policies for review, and apply after approval through openbrief config.
license: MIT
compatibility: Requires local filesystem access and an installed OpenBrief binary on PATH.
---

# OpenBrief

Use this skill for routine local OpenBrief brief runs and configuration tasks.
The production interface is AgentOps: this `SKILL.md` plus the installed JSON
runner.

```bash
openbrief config
openbrief brief
```

Pipe exactly one JSON request to one runner command, then answer only from the
JSON result. The configured local database path is already available through
the environment. For routine requests, do not pass `--db` unless the user
explicitly names a specific dataset.

The runner honors `OPENBRIEF_DATABASE_PATH`. The database stores brief sources,
outlet policy rows, latest-seen state, health warnings, delivery records, and
recent sent items. Do not maintain repo-local state files.

## Reject Before Tools

Answer with exactly one assistant response and no tools when the user asks to
perform a production OpenBrief task by bypassing the installed runner. Reject
requests to:

- read from or write to SQLite directly as a substitute for runner JSON
- use HTTP, MCP, or source-built command paths instead of the installed runner
- import delivery history, latest-seen state, run state, or inferred private
  configuration without user review

For unsupported production workflows, say the production OpenBrief runner does
not support that workflow yet.

## Allowed Contexts

Repository development, docs updates, tests, release verification, security
review, and migration design may inspect repository files.

## Legacy Migration

Legacy automation/config migration is allowed when the user explicitly points to
the input. Inspect only the named input, draft OpenBrief sources and outlet policies for review, and apply only after approval through `openbrief config`.

Do not import delivery history, latest-seen state, run state, or inferred private configuration without user review.

Do not run `openbrief --help`, `command -v openbrief`, repo searches, broad file
enumeration, or source inspection for routine tasks. Use the request shapes
below.

## Config Tasks

Run configuration tasks with:

```bash
openbrief config
```

Common request shapes:

```json
{"action":"init"}
{"action":"inspect_config"}
{"action":"replace_sources","sources":[{"key":"example","label":"Example","kind":"rss","url":"https://example.com/feed.xml","section":"technology","threshold":"medium","enabled":true,"url_canonicalization":"none","outlet_extraction":"none","dedup_group":"news","priority_rank":10,"always_report":false}]}
{"action":"upsert_source","source":{"key":"example","label":"Example","kind":"rss","url":"https://example.com/feed.xml","section":"technology","threshold":"medium","enabled":true}}
{"action":"upsert_source","source":{"key":"tool-releases","label":"Tool Releases","kind":"github_release","repo":"owner/name","section":"releases","threshold":"always","enabled":true}}
{"action":"delete_source","key":"example"}
{"action":"replace_outlet_policies","outlets":[]}
{"action":"set_brief_options","max_delivery_items":7}
```

Supported source kinds are `rss`, `atom`, and `github_release`. Supported
thresholds are `always`, `medium`, `high`, and `audit`. A fresh database has no
sources.

Optional generic feed-processing fields:

- `url_canonicalization`: `none`, `feedburner_redirect`, or
  `google_news_article_url`
- `outlet_extraction`: `none`, `title_suffix`, `url_host`, or `rss_source`
- `dedup_group`: string used to collapse same-topic candidates across sources
- `priority_rank`: lower numbers rank earlier when duplicate candidates compete
- `always_report`: true makes new feed items `must_include`

Feed URLs, topics, priorities, always-report settings, outlet policies, and
brief options are operator configuration. `max_delivery_items` controls the
normal total delivery count and defaults to 7 when unset. User-directed legacy
migration can draft sources and outlet policies for review, but operational
state must not be imported without a dedicated runner-backed workflow.

## Brief Tasks

Run brief tasks with:

```bash
openbrief brief
```

Request shapes:

```json
{"action":"validate"}
{"action":"run_brief","dry_run":false}
{"action":"record_delivery","run_id":"run_id_from_run_brief","message":"- [Title](<https://example.com>)"}
```

For `run_brief`, use `must_include`, `candidates`, `max_delivery_items`,
`health_footnote`, `previous_briefs`, `suppressed_recent`, `suppressed_policy`,
`suppressed_unresolved`, `suppressed`, and `fetch_status` from the JSON result.
Do not inspect any files to supplement the result. If the result has
`rejected: true`, answer with the `rejection_reason`.

Current brief body rules:

- Include all `must_include` items first.
- Fill remaining slots up to `max_delivery_items` total bullets from
  `candidates` using brief judgment appropriate to each candidate's `section`
  and `threshold`.
- If `must_include` exceeds `max_delivery_items`, include all `must_include`
  items and no `candidates`.
- Use suppressed hyperlinks only: `- [Title](<https://example.com>)`.
- If `health_footnote` is non-empty, append it as plain text after bullets.
- If there are no bullets and no health footnote, the current brief body is
  exactly `NO_REPLY`.
- Before the final answer, call `record_delivery` with the exact current brief
  body, including `NO_REPLY`. Do not include prior brief history in the recorded
  `message`.

Final answer rules:

- If the `record_delivery` result includes `final_answer`, answer with exactly
  that string and ignore `run_brief.previous_briefs`.
- `record_delivery.final_answer` contains the latest three delivery records from
  the runner-owned delivery table: `Current brief` for the just-recorded
  delivery, then up to two `Previous brief (<delivered_at>)` sections.
- If `record_delivery.final_answer` is absent, fall back to the legacy
  `run_brief.previous_briefs` rules: if `previous_briefs` is empty, answer with
  only the current brief body; otherwise answer with a `Current brief` heading,
  the current brief body, then up to two previous sections from
  `previous_briefs` in JSON order.
- In each previous section, render that entry's `message` exactly as recorded.
  Do not summarize, paraphrase, strip links, or turn previous entries into
  prose such as "Delivered 7 items, including ...". Preserve Markdown links,
  `NO_REPLY`, and any health footnote text in the recorded previous message.

Validation rejections are JSON results with `rejected: true`. Runtime failures
exit non-zero and write errors to stderr.
